# Native modern HTTP/API feature set for `fh`

This build adds a native developer platform layer around the existing custom HTTP runtime while keeping the original `func(*fh.Ctx) error` route API intact.

## 1. Server/runtime necessities

Implemented or wired in the runtime:

- HTTP/1.1 keep-alive with `DisableKeepAlive` escape hatch.
- HTTP/2 support with ALPN/h2c paths already present in the runtime.
- `ReadTimeout`, `WriteTimeout`, `IdleTimeout`.
- `MaxRequestBodySize`, `MaxHeaderListSize`, `MaxRequestLineSize`, and new `MaxHeaderCount`.
- `MaxConnections` semaphore.
- Graceful shutdown and timeout shutdown.
- Drain mode through shutdown: closes idle connections and sends HTTP/2 drain.
- Connection lifecycle hooks: `OnConnect`, `OnClose`, `OnListen`, `OnShutdown`, `OnError`.
- Request lifecycle hooks: `fh.LifecycleHooks`.

Best practice defaults:

```go
app := fh.New(fh.Config{
    ReadTimeout:        5 * time.Second,
    WriteTimeout:       10 * time.Second,
    IdleTimeout:        60 * time.Second,
    MaxRequestBodySize: 1 << 20,
    MaxHeaderListSize:  32 << 10,
    MaxHeaderCount:     64,
    MaxRequestLineSize: 8 << 10,
    MaxConnections:     100_000,
})
```

## 2. Typed endpoints and OpenAPI

Native typed handlers:

```go
app.PostTyped("/users", func(c *fh.Ctx, req CreateUserRequest) (UserResponse, error) {
    return createUser(req)
})
```

The wrapper automatically:

- Parses JSON request bodies.
- Calls `Validate() error` when implemented by the DTO.
- Calls the typed handler.
- Encodes JSON response.
- Emits structured JSON errors.
- Registers request/response schemas for `app.OpenAPI()` and `app.EnableOpenAPI()`.

Note: Go does not support generic methods, so `PostTyped` accepts `any` and validates the function signature at registration time. The application code remains typed.

## 3. Security essentials

Native or included middleware/helpers:

- Security headers via `mw/security`.
- CORS via `mw/cors`.
- CSRF via `mw/csrf`.
- Basic auth via `mw/basicauth`.
- Request ID via `mw/requestid`.
- Rate limiting via `mw/ratelimiter`.
- Body limit via `fh.BodyLimit` and `mw/bodylimit`.
- Request timeout via `fh.RequestTimeout` and `mw/timeout`.
- API key middleware: `fh.APIKey`.
- IP allow/block list: `fh.IPList`.
- HMAC signed requests: `fh.HMACSignedRequest`.
- HMAC webhook verification: `fh.HMACWebhook`.
- Replay protection with in-memory store and pluggable `ReplayProtectionStore`.
- Constant-time compare: `fh.ConstantTimeEqual`.
- Secret redaction: `fh.RedactSecret`.
- Signed cookie helpers: `fh.SignCookie`, `fh.VerifySignedCookie`.
- Session middleware with secure cookie support under `mw/session`.

Recommended edge chain:

```go
app.Use(requestid.New())
app.Use(security.New())
app.Use(cors.New(cors.Config{AllowOrigins: []string{"https://app.example.com"}}))
app.Use(fh.BodyLimit(1 << 20))
app.Use(fh.RequestTimeout(5 * time.Second))
app.Use(fh.APIKey(fh.APIKeyConfig{Header: "X-API-Key", Keys: keys}))
app.Use(fh.ReplayProtection(nil, "X-Nonce", 5*time.Minute))
```

## 4. Reliability, idempotency, queues, and jobs

Existing reliability primitives are preserved and expanded:

- Request ID.
- Idempotency key support.
- Deterministic idempotency middleware.
- Response replay.
- Body hash conflict detection.
- Request journal.
- Durable file-backed queue by default.
- Storage interfaces: `RequestJournalStore`, `IdempotencyRepository`, `QueueStorage`.
- Outbox/inbox helpers already present in reliability extensions.
- Transactional reliability API: `Reliability.BeginTx`.
- DLQ via failed queue directory.
- Retry/backoff.
- Priority.
- Delay/scheduled visibility.
- Concurrency key.
- Queue stats and events JSONL.
- Atomic request-to-job handoff through `fh.AtomicJob`.

Per-route reliability:

```go
app.Post("/orders",
    fh.Reliable(fh.ReliabilityPolicy{
        Enabled:             true,
        RequireIdempotency:  true,
        Journal:             true,
        ReplayResponse:      true,
        ConflictOnBodyDrift: true,
        MaxReplayAge:        24 * time.Hour,
    }),
    createOrder,
)
```

Deterministic idempotency:

```go
app.Post("/payments",
    fh.DeterministicIdempotency(func(c *fh.Ctx) string {
        return c.Get("X-Tenant-ID") + ":payment:" + c.Get("X-External-ID")
    }),
    fh.Reliable(paymentPolicy),
    createPayment,
)
```

Durable async endpoint:

```go
app.Post("/emails", fh.Reliable(emailPolicy), func(c *fh.Ctx) error {
    job, err := fh.AtomicJob(c, fh.AtomicJobOptions{
        Type:     "email.send",
        Body:     c.BodyCopy(),
        Priority: fh.PriorityHigh,
    })
    if err != nil { return err }
    return c.Status(202).JSON(fh.Map{"status": "accepted", "job_id": job.ID})
})
```

SQLite and Postgres should be provided as separate adapters implementing the storage interfaces, so the core server remains stdlib-only and high-performance. The contracts are complete; no application code must change when swapping storage.

## 5. Gateway and reverse proxy

Available native gateway pieces:

- `fh.ReverseProxy(fh.ProxyConfig{...})`.
- `fh.APIGateway(map[string]fh.ProxyConfig{...})`.
- Path strip/add rewrite.
- Header/director rewrite.
- Per-upstream timeout.
- Circuit breaker middleware.

Recommended additions for later adapter-specific gateway packages:

- Active upstream health checks.
- Weighted pools.
- Request mirroring/traffic shadowing.
- WebSocket/SSE proxy hardening.
- Service discovery interface.

## 6. Streaming and real-time

Native support:

- SSE: `ctx.SSE`.
- WebSocket package under `pkg/websocket`.
- Chunked responses in the runtime.
- File/static response helpers.
- Cancellation available through `ctx.Context()`.

SSE example:

```go
app.Get("/events", func(c *fh.Ctx) error {
    return c.SSE(func(s *fh.SSE) error {
        return s.Event("queue.stats", stats)
    })
})
```

Use SSE for job progress, queue stats, workflow progress, log streaming, and admin dashboards. Use WebSocket for bidirectional chat, collaboration, presence, and real-time dashboards.

## 7. Static files and assets

`fh.StaticAdvanced` adds:

- Safe path cleaning and path traversal protection.
- ETag.
- Last-Modified.
- Cache-Control / immutable assets.
- Download disposition.
- Content-Type detection.
- Index control.
- SPA fallback.

```go
app.Get("/static/*", fh.StaticAdvanced("./public", fh.StaticAdvancedConfig{
    ETag: true,
    LastModified: true,
    CacheControl: "public, max-age=31536000, immutable",
}))
```

## 8. API versioning and deprecation

Use `fh.APIVersion` with `fh.APIVersionConfig`:

```go
app.Use(fh.APIVersion(fh.APIVersionConfig{
    Header:  "Accept-Version",
    Default: "2026-06-01",
    Allowed: []string{"2026-01-01", "2026-06-01"},
    Deprecated: map[string]string{"2026-01-01": "2026-12-31"},
}))
```

It sets `api_version` in locals and emits `Deprecation` and `Sunset` headers for deprecated versions.

## 9. Developer experience

Native DX endpoints and helpers:

- `app.Routes()` route registry.
- `app.EnableRouteList("/_fh/routes")`.
- `app.EnableOpenAPI("/openapi.json", ...)`.
- `app.EnableDocs("/docs")`.
- `app.EnableMetrics("/_fh/metrics")`.
- Existing test helpers and benchmark files remain.
- Mockable queue/journal interfaces.
- `fh doctor` can be added as a CLI wrapper around config validation and route inspection.

## 10. High-performance design rules used

- Keep the low-level `HandlerFunc` path unchanged.
- Do not force typed/reflection path on normal routes.
- Make reliability, OpenAPI, gateway, security, and DX opt-in.
- Use file-backed durability by default for zero external dependency local production.
- Keep DB backends behind interfaces.
- Validate all external IDs and headers before storing or replaying.
- Prefer append-only journals and atomic rename for file-backed queue correctness.

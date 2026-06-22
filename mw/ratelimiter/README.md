# ratelimiter middleware

`ratelimiter` provides fixed-window rate limiting with a sharded in-memory store and standard rate-limit headers.

## Import

```go
import "github.com/oarkflow/fh/mw/ratelimiter"
```

## Basic usage

```go
app.Use(ratelimiter.New(ratelimiter.Config{
    Max: 100,
    Window: time.Minute,
}))
```

## Per-user or per-tenant key

```go
app.Use(ratelimiter.New(ratelimiter.Config{
    Max: 1000,
    Window: time.Minute,
    KeyFunc: func(c *fh.Ctx) string {
        tenant := c.Get("X-Tenant-ID")
        if tenant == "" {
            tenant = c.IP()
        }
        return tenant
    },
}))
```

## Route-specific limit

```go
app.Post("/login",
    ratelimiter.New(ratelimiter.Config{
        Max: 5,
        Window: time.Minute,
        KeyFunc: func(c *fh.Ctx) string { return "login:" + c.IP() },
    }),
    login,
)
```

## Custom rejection

```go
app.Use(ratelimiter.New(ratelimiter.Config{
    Max: 10,
    Window: time.Minute,
    LimitReached: func(c *fh.Ctx, r ratelimiter.Result) error {
        c.Set(ratelimiter.HeaderRetryAfter, strconv.Itoa(int(r.RetryAfter.Seconds())))
        return c.Status(429).JSON(fh.Map{"error": "rate_limited"})
    },
}))
```

## Response headers

- `X-RateLimit-Limit`
- `X-RateLimit-Remaining`
- `X-RateLimit-Reset`
- `Retry-After` when rejected

## Best practice

Use different keys and limits for unauthenticated IP traffic, authenticated users, tenants, login endpoints, and expensive endpoints.

# replay middleware

`replay` rejects duplicate nonce/key values within a TTL window. It is useful with signed requests and webhook verification.

## Import

```go
import "github.com/oarkflow/fh/mw/replay"
```

## Usage

```go
app.Post("/webhook",
    replay.New(replay.Config{
        Header: "X-Nonce",
        TTL: 5 * time.Minute,
    }),
    webhookHandler,
)
```

## Custom key

```go
app.Use(replay.New(replay.Config{
    TTL: 10 * time.Minute,
    Key: func(c *fh.Ctx) string {
        return c.Get("X-Tenant-ID") + ":" + c.Get("X-Nonce")
    },
}))
```

## Custom store

```go
store := replay.NewMemoryStore()
app.Use(replay.New(replay.Config{Store: store}))
```

## Best practice

Use a shared durable store if multiple server instances must coordinate replay protection. Pair with `signature` for HMAC-signed requests.

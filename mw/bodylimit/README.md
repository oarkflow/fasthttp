# bodylimit middleware

`bodylimit` rejects requests whose buffered body exceeds a configured byte limit.

## Import

```go
import "github.com/oarkflow/fh/mw/bodylimit"
```

## Global limit

```go
app.Use(bodylimit.New(16 << 20)) // 16 MiB
```

## Route-specific limit

```go
app.Post("/avatar",
    bodylimit.New(2 << 20),
    uploadAvatar,
)
```

## Skip selected requests

```go
app.Use(bodylimit.WithConfig(bodylimit.Config{
    Limit: 8 << 20,
    Next: func(c *fh.Ctx) bool {
        return c.Path() == "/webhooks/large-provider"
    },
}))
```

## Behavior

When the request body is too large, the middleware returns a `413 Payload Too Large` error.

## Best practice

Set both server-level body size limits and route-level middleware limits for sensitive endpoints. Put this middleware early, before JSON parsing or expensive authentication work.

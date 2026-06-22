# cache middleware

`cache` provides bounded in-memory response caching for safe responses. It is intended for GET/HEAD endpoints where a short TTL reduces repeated handler work.

## Import

```go
import "github.com/oarkflow/fh/mw/cache"
```

## Basic usage

```go
app.Get("/public/config",
    cache.New(cache.Config{TTL: 30 * time.Second}),
    func(c *fh.Ctx) error {
        return c.JSON(loadPublicConfig())
    },
)
```

## Custom cache key and vary headers

```go
app.Use(cache.New(cache.Config{
    TTL: 5 * time.Minute,
    MaxEntries: 4096,
    MaxBodySize: 256 << 10,
    VaryHeaders: []string{"Accept-Language"},
    KeyGenerator: func(c *fh.Ctx) string {
        return c.Method() + ":" + c.Path() + ":" + c.Query("page")
    },
}))
```

## Behavior

The middleware skips caching when:

- method is not configured, by default only `GET` and `HEAD`
- `Authorization` is present
- request cookies exist unless `AllowRequestCookies` is true
- request `Cache-Control` includes `no-cache` or `no-store`
- response sets cookies
- response `Cache-Control` includes `private` or `no-store`
- body exceeds `MaxBodySize`

It sets `X-Cache: HIT` or `X-Cache: MISS`.

## Best practice

Do not cache personalized or authorization-dependent responses unless your key includes the authenticated subject and you fully understand the data-leak risk.

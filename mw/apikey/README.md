# apikey middleware

`apikey` authenticates requests using an API key from a header or query parameter. Static keys are compared with `hmac.Equal`; dynamic lookup can be provided for database-backed keys.

## Import

```go
import "github.com/oarkflow/fh/mw/apikey"
```

## Header key usage

```go
app.Use(apikey.New(apikey.Config{
    Header: "X-API-Key",
    Keys: []string{"dev-secret-key"},
}))
```

## Database/dynamic lookup

```go
app.Use(apikey.New(apikey.Config{
    Header: "X-API-Key",
    Lookup: func(c *fh.Ctx, key string) bool {
        // Look up a hashed key in your database/cache.
        return key == "service-key-1"
    },
}))
```

## Route-specific usage

```go
app.Post("/internal/jobs",
    apikey.New(apikey.Config{Keys: []string{os.Getenv("INTERNAL_API_KEY")}}),
    createJob,
)
```

## Custom error

```go
app.Use(apikey.New(apikey.Config{
    Keys: []string{"secret"},
    Error: func(c *fh.Ctx) error {
        return c.Status(fh.StatusUnauthorized).JSON(fh.Map{"error": "api_key_required"})
    },
}))
```

## Best practice

Prefer header keys over query keys because URLs are commonly logged. Store only hashed API keys server-side and rotate keys periodically.

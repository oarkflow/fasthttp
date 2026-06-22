# requestid middleware

`requestid` validates and propagates an incoming request ID or generates a new one. It stores the ID in locals and sets a response header.

## Import

```go
import "github.com/oarkflow/fh/mw/requestid"
```

## Basic usage

```go
app.Use(requestid.New())
```

## Custom header and generator

```go
gen := requestid.NewAtomicGeneratorWithPrefix("api")

app.Use(requestid.New(requestid.Config{
    Header: "X-Request-ID",
    Generator: gen,
    TrustIncoming: true,
}))
```

## Custom validator

```go
app.Use(requestid.New(requestid.Config{
    Validator: func(id string) bool {
        return len(id) >= 8 && len(id) <= 128 && requestid.DefaultValidator(id)
    },
}))
```

## Handler usage

```go
app.Get("/", func(c *fh.Ctx) error {
    return c.JSON(fh.Map{"request_id": c.Locals("request_id")})
})
```

## Best practice

Register request IDs before logger, metrics, auth, and proxy middleware so all downstream components can reference the same ID.

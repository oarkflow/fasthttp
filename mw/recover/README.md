# recover middleware

`recover` catches panics, logs them, and converts them into controlled errors instead of crashing the process.

## Import

```go
import recovermw "github.com/oarkflow/fh/mw/recover"
```

The alias avoids colliding with Go's built-in `recover` function.

## Basic usage

```go
app.Use(recovermw.New())
```

## Custom handler

```go
app.Use(recovermw.New(recovermw.Config{
    EnableStackTrace: true,
    StackTraceLimit: 32 << 10,
    Handler: func(c *fh.Ctx, recovered any, stack []byte) error {
        return c.Status(500).JSON(fh.Map{
            "error": "internal_error",
            "request_id": c.Locals("request_id"),
        })
    },
}))
```

## Best practice

Register `recover` as the first middleware. In production, return generic errors to clients and send detailed stack traces only to logs.

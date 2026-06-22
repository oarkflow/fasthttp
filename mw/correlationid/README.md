# correlationid middleware

`correlationid` propagates a correlation ID through request and response headers. It is useful for linking client logs, proxy logs, app logs, and downstream calls.

## Import

```go
import "github.com/oarkflow/fh/mw/correlationid"
```

## Usage

```go
app.Use(correlationid.New(correlationid.Config{
    Header: "X-Correlation-ID",
}))
```

Inside a handler:

```go
app.Get("/debug", func(c *fh.Ctx) error {
    return c.JSON(fh.Map{
        "correlation_id": c.Locals("correlationID"),
    })
})
```

## Behavior

- Uses the incoming correlation header when present.
- Generates a value when missing if the package configuration supports generation.
- Sets the response header so clients can report it back.

## Best practice

Use `requestid` for per-hop request identity and `correlationid` for end-to-end workflow identity.

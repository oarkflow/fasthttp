# timeout middleware

`timeout` adds a request context deadline and returns a timeout response if the deadline is exceeded before a response is committed.

## Import

```go
import "github.com/oarkflow/fh/mw/timeout"
```

## Usage

```go
app.Use(timeout.New(5 * time.Second))
```

## Route-specific timeout

```go
app.Get("/reports/:id",
    timeout.New(30 * time.Second),
    generateReport,
)
```

## Handler cancellation

```go
app.Get("/slow", func(c *fh.Ctx) error {
    select {
    case <-time.After(10 * time.Second):
        return c.SendString("done")
    case <-c.Context().Done():
        return c.Context().Err()
    }
})
```

## Best practice

Handlers must respect `c.Context().Done()` for timeouts to stop work early. Also configure server-level read/write timeouts.

# idempotency middleware

`idempotency` computes and stores an idempotency key in request locals. It is intentionally lightweight and works well with the core reliability layer.

## Import

```go
import "github.com/oarkflow/fh/mw/idempotency"
```

## Usage

```go
app.Post("/payments",
    idempotency.New(func(c *fh.Ctx) string {
        return c.Get("X-Tenant-ID") + ":payment:" + c.Get("X-External-ID")
    }),
    func(c *fh.Ctx) error {
        key := c.Locals("idempotency_key")
        return c.JSON(fh.Map{"idempotency_key": key})
    },
)
```

## Best practice

Use deterministic idempotency for externally retried operations such as payments, orders, invoice creation, email sending, and webhook processing. Combine this middleware with `mw/reliability` when response replay or body-drift conflict detection is required.

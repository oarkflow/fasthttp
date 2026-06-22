# actor middleware

`actor` serializes request execution for the same computed key. It is useful when requests for one user, account, tenant, order, or resource must not run concurrently in the same process.

## Import

```go
import "github.com/oarkflow/fh/mw/actor"
```

## Basic usage

```go
app.Post("/accounts/:id/transfer",
    actor.New(actor.Config{
        Key: func(c *fh.Ctx) string {
            return "account:" + c.Param("id")
        },
    }),
    func(c *fh.Ctx) error {
        return c.JSON(fh.Map{"status": "processed"})
    },
)
```

## Behavior

- Empty keys bypass serialization.
- Locks are process-local; use database locks or distributed locks if you need cross-process serialization.
- Keep handlers short because all requests with the same key wait for the previous request to finish.

## Best practice

Use this for high-value critical sections such as wallet mutation, per-tenant config updates, or single-resource state transitions. Do not use one global key for all traffic.

# reliability middleware

`reliability` connects routes to the core `fh` reliability layer. It supports per-route policies and typed reliable endpoints.

## Import

```go
import "github.com/oarkflow/fh/mw/reliability"
```

## Route policy

```go
app.Post("/orders",
    reliability.New(fh.ReliabilityPolicy{
        Enabled: true,
        RequireIdempotency: true,
        Journal: true,
        ReplayResponse: true,
        ConflictOnBodyDrift: true,
        MaxReplayAge: 24 * time.Hour,
    }),
    createOrder,
)
```

## Typed reliable endpoint

```go
type CreateEmail struct {
    To string `json:"to"`
    Subject string `json:"subject"`
}

type Accepted struct {
    JobID string `json:"job_id"`
}

app.Post("/emails", reliability.Endpoint[CreateEmail, Accepted](reliability.EndpointOptions[CreateEmail, Accepted]{
    Policy: fh.ReliabilityPolicy{Enabled: true, RequireIdempotency: true, Journal: true},
    Validate: func(c *fh.Ctx, req *CreateEmail) error {
        if req.To == "" { return fh.NewHTTPError(422, "EMAIL_REQUIRED", "email is required") }
        return nil
    },
    Handle: func(ctx context.Context, c *fh.Ctx, req CreateEmail) (Accepted, error) {
        return Accepted{JobID: "sync-result"}, nil
    },
}))
```

## Async endpoint

```go
app.Post("/emails", reliability.Endpoint[CreateEmail, fh.Map](reliability.EndpointOptions[CreateEmail, fh.Map]{
    Policy: fh.ReliabilityPolicy{Enabled: true, RequireIdempotency: true, Journal: true},
    Async: true,
    QueueType: "email.send",
}))
```

## Best practice

Use reliability for mutation endpoints that clients may retry: payments, orders, webhooks, email, inventory updates, and external provider callbacks.

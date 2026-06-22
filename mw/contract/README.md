# contract middleware

`contract` enforces request contract rules such as allowed methods, required content types, required headers, and body size limits.

## Import

```go
import "github.com/oarkflow/fh/mw/contract"
```

## Usage

```go
app.Post("/users",
    contract.New(contract.Config{
        Methods: []string{"POST"},
        ContentTypes: []string{"application/json"},
        RequireHeaders: []string{"X-Tenant-ID"},
        MaxBodyBytes: 1 << 20,
    }),
    createUser,
)
```

## Behavior

- Unsupported methods return `405 Method Not Allowed`.
- Bodies larger than `MaxBodyBytes` return `413 Payload Too Large`.
- Content types are matched by prefix, so `application/json; charset=utf-8` matches `application/json`.
- Missing required headers return `400 Bad Request`.

## Best practice

Use this on typed API endpoints, JSON-only endpoints, webhook endpoints, and routes where strict input contracts help avoid ambiguous parsing.

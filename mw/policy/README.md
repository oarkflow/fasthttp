# policy middleware

`policy` groups cross-cutting route metadata such as data sensitivity with API version enforcement.

## Import

```go
import "github.com/oarkflow/fh/mw/policy"
```

## Usage

```go
app.Use(policy.New(policy.Config{
    Data: fh.DataPolicy{
        Sensitivity: "pii",
        Retention: "90d",
    },
    Version: apiversion.Config{
        Default: "2026-06-01",
        Supported: []string{"2026-06-01"},
    },
}))
```

Inside a handler:

```go
if p, ok := c.Locals("fh.data_policy").(fh.DataPolicy); ok {
    log.Println("sensitivity", p.Sensitivity)
}
```

## Best practice

Use data policies to make logging, auditing, redaction, retention, and compliance behavior explicit at route/group level.

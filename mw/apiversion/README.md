# apiversion middleware

`apiversion` reads an API version from a request header, applies a default version, rejects unsupported versions, and emits deprecation metadata.

## Import

```go
import "github.com/oarkflow/fh/mw/apiversion"
```

## Usage

```go
app.Use(apiversion.New(apiversion.Config{
    Header:  "Accept-Version",
    Default: "2026-06-01",
    Supported: []string{
        "2026-01-01",
        "2026-06-01",
    },
    Deprecated: map[string]string{
        "2026-01-01": "2026-12-31",
    },
}))
```

Inside a handler:

```go
app.Get("/users", func(c *fh.Ctx) error {
    version, _ := c.Locals("api_version").(string)
    return c.JSON(fh.Map{"version": version})
})
```

## Response headers for deprecated versions

When a request uses a deprecated version, the middleware sets:

- `Deprecation: true`
- `Sunset: <configured date>`

## Best practice

Use date-based API versions for long-lived public APIs and keep OpenAPI schemas aligned per version.

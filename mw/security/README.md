# security middleware

`security` sets hardened response security headers.

## Import

```go
import "github.com/oarkflow/fh/mw/security"
```

## Basic usage

```go
app.Use(security.New())
```

Default headers include hardened frame, content-type, referrer, cross-origin, permissions, and HSTS settings.

## Custom policy

```go
app.Use(security.New(security.Config{
    ContentSecurityPolicy: "default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'self'",
    HSTSMaxAge: 31536000,
    HSTSIncludeSubDomains: true,
    HSTSPreload: true,
    FrameDeny: true,
    ContentTypeNosniff: true,
    ReferrerPolicy: "no-referrer",
    CrossOriginOpenerPolicy: "same-origin",
    CrossOriginResourcePolicy: "same-origin",
    CrossOriginEmbedderPolicy: "require-corp",
    PermissionsPolicy: "camera=(), microphone=(), geolocation=()",
}))
```

## Headers managed

- `Content-Security-Policy`
- `Strict-Transport-Security`
- `X-Frame-Options`
- `X-Content-Type-Options`
- `X-XSS-Protection`
- `Referrer-Policy`
- `Cross-Origin-Opener-Policy`
- `Cross-Origin-Resource-Policy`
- `Cross-Origin-Embedder-Policy`
- `Permissions-Policy`

## Best practice

Use a strict CSP in production and test it carefully with your frontend assets. Only enable HSTS preload when every subdomain is HTTPS-ready.

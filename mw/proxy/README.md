# proxy middleware

`proxy` provides a reverse proxy and a simple prefix-based gateway using the Go standard library reverse proxy underneath.

## Import

```go
import "github.com/oarkflow/fh/mw/proxy"
```

## Single upstream

```go
app.Use("/users", proxy.New(proxy.Config{
    Target: "http://localhost:8081",
    StripPrefix: "/users",
    AddPrefix: "/api/users",
    Timeout: 2 * time.Second,
}))
```

## Gateway routing

```go
app.Use(proxy.Gateway(map[string]proxy.Config{
    "/users": {
        Target: "http://users-service:8080",
        Timeout: 2 * time.Second,
    },
    "/billing": {
        Target: "http://billing-service:8080",
        Timeout: 3 * time.Second,
    },
}))
```

## Header customization

```go
app.Use("/api", proxy.New(proxy.Config{
    Target: "http://upstream:8080",
    Director: func(r *http.Request) {
        r.Header.Set("X-Forwarded-Host", r.Host)
        r.Header.Set("X-Gateway", "fh")
    },
}))
```

## Best practice

Set timeouts. Add request IDs before the proxy. For public edge deployments, combine proxy with body limits, rate limits, circuit breakers, and strict trusted-proxy handling.

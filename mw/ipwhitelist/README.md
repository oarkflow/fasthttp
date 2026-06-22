# ipwhitelist middleware

`ipwhitelist` enforces IP/CIDR allowlists and blocklists. It supports in-memory stores, custom stores, trusted proxy handling, and custom forbidden responses.

## Import

```go
import "github.com/oarkflow/fh/mw/ipwhitelist"
```

## Simple allowlist

```go
app.Use(ipwhitelist.New("127.0.0.1", "10.0.0.0/8", "192.168.1.0/24"))
```

## Allowlist and blocklist

```go
store := ipwhitelist.MustMemoryStore("10.0.0.0/8")

app.Use(ipwhitelist.NewWithConfig(ipwhitelist.Config{
    Store: store,
    Blocked: []string{"10.0.4.20", "203.0.113.0/24"},
    TrustProxy: true,
    Forbidden: func(c *fh.Ctx) error {
        return c.Status(fh.StatusForbidden).JSON(fh.Map{"error": "ip_forbidden"})
    },
}))
```

## Custom key function

```go
app.Use(ipwhitelist.NewWithConfig(ipwhitelist.Config{
    Store: ipwhitelist.MustMemoryStore("198.51.100.10"),
    KeyFunc: func(c *fh.Ctx) string {
        return c.Get("X-Real-IP")
    },
}))
```

## Best practice

Only trust forwarded IP headers when the request comes through your own reverse proxy/load balancer. For public apps, prefer CIDR ranges from infrastructure and partner systems rather than individual dynamic IPs.

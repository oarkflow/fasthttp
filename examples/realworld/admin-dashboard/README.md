# Admin Dashboard Example

Demonstrates protected admin endpoints using middleware.

## Features

- panic recovery middleware
- security headers middleware
- fixed-window rate limiter
- Basic Auth protected `/admin` group
- queue stats and audit locations

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i -u admin:admin123 http://localhost:3000/admin/
curl -i -u admin:admin123 http://localhost:3000/admin/queue
```

Use stronger credentials and TLS in production.

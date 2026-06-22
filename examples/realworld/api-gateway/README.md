# API Gateway Example

Demonstrates using `fh` as a lightweight API gateway/edge service.

## Features

- recovery middleware
- security headers
- CORS middleware
- rate limiting
- rewrite from `/v1/*` to `/api/v1/*`
- route groups
- request journal

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i http://localhost:3000/health
curl -i http://localhost:3000/v1/users/42
curl -i -X POST http://localhost:3000/api/v1/orders -d '{}'
```

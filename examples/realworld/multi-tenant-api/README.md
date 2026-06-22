# Multi-Tenant API Example

Demonstrates tenant-scoped request handling and queue jobs.

## Features

- middleware requires `X-Tenant-ID`
- tenant ID stored in `Ctx.Locals`
- route groups
- idempotent project creation
- tenant-scoped report job

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i -X POST http://localhost:3000/api/projects \
  -H 'Content-Type: application/json' \
  -H 'X-Tenant-ID: tenant_a' \
  -H 'Idempotency-Key: tenant-a-project-001' \
  -d '{"name":"Migration"}'

curl -i -X POST http://localhost:3000/api/projects/prj_1/report \
  -H 'X-Tenant-ID: tenant_a' \
  -H 'Idempotency-Key: tenant-a-report-001'
```

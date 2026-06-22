# CSV Importer Example

Demonstrates multipart upload plus durable background processing for large or slow imports.

## Features

- HTML upload form at `/`
- `POST /imports/users` receives a CSV file
- request body limit middleware
- file is saved to `uploads/` before queueing
- durable queue worker counts rows using `encoding/csv`

## Run

```bash
go run . -addr :3000
```

## Test

```bash
printf 'name,email\nAlice,a@example.com\nBob,b@example.com\n' > users.csv
curl -i -X POST http://localhost:3000/imports/users \
  -H 'Idempotency-Key: import-001' \
  -F 'file=@users.csv'

curl http://localhost:3000/queue/stats
```

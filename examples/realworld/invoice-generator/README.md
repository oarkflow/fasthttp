# Invoice Generator Example

Demonstrates asynchronous generated-file workflows.

## Features

- `POST /invoices` accepts invoice data
- durable queue worker generates invoice text file
- static file serving exposes generated invoices under `/files/invoices/...`

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i -X POST http://localhost:3000/invoices \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: inv-001' \
  -d '{"customer":"Acme Ltd","amount":12500}'
```

Open the returned `download` URL after the queue worker completes.

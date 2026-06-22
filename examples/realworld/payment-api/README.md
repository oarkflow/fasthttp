# Payment API Example

Demonstrates a payment-style API where idempotency is mandatory for unsafe operations.

## Features

- required `Idempotency-Key` on `POST /payments`
- replay-safe payment intent response
- webhook payload queued for durable processing

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i -X POST http://localhost:3000/payments \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: pay-create-001' \
  -d '{"customer_id":"cust_1","amount":5000,"currency":"NPR"}'
```

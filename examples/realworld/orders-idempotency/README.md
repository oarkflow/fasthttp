# Orders Idempotency Example

Demonstrates safe order creation where client retries cannot create duplicate orders.

## Features

- requires `Idempotency-Key` on `POST /orders`
- stores completed response for replay
- detects reused key with different request body and returns `409 Conflict`
- writes request journal records

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i -X POST http://localhost:3000/orders \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: order-001' \
  -d '{"customer":"cust_1","items":[{"sku":"A1","qty":2}]}'
```

Run the same command twice. The second response is replayed instead of creating a second order.

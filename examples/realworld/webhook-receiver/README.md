# Webhook Receiver Example

Demonstrates secure webhook ingestion with HMAC validation, body copy safety, idempotency, journal, and durable async processing.

## Features

- validates `X-Signature: sha256=<hex>` using the raw body
- parses event payload only after signature validation
- queues `webhook.process` jobs
- records request lifecycle and queue lifecycle

## Run

```bash
go run . -addr :3000 -secret dev-secret
```

## Test

```bash
body='{"id":"evt_001","type":"payment.succeeded","data":{"payment_id":"pay_1"}}'
sig=$(printf '%s' "$body" | openssl dgst -sha256 -hmac 'dev-secret' -hex | awk '{print $2}')
curl -i -X POST http://localhost:3000/webhooks/payment \
  -H 'Content-Type: application/json' \
  -H "X-Signature: sha256=$sig" \
  -H 'Idempotency-Key: evt_001' \
  -d "$body"
```

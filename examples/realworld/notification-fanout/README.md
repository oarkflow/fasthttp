# Notification Fanout Example

Demonstrates enqueueing multiple durable jobs from a single request.

## Features

- `POST /notifications` fans out to multiple channel jobs
- workers registered for `notify.email`, `notify.sms`, and `notify.websocket`
- idempotency prevents duplicate fanout on retry

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i -X POST http://localhost:3000/notifications \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: ntf-001' \
  -d '{"user_id":"user_1","message":"Build completed","channels":["email","sms","websocket"]}'
```

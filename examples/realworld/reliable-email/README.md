# Reliable Email Example

Demonstrates durable async request handling with request journal, idempotency, and the embedded queue.

## Features

- `POST /email` accepts an email request and returns `202 Accepted`
- `Idempotency-Key` prevents duplicate email job creation
- request journal tracks received/completed request lifecycle
- durable queue writes job state and queue event log under `.fh-data/queue`
- worker processes `email.send` jobs

## Run

```bash
go run . -addr :3000
```

## Test

```bash
curl -i -X POST http://localhost:3000/email \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: email-001' \
  -d '{"to":"user@example.com","subject":"Hello","message":"Queued safely"}'

curl http://localhost:3000/queue/stats
cat .fh-data/queue/events.jsonl
```

Reuse the same idempotency key with the same body to replay the response. Use a new key to enqueue another job.

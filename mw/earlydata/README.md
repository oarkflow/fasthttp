# earlydata middleware

`earlydata` rejects replay-prone unsafe requests sent with TLS 0-RTT early data.

## Import

```go
import "github.com/oarkflow/fh/mw/earlydata"
```

## Usage

```go
app.Use(earlydata.New())
```

## Custom configuration

```go
app.Use(earlydata.New(earlydata.Config{
    AllowMethods: []string{"GET", "HEAD", "OPTIONS", "TRACE"},
    AllowWithIdempotencyKey: true,
    IdempotencyHeader: "Idempotency-Key",
}))
```

## Behavior

- If `Early-Data: 1` is not present, the request continues.
- Methods in `AllowMethods` continue.
- If `AllowWithIdempotencyKey` is true, unsafe early-data requests with the configured idempotency header continue.
- Other unsafe early-data requests receive `425 Too Early`.

## Best practice

Enable this on public HTTPS APIs when a proxy or TLS terminator forwards the `Early-Data` header. Keep unsafe operations protected by idempotency keys.

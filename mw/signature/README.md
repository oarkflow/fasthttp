# signature middleware

`signature` verifies HMAC-SHA256 request signatures with timestamp tolerance, optional key IDs, secret rotation, and custom signed payloads.

## Import

```go
import "github.com/oarkflow/fh/mw/signature"
```

## Basic signed webhook

```go
app.Post("/webhooks/provider",
    signature.New(signature.Config{
        Secret: []byte(os.Getenv("WEBHOOK_SECRET")),
        SignatureHeader: "X-Signature",
        TimestampHeader: "X-Timestamp",
        Scheme: "sha256=",
        Tolerance: 5 * time.Minute,
    }),
    handleWebhook,
)
```

The default signed payload is:

```text
<timestamp>.<raw body>
```

## Multiple active secrets

```go
app.Use(signature.New(signature.Config{
    Secrets: [][]byte{
        []byte(os.Getenv("WEBHOOK_SECRET_CURRENT")),
        []byte(os.Getenv("WEBHOOK_SECRET_PREVIOUS")),
    },
}))
```

## Key ID resolver

```go
app.Use(signature.New(signature.Config{
    KeyIDHeader: "X-Key-ID",
    Resolve: func(c *fh.Ctx, keyID string) [][]byte {
        return lookupSecretsForKeyID(keyID)
    },
}))
```

## Custom payload

```go
app.Use(signature.New(signature.Config{
    Secret: []byte("secret"),
    SignedPayload: func(c *fh.Ctx, ts string) []byte {
        return []byte(c.Method() + "." + c.Path() + "." + ts + "." + string(c.Body()))
    },
}))
```

## Generating a compatible signature

```go
payload := []byte(timestamp + "." + string(body))
mac := hmac.New(sha256.New, secret)
mac.Write(payload)
sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
```

## Best practice

Combine `signature` with `replay` or a durable nonce store. Rotate secrets by accepting current and previous secrets during the transition window.

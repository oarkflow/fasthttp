# compress middleware

`compress` applies gzip response compression when the client accepts it and the response is compressible.

## Import

```go
import "github.com/oarkflow/fh/mw/compress"
```

## Basic usage

```go
app.Use(compress.New())
```

## Custom configuration

```go
app.Use(compress.New(compress.Config{
    Level: gzip.BestSpeed,
    MinSize: 1024,
    CompressibleTypes: []string{
        "text/plain",
        "text/html",
        "application/json",
        "application/javascript",
        "text/css",
    },
}))
```

## Behavior

- Checks `Accept-Encoding` for gzip support.
- Skips responses smaller than `MinSize`.
- Skips non-compressible content types.
- Adds `Content-Encoding: gzip` and `Vary: Accept-Encoding`.
- Uses response body transforms, so middleware should be registered before handlers that send normal buffered responses.

## Best practice

Do not compress already-compressed content such as images, archives, video, or encrypted blobs. Avoid compressing sensitive cross-origin responses when compression side-channel risk matters.

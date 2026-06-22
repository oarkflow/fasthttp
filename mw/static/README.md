# static middleware

`static` serves files safely from a root directory with path traversal protection, cache headers, ETag, Last-Modified, SPA fallback, and download controls.

## Import

```go
import staticmw "github.com/oarkflow/fh/mw/static"
```

The alias avoids confusion with the `static` keyword-like name in prose.

## Basic usage

```go
app.Get("/static/*", staticmw.New("./public", staticmw.Config{
    Prefix: "/static/",
    ETag: true,
    LastModified: true,
    MaxAge: time.Hour,
}))
```

## Immutable assets

```go
app.Get("/assets/*", staticmw.New("./dist/assets", staticmw.Config{
    Prefix: "/assets/",
    Immutable: true,
    ETag: true,
    LastModified: true,
}))
```

## SPA fallback

```go
app.Get("/*", staticmw.New("./dist", staticmw.Config{
    SPAFallback: "index.html",
    ETag: true,
    CacheControl: "no-cache",
}))
```

## File downloads

```go
app.Get("/downloads/*", staticmw.New("./files", staticmw.Config{
    Prefix: "/downloads/",
    Download: true,
}))
```

## Best practice

Disable directory browsing in production. Use immutable caching only for content-hashed filenames. Keep uploads outside the static root unless you explicitly validate and control served file types.

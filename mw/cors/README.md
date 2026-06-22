# cors middleware

`cors` handles Cross-Origin Resource Sharing for browser clients, including preflight requests, static origin lists, dynamic origin functions, and origin stores.

## Import

```go
import "github.com/oarkflow/fh/mw/cors"
```

## Basic usage

```go
app.Use(cors.New(cors.Config{
    AllowOrigins: []string{"https://app.example.com"},
    AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
    AllowHeaders: []string{"Authorization", "Content-Type", "X-Request-ID"},
    ExposeHeaders: []string{"X-Request-ID"},
    MaxAge: 86400,
}))
```

## Dynamic origins

```go
app.Use(cors.New(cors.Config{
    AllowOriginFunc: func(c *fh.Ctx, origin string) bool {
        return strings.HasSuffix(origin, ".example.com")
    },
    AllowCredentials: true,
}))
```

## Origin store

```go
store := cors.NewMemoryOriginStore("https://admin.example.com")
app.Use(cors.New(cors.Config{OriginStore: store}))
```

## Best practice

Never use wildcard origins with credentials. Keep CORS as narrow as possible and place it before auth middleware so preflight requests do not fail authentication.

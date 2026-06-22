# logger middleware

`logger` provides high-performance async access logging with text formats, JSON format, `slog`, custom writers, queue limits, skip rules, and dropped-log accounting.

## Import

```go
import "github.com/oarkflow/fh/mw/logger"
```

## Basic usage

```go
app.Use(logger.New())
```

## JSON logs

```go
app.Use(logger.New(logger.Config{
    FormatName: "json",
    QueueSize: 8192,
    MaxLineBytes: 4096,
}))
```

## Common log format

```go
app.Use(logger.New(logger.Config{FormatName: "common"}))
```

## Custom format

Supported tokens include `${time}`, `${ip}`, `${method}`, `${path}`, `${query}`, `${uri}`, `${status}`, `${latency}`, and `${error}`.

```go
app.Use(logger.New(logger.Config{
    Format: "${time} ${ip} ${method} ${uri} ${status} ${latency} ${error}\n",
}))
```

## Skip noisy traffic

```go
app.Use(logger.New(logger.Config{
    SkipPrefixes: []string{"/_fh/metrics", "/static/"},
    SkipStatusCodes: []int{304},
}))
```

## Managed shutdown

```go
lm := logger.NewMiddleware(logger.Config{FormatName: "json"})
app.Use(lm.Handler())
defer lm.Close()
```

## Best practice

Use async mode in production, keep log lines bounded, and enable request/correlation IDs before the logger so IDs can be included by downstream logging hooks.

# skip middleware

`skip` wraps another middleware and skips it when a predicate matches. It also provides a rich predicate toolkit.

## Import

```go
import "github.com/oarkflow/fh/mw/skip"
```

## Basic usage

```go
app.Use(skip.New(logger.New(), skip.Prefixes("/static/", "/_fh/metrics")))
```

## Multiple predicates

```go
app.Use(skip.NewWithConfig(compress.New(), skip.Config{
    Logic: skip.LogicAny,
    Predicates: []skip.Predicate{
        skip.Static(),
        skip.Health(),
        skip.Methods("HEAD"),
    },
}))
```

## Predicate examples

```go
skip.Paths("/health", "/ready")
skip.Prefixes("/assets/", "/static/")
skip.Suffixes(".png", ".jpg")
skip.Globs("/public/*")
skip.Methods("OPTIONS")
skip.HeaderExists("Authorization")
skip.QueryEquals("debug", "true")
skip.SafeMethods()
skip.Not(skip.Static())
skip.Any(skip.Health(), skip.Static())
skip.All(skip.Methods("GET"), skip.Prefixes("/public/"))
```

## Best practice

Use `skip` to keep middleware composition explicit instead of adding skip logic inside every middleware configuration.

# rewrite middleware

`rewrite` rewrites request paths using ordered rules. Rules can match by path pattern and can be constrained by method and host.

## Import

```go
import "github.com/oarkflow/fh/mw/rewrite"
```

## Basic usage

```go
app.Use(rewrite.New(
    rewrite.Rule{From: "/old", To: "/new"},
    rewrite.Rule{From: "/v1/users/:id", To: "/users/:id"},
))
```

## With host and method constraints

```go
app.Use(rewrite.WithConfig(rewrite.Config{
    Rules: []rewrite.Rule{
        {
            From: "/api/:tenant/*",
            To: "/tenants/:tenant/*",
            Methods: []string{"GET", "POST"},
            Host: "*.internal.example.com",
        },
    },
}))
```

## Best practice

Place rewrite before route matching or route groups that depend on the rewritten path. Keep rewrite rules deterministic and avoid broad catch-all rules before specific rules.

# basicauth middleware

`basicauth` provides Basic Auth with constant-time password verification, optional HTTPS enforcement, storage backends, reloadable user files, PBKDF2 helpers, and authenticated principal hooks.

## Import

```go
import "github.com/oarkflow/fh/mw/basicauth"
```

## Simple usage

```go
app.Use(basicauth.New("admin", "change-me"))
```

## Production-style memory storage

```go
store := basicauth.NewMemoryStorage()
store.Set(basicauth.User{
    Username: "admin",
    PasswordHash: basicauth.MustHashPassword("strong-password"),
    Roles: []string{"admin"},
})

app.Use(basicauth.NewWithConfig(basicauth.Config{
    Realm: "admin",
    Storage: store,
    RequireHTTPS: true,
    TrustProxy: true,
    OnAuthenticated: func(c *fh.Ctx, p basicauth.Principal) error {
        c.Locals("user", p.Username)
        c.Locals("roles", p.Roles)
        return nil
    },
}))
```

## CSV or JSON users

```go
csvStore, err := basicauth.NewCSVStorage("./users.csv")
if err != nil { log.Fatal(err) }
app.Use(basicauth.NewWithConfig(basicauth.Config{Storage: csvStore}))
```

```go
jsonStore, err := basicauth.NewJSONStorage("./users.json")
if err != nil { log.Fatal(err) }
app.Use(basicauth.NewWithConfig(basicauth.Config{Storage: jsonStore}))
```

## User file format

CSV rows can include username, password hash, roles, scopes, and metadata depending on the helpers used. Generate password hashes with:

```go
hash := basicauth.MustHashPassword("password")
```

## Best practice

Use Basic Auth for internal tools, admin panels behind TLS, health dashboards, or temporary protection. For user-facing applications, prefer sessions, OAuth, or JWT-style authentication.

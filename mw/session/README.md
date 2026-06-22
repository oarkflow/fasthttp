# session middleware

`session` provides signed-cookie session management with memory and file stores, session rotation, regeneration, destroy, flash messages, and secure cookie options.

## Import

```go
import "github.com/oarkflow/fh/mw/session"
```

## Memory store

```go
store := session.NewMemoryStore(10 * time.Minute)
manager := session.NewSessionManager(store,
    session.SessionCookieName("__Host-session"),
    session.SessionSecret([]byte(os.Getenv("SESSION_SECRET"))),
    session.SessionMaxAge(24 * time.Hour),
    session.SessionSecure(true),
    session.SessionHTTPOnly(true),
    session.SessionSameSite(fh.SameSiteLax),
    session.SessionPath("/"),
)

app.Use(session.New(manager))
```

## File store

```go
store := session.NewFileStore("./data/sessions", 10 * time.Minute)
defer store.StopGC()

manager := session.NewSessionManager(store,
    session.SessionSecret([]byte(os.Getenv("SESSION_SECRET"))),
    session.SessionSecure(true),
)
app.Use(session.New(manager))
```

## Handler usage

```go
app.Post("/login", func(c *fh.Ctx) error {
    s := session.Get(c)
    s.Set("user_id", "u_123")
    return c.JSON(fh.Map{"status": "logged_in"})
})

app.Post("/logout", func(c *fh.Ctx) error {
    s := session.Get(c)
    return manager.Destroy(c, s)
})
```

## Flash messages

```go
s := session.Get(c)
s.Flash("notice", "Saved successfully")
```

## Best practice

Use at least 32 bytes of random secret material. Use `Secure`, `HTTPOnly`, `SameSite`, and `__Host-` cookie naming for HTTPS applications. Regenerate sessions after login or privilege changes.

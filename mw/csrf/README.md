# csrf middleware

`csrf` implements a signed-origin-aware double-submit cookie pattern for browser requests. It creates a CSRF token cookie, exposes the token through `Locals("csrf_token")`, and requires unsafe requests to send the same token in a header.

## Import

```go
import "github.com/oarkflow/fh/mw/csrf"
```

## Basic usage

```go
app.Use(csrf.New(csrf.Config{
    CookieName: "csrf_token",
    HeaderName: "X-CSRF-Token",
    CookiePath: "/",
    CookieSecure: true,
    CookieSameSite: fh.SameSiteLax,
    CookieMaxAge: 12 * time.Hour,
    TrustedOrigins: []string{"https://app.example.com"},
}))
```

## Rendering the token

```go
app.Get("/form", func(c *fh.Ctx) error {
    token, _ := c.Locals("csrf_token").(string)
    return c.Type("html").SendString(`<form method="post">
        <input type="hidden" name="csrf_token" value="` + token + `">
        <button>Save</button>
    </form>`)
})
```

For HTML forms, copy the token into the configured request header from JavaScript, or adapt your form handler to set `X-CSRF-Token`.

## Unsafe request example

```js
await fetch("/profile", {
  method: "POST",
  headers: { "X-CSRF-Token": tokenFromPage },
  credentials: "include",
  body: JSON.stringify({ name: "Alice" })
})
```

## Behavior

- Safe methods such as `GET`, `HEAD`, and `OPTIONS` are bypassed.
- A missing token cookie is generated automatically.
- Unsafe methods require an allowed origin and matching `HeaderName` token.

## Best practice

Use CSRF for cookie-authenticated browser sessions. API-key, bearer-token, and signed webhook routes usually do not need CSRF.

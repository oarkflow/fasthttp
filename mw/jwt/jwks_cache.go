package jwt

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/oarkflow/fh"
)

// JWKSCache is a small dependency-free kid -> key cache for JWT verification.
// It can be warmed with JWKS JSON or refreshed from a JWKS URL. Use it through
// KeyFunc so JWT remains authentication-only and authorization stays in fh core
// helpers or contrib/mw/authz.
type JWKSCache struct {
	mu        sync.RWMutex
	keys      map[string][]byte
	url       string
	ttl       time.Duration
	refreshed time.Time
	client    *http.Client
	now       func() time.Time
}

type JWKSCacheConfig struct {
	URL    string
	TTL    time.Duration
	Client *http.Client
	Now    func() time.Time
}

func NewJWKSCache(cfg JWKSCacheConfig) *JWKSCache {
	if cfg.TTL <= 0 {
		cfg.TTL = 5 * time.Minute
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 5 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &JWKSCache{keys: map[string][]byte{}, url: cfg.URL, ttl: cfg.TTL, client: cfg.Client, now: cfg.Now}
}

func (c *JWKSCache) Set(keys map[string][]byte) {
	if c == nil {
		return
	}
	cp := map[string][]byte{}
	for k, v := range keys {
		if k != "" && len(v) > 0 {
			cp[k] = append([]byte(nil), v...)
		}
	}
	c.mu.Lock()
	c.keys = cp
	c.refreshed = c.now()
	c.mu.Unlock()
}

func (c *JWKSCache) LoadJWKS(data []byte) error {
	keys, err := ParseJWKS(data)
	if err != nil {
		return err
	}
	c.Set(keys)
	return nil
}

func (c *JWKSCache) Key(kid string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	v, ok := c.keys[kid]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return append([]byte(nil), v...), true
}

func (c *JWKSCache) KeyFunc() KeyFunc {
	return func(ctx fh.Ctx, header map[string]any, claims map[string]any) ([]byte, error) {
		kid, _ := header["kid"].(string)
		if kid == "" {
			return nil, ErrMissingKID
		}
		if key, ok := c.Key(kid); ok {
			return key, nil
		}
		if c.url != "" {
			if err := c.Refresh(ctx.Context()); err != nil {
				return nil, err
			}
			if key, ok := c.Key(kid); ok {
				return key, nil
			}
		}
		return nil, ErrUnknownKID
	}
}

var ErrMissingKID = errString("jwt: missing kid")
var ErrUnknownKID = errString("jwt: unknown kid")

type errString string

func (e errString) Error() string { return string(e) }

func (c *JWKSCache) Refresh(ctx context.Context) error {
	if c == nil {
		return ErrUnknownKID
	}
	if c.url == "" {
		return nil
	}
	c.mu.RLock()
	fresh := !c.refreshed.IsZero() && c.now().Sub(c.refreshed) < c.ttl
	c.mu.RUnlock()
	if fresh {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	return c.LoadJWKS(b)
}

package jwt

import "testing"

func TestJWKSCacheLoadAndKeyFunc(t *testing.T) {
	jwks := []byte(`{"keys":[{"kty":"oct","kid":"k1","k":"c2VjcmV0"}]}`)
	cache := NewJWKSCache(JWKSCacheConfig{})
	if err := cache.LoadJWKS(jwks); err != nil {
		t.Fatal(err)
	}
	key, ok := cache.Key("k1")
	if !ok || string(key) != "secret" {
		t.Fatalf("bad key %q ok=%v", string(key), ok)
	}
}

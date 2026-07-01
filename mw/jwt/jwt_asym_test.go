package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestRS256SignVerifyAndRequiredClaims(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	tok, err := SignRS256(map[string]any{"sub": "u1", "exp": time.Now().Add(time.Hour).Unix()}, privPEM, "k1")
	if err != nil {
		t.Fatal(err)
	}
	_, claims, err := Verify(nil, tok, Config{PublicKeys: map[string][]byte{"k1": pubPEM}, Algorithms: []string{"RS256"}, RequiredClaims: []string{"sub"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "u1" {
		t.Fatalf("unexpected sub: %#v", claims["sub"])
	}
	_, _, err = Verify(nil, tok, Config{PublicKeys: map[string][]byte{"k1": pubPEM}, Algorithms: []string{"RS256"}, RequiredClaims: []string{"tenant_id"}}, nil)
	if err == nil {
		t.Fatal("expected missing claim error")
	}
}

package basicauth

import (
	"crypto/subtle"

	fh "github.com/oarkflow/fasthttp"
)

func New(username, password string) fh.HandlerFunc {
	expected := "Basic " + base64Encode(username+":"+password)
	return func(ctx *fh.Ctx) error {
		auth := ctx.Get("Authorization")
		if len(auth) != len(expected) || subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
			ctx.Set("WWW-Authenticate", `Basic realm="Restricted"`)
			return ctx.Status(401).SendString("Unauthorized")
		}
		return ctx.Next()
	}
}

const b64chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func base64Encode(s string) string {
	src := []byte(s)
	dst := make([]byte, (len(src)+2)/3*4)
	n := 0
	for i := 0; i < len(src); i += 3 {
		var b0, b1, b2 byte
		b0 = src[i]
		if i+1 < len(src) {
			b1 = src[i+1]
		}
		if i+2 < len(src) {
			b2 = src[i+2]
		}
		dst[n] = b64chars[b0>>2]
		dst[n+1] = b64chars[((b0&0x3)<<4)|b1>>4]
		dst[n+2] = b64chars[((b1&0xf)<<2)|b2>>6]
		dst[n+3] = b64chars[b2&0x3f]
		n += 4
	}
	switch len(src) % 3 {
	case 1:
		dst[n-2] = '='
		dst[n-1] = '='
	case 2:
		dst[n-1] = '='
	}
	return string(dst)
}

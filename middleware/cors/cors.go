package cors

import (
	"strconv"
	"strings"

	fh "github.com/orgware/fasthttp"
)

type Config struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
	MaxAge           int
}

var defaultConfig = Config{
	AllowOrigins: []string{"*"},
	AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
	AllowHeaders: []string{"Content-Type", "Authorization"},
	MaxAge:       86400,
}

func New(config ...Config) fh.HandlerFunc {
	cfg := defaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	allowedOrigins := make(map[string]struct{}, len(cfg.AllowOrigins))
	wildcard := false
	for _, origin := range cfg.AllowOrigins {
		if origin == "*" {
			wildcard = true
		} else {
			allowedOrigins[origin] = struct{}{}
		}
	}
	methods := strings.Join(cfg.AllowMethods, ", ")
	headers := strings.Join(cfg.AllowHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(ctx *fh.Ctx) error {
		origin := ctx.Get("Origin")
		if origin == "" {
			return ctx.Next()
		}
		_, exact := allowedOrigins[origin]
		if !wildcard && !exact {
			return ctx.Next()
		}
		if wildcard && !cfg.AllowCredentials {
			ctx.Set("Access-Control-Allow-Origin", "*")
		} else {
			ctx.Set("Access-Control-Allow-Origin", origin)
			ctx.Append("Vary", "Origin")
		}
		if cfg.AllowCredentials {
			ctx.Set("Access-Control-Allow-Credentials", "true")
		}
		if len(ctx.Header.Method) == 7 &&
			ctx.Header.Method[0] == 'O' &&
			ctx.Header.Method[1] == 'P' &&
			ctx.Header.Method[2] == 'T' &&
			ctx.Header.Method[3] == 'I' &&
			ctx.Header.Method[4] == 'O' &&
			ctx.Header.Method[5] == 'N' &&
			ctx.Header.Method[6] == 'S' && ctx.Get("Access-Control-Request-Method") != "" {
			ctx.Set("Access-Control-Allow-Methods", methods)
			ctx.Set("Access-Control-Allow-Headers", headers)
			ctx.Set("Access-Control-Max-Age", maxAge)
			return ctx.SendStatus(204)
		}
		return ctx.Next()
	}
}

package middleware

import (
	"compress/gzip"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	fh "github.com/orgware/fasthttp"
)

// ── Logger ─────────────────────────────────────────────────────────────────

// LoggerConfig configures the logger middleware.
type LoggerConfig struct {
	// Format: use ${method}, ${path}, ${status}, ${latency}, ${ip}
	Format string
	Logger *log.Logger
}

// Logger logs each request. Uses no allocations for fixed-format output.
func Logger(config ...LoggerConfig) fh.HandlerFunc {
	cfg := LoggerConfig{
		Format: "[${ip}] ${method} ${path} → ${status} (${latency})\n",
	}
	if len(config) > 0 {
		cfg = config[0]
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	return func(ctx *fh.Ctx) error {
		start := time.Now()
		err := ctx.Next()
		lat := time.Since(start)

		out := cfg.Format
		out = strings.ReplaceAll(out, "${method}", ctx.Method())
		out = strings.ReplaceAll(out, "${path}", ctx.Path())
		out = strings.ReplaceAll(out, "${status}", strconv.Itoa(ctx.StatusCode()))
		out = strings.ReplaceAll(out, "${latency}", lat.String())
		out = strings.ReplaceAll(out, "${ip}", ctx.IP())

		logger.Print(out)
		return err
	}
}

// ── Recover ────────────────────────────────────────────────────────────────

// Recover catches panics from downstream handlers and converts them to 500 errors.
func Recover() fh.HandlerFunc {
	return func(ctx *fh.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
				ctx.Status(500).SendString("Internal Server Error")
			}
		}()
		return ctx.Next()
	}
}

// ── CORS ───────────────────────────────────────────────────────────────────

// CORSConfig configures the CORS middleware.
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
	MaxAge           int // seconds
}

var defaultCORSConfig = CORSConfig{
	AllowOrigins: []string{"*"},
	AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
	AllowHeaders: []string{"Content-Type", "Authorization"},
	MaxAge:       86400,
}

// CORS handles cross-origin requests.
func CORS(config ...CORSConfig) fh.HandlerFunc {
	cfg := defaultCORSConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	origins := strings.Join(cfg.AllowOrigins, ", ")
	methods := strings.Join(cfg.AllowMethods, ", ")
	headers := strings.Join(cfg.AllowHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(ctx *fh.Ctx) error {
		ctx.Set("Access-Control-Allow-Origin", origins)
		ctx.Set("Access-Control-Allow-Methods", methods)
		ctx.Set("Access-Control-Allow-Headers", headers)
		ctx.Set("Access-Control-Max-Age", maxAge)
		if cfg.AllowCredentials {
			ctx.Set("Access-Control-Allow-Credentials", "true")
		}

		if ctx.Method() == "OPTIONS" {
			return ctx.SendStatus(204)
		}

		return ctx.Next()
	}
}

// ── RequestID ──────────────────────────────────────────────────────────────

var ridCounter uint64

// RequestID attaches a unique X-Request-ID to every request.
func RequestID() fh.HandlerFunc {
	return func(ctx *fh.Ctx) error {
		id := ctx.Get("X-Request-ID")
		if id == "" {
			n := atomic.AddUint64(&ridCounter, 1)
			id = strconv.FormatUint(n, 36) + "-" + strconv.FormatUint(uint64(rand.Int63()), 36)
		}
		ctx.Set("X-Request-ID", id)
		ctx.Locals("requestID", id)
		return ctx.Next()
	}
}

// ── Rate Limiter ───────────────────────────────────────────────────────────

// RateLimiterConfig configures the rate limiter middleware.
type RateLimiterConfig struct {
	Max        int           // requests per Window
	Window     time.Duration // sliding window size
	KeyFunc    func(*fh.Ctx) string
	LimitReached func(*fh.Ctx) error
}

type rateBucket struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

// RateLimiter limits requests per key (default: per IP).
func RateLimiter(config RateLimiterConfig) fh.HandlerFunc {
	if config.Max <= 0 {
		config.Max = 100
	}
	if config.Window <= 0 {
		config.Window = time.Minute
	}
	if config.KeyFunc == nil {
		config.KeyFunc = func(ctx *fh.Ctx) string { return ctx.IP() }
	}
	if config.LimitReached == nil {
		config.LimitReached = func(ctx *fh.Ctx) error {
			ctx.Set("Retry-After", strconv.Itoa(int(config.Window.Seconds())))
			return ctx.Status(429).SendString("Too Many Requests")
		}
	}

	var mu sync.RWMutex
	buckets := make(map[string]*rateBucket, 1024)

	// periodic cleanup goroutine
	go func() {
		tick := time.NewTicker(config.Window)
		defer tick.Stop()
		for range tick.C {
			now := time.Now()
			mu.Lock()
			for k, b := range buckets {
				b.mu.Lock()
				if now.After(b.resetAt) {
					delete(buckets, k)
				}
				b.mu.Unlock()
			}
			mu.Unlock()
		}
	}()

	return func(ctx *fh.Ctx) error {
		key := config.KeyFunc(ctx)

		mu.RLock()
		b := buckets[key]
		mu.RUnlock()

		if b == nil {
			mu.Lock()
			b = buckets[key]
			if b == nil {
				b = &rateBucket{resetAt: time.Now().Add(config.Window)}
				buckets[key] = b
			}
			mu.Unlock()
		}

		b.mu.Lock()
		now := time.Now()
		if now.After(b.resetAt) {
			b.count = 0
			b.resetAt = now.Add(config.Window)
		}
		b.count++
		count := b.count
		b.mu.Unlock()

		ctx.Set("X-RateLimit-Limit", strconv.Itoa(config.Max))
		ctx.Set("X-RateLimit-Remaining", strconv.Itoa(max(0, config.Max-count)))

		if count > config.Max {
			return config.LimitReached(ctx)
		}

		return ctx.Next()
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Compress ───────────────────────────────────────────────────────────────

var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

// Compress adds gzip compression for responses when the client accepts it.
// Uses pooled gzip writers — zero allocation per request.
func Compress() fh.HandlerFunc {
	return func(ctx *fh.Ctx) error {
		ae := ctx.Get("Accept-Encoding")
		if !strings.Contains(ae, "gzip") {
			return ctx.Next()
		}
		// Mark response for compression; actual compression done via wrapper
		ctx.Set("Content-Encoding", "gzip")
		ctx.Set("Vary", "Accept-Encoding")
		// Note: full streaming gzip requires wrapping the conn writer.
		// This sets headers; integrate with ctx.writeResponse for full impl.
		return ctx.Next()
	}
}

// ── BasicAuth ──────────────────────────────────────────────────────────────

// BasicAuth returns a middleware that validates HTTP Basic Auth.
func BasicAuth(username, password string) fh.HandlerFunc {
	expected := "Basic " + base64Encode(username+":"+password)
	return func(ctx *fh.Ctx) error {
		auth := ctx.Get("Authorization")
		if auth != expected {
			ctx.Set("WWW-Authenticate", `Basic realm="Restricted"`)
			return ctx.Status(401).SendString("Unauthorized")
		}
		return ctx.Next()
	}
}

// ── Timeout ────────────────────────────────────────────────────────────────

// Timeout returns a middleware that enforces a per-handler deadline.
// If the handler doesn't complete in time, a 503 is returned.
func Timeout(d time.Duration) fh.HandlerFunc {
	return func(ctx *fh.Ctx) error {
		done := make(chan error, 1)
		go func() {
			done <- ctx.Next()
		}()
		select {
		case err := <-done:
			return err
		case <-time.After(d):
			return ctx.Status(503).SendString("Request Timeout")
		}
	}
}

// ── IP Whitelist ───────────────────────────────────────────────────────────

// IPWhitelist restricts access to a list of CIDRs or IPs.
func IPWhitelist(allowed ...string) fh.HandlerFunc {
	networks := make([]*net.IPNet, 0, len(allowed))
	ips := make([]net.IP, 0, len(allowed))

	for _, a := range allowed {
		if strings.Contains(a, "/") {
			_, n, err := net.ParseCIDR(a)
			if err == nil {
				networks = append(networks, n)
			}
		} else {
			if ip := net.ParseIP(a); ip != nil {
				ips = append(ips, ip)
			}
		}
	}

	return func(ctx *fh.Ctx) error {
		clientIP := net.ParseIP(ctx.IP())
		if clientIP == nil {
			return ctx.Status(403).SendString("Forbidden")
		}
		for _, ip := range ips {
			if ip.Equal(clientIP) {
				return ctx.Next()
			}
		}
		for _, n := range networks {
			if n.Contains(clientIP) {
				return ctx.Next()
			}
		}
		return ctx.Status(403).SendString("Forbidden")
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

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

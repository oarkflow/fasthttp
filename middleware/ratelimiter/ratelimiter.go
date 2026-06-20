package ratelimiter

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	fh "github.com/oarkflow/fasthttp"
)

type Config struct {
	Max          int
	Window       time.Duration
	KeyFunc      func(*fh.Ctx) string
	LimitReached func(*fh.Ctx) error
}

type rateBucket struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

func New(config Config) fh.HandlerFunc {
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

	maxStr := strconv.Itoa(config.Max)

	var mu sync.RWMutex
	buckets := make(map[string]*rateBucket, 1024)
	var nextCleanup atomic.Int64
	nextCleanup.Store(time.Now().Add(config.Window).UnixNano())

	return func(ctx *fh.Ctx) error {
		now := time.Now()
		if deadline := nextCleanup.Load(); now.UnixNano() >= deadline && nextCleanup.CompareAndSwap(deadline, now.Add(config.Window).UnixNano()) {
			mu.Lock()
			for k, b := range buckets {
				b.mu.Lock()
				expired := now.After(b.resetAt)
				b.mu.Unlock()
				if expired {
					delete(buckets, k)
				}
			}
			mu.Unlock()
		}
		key := config.KeyFunc(ctx)

		mu.RLock()
		b := buckets[key]
		mu.RUnlock()

		if b == nil {
			mu.Lock()
			b = buckets[key]
			if b == nil {
				b = &rateBucket{resetAt: now.Add(config.Window)}
				buckets[key] = b
			}
			mu.Unlock()
		}

		b.mu.Lock()
		if now.After(b.resetAt) {
			b.count = 0
			b.resetAt = now.Add(config.Window)
		}
		b.count++
		count := b.count
		b.mu.Unlock()

		ctx.Set("X-RateLimit-Limit", maxStr)
		rem := config.Max - count
		if rem < 0 {
			rem = 0
		}
		var remBuf [10]byte
		remStr := string(strconv.AppendInt(remBuf[:0], int64(rem), 10))
		ctx.Set("X-RateLimit-Remaining", remStr)

		if count > config.Max {
			return config.LimitReached(ctx)
		}
		return ctx.Next()
	}
}

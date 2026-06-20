package fasthttp

import (
	"errors"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// ── Lifecycle events ───────────────────────────────────────────────────────

// HookFunc is a lifecycle hook with optional error propagation.
type HookFunc func() error

// Hooks groups all application lifecycle hooks.
type Hooks struct {
	onListen   []HookFunc
	onShutdown []HookFunc
	onConnect  []func(net.Conn)
	onClose    []func(net.Conn)
	onError    []func(error)
}

// ── Config ─────────────────────────────────────────────────────────────────

// Config holds server configuration.
type Config struct {
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	MaxConnections  int
	ReadBufferSize  int
	DisableKeepAlive bool
	ErrorHandler    func(*Ctx, error)
	Logger          *log.Logger
}

var defaultConfig = Config{
	ReadTimeout:    10 * time.Second,
	WriteTimeout:   10 * time.Second,
	IdleTimeout:    60 * time.Second,
	ReadBufferSize: 16384,
}

// ── App ────────────────────────────────────────────────────────────────────

// App is the top-level application object. Create with New().
type App struct {
	cfg       Config
	router    *Router
	hooks     Hooks
	logger    *log.Logger
	middleware []HandlerFunc
	sem       chan struct{}
	listener  net.Listener
	activeConn sync.WaitGroup
	closed    atomic.Bool
	groups    []*Group
}

// New creates a new App with optional config.
func New(config ...Config) *App {
	cfg := defaultConfig
	if len(config) > 0 {
		c := config[0]
		if c.ReadTimeout > 0 {
			cfg.ReadTimeout = c.ReadTimeout
		}
		if c.WriteTimeout > 0 {
			cfg.WriteTimeout = c.WriteTimeout
		}
		if c.IdleTimeout > 0 {
			cfg.IdleTimeout = c.IdleTimeout
		}
		if c.ReadBufferSize > 0 {
			cfg.ReadBufferSize = c.ReadBufferSize
		}
		if c.MaxConnections > 0 {
			cfg.MaxConnections = c.MaxConnections
		}
		cfg.DisableKeepAlive = c.DisableKeepAlive
		cfg.ErrorHandler = c.ErrorHandler
		cfg.Logger = c.Logger
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultErrorHandler
	}

	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	app := &App{
		cfg:    cfg,
		router: newRouter(),
		logger: logger,
	}

	if cfg.MaxConnections > 0 {
		app.sem = make(chan struct{}, cfg.MaxConnections)
	}

	return app
}

// ── Routing methods ────────────────────────────────────────────────────────

func (a *App) Add(method, path string, handlers ...HandlerFunc) *App {
	a.router.Add(method, path, a.chain(handlers))
	return a
}

func (a *App) Get(path string, handlers ...HandlerFunc) *App {
	return a.Add("GET", path, handlers...)
}

func (a *App) Post(path string, handlers ...HandlerFunc) *App {
	return a.Add("POST", path, handlers...)
}

func (a *App) Put(path string, handlers ...HandlerFunc) *App {
	return a.Add("PUT", path, handlers...)
}

func (a *App) Delete(path string, handlers ...HandlerFunc) *App {
	return a.Add("DELETE", path, handlers...)
}

func (a *App) Patch(path string, handlers ...HandlerFunc) *App {
	return a.Add("PATCH", path, handlers...)
}

func (a *App) Head(path string, handlers ...HandlerFunc) *App {
	return a.Add("HEAD", path, handlers...)
}

func (a *App) Options(path string, handlers ...HandlerFunc) *App {
	return a.Add("OPTIONS", path, handlers...)
}

func (a *App) All(path string, handlers ...HandlerFunc) *App {
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"} {
		a.Add(m, path, handlers...)
	}
	return a
}

// Use registers global middleware (applied to all routes).
func (a *App) Use(handlers ...HandlerFunc) *App {
	a.middleware = append(a.middleware, handlers...)
	return a
}

// Group creates a route group with a shared prefix and optional middleware.
func (a *App) Group(prefix string, handlers ...HandlerFunc) *Group {
	g := &Group{app: a, prefix: prefix, middleware: handlers}
	a.groups = append(a.groups, g)
	return g
}

// ── Lifecycle hooks ────────────────────────────────────────────────────────

func (a *App) OnListen(fn HookFunc) *App {
	a.hooks.onListen = append(a.hooks.onListen, fn)
	return a
}

func (a *App) OnShutdown(fn HookFunc) *App {
	a.hooks.onShutdown = append(a.hooks.onShutdown, fn)
	return a
}

func (a *App) OnConnect(fn func(net.Conn)) *App {
	a.hooks.onConnect = append(a.hooks.onConnect, fn)
	return a
}

func (a *App) OnClose(fn func(net.Conn)) *App {
	a.hooks.onClose = append(a.hooks.onClose, fn)
	return a
}

func (a *App) OnError(fn func(error)) *App {
	a.hooks.onError = append(a.hooks.onError, fn)
	return a
}

// ── Listen ─────────────────────────────────────────────────────────────────

func (a *App) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return a.Serve(ln)
}

func (a *App) Serve(ln net.Listener) error {
	a.listener = ln
	a.closed.Store(false)

	for _, fn := range a.hooks.onListen {
		if err := fn(); err != nil {
			return err
		}
	}

	a.logger.Printf("[fasthttp] Listening on %s", ln.Addr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			if a.closed.Load() {
				break
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				a.logger.Printf("[fasthttp] accept timeout: %v", err)
				continue
			}
			a.logger.Printf("[fasthttp] accept error: %v", err)
			continue
		}

		if a.sem != nil {
			a.sem <- struct{}{}
		}

		a.activeConn.Add(1)
		go a.serveConn(conn)
	}

	a.activeConn.Wait()

	for _, fn := range a.hooks.onShutdown {
		if err := fn(); err != nil {
			a.logger.Printf("[fasthttp] shutdown hook error: %v", err)
		}
	}

	return nil
}

func (a *App) Shutdown() error {
	a.closed.Store(true)
	if a.listener != nil {
		return a.listener.Close()
	}
	return nil
}

func (a *App) ShutdownWithTimeout(d time.Duration) error {
	if err := a.Shutdown(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		a.activeConn.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(d):
		return errors.New("shutdown timed out")
	}
}

// ── Connection handler ─────────────────────────────────────────────────────

func (a *App) serveConn(conn net.Conn) {
	defer func() {
		conn.Close()
		a.activeConn.Done()
		if a.sem != nil {
			<-a.sem
		}
		for _, fn := range a.hooks.onClose {
			fn(conn)
		}
	}()

	for _, fn := range a.hooks.onConnect {
		fn(conn)
	}

	rawBuf := getBuf(a.cfg.ReadBufferSize)
	defer putBuf(rawBuf)
	buf := *rawBuf

	accumulated := buf[:0]

	for {
		if err := conn.SetReadDeadline(time.Now().Add(a.cfg.IdleTimeout)); err != nil {
			return
		}

		headEnd := -1
		for headEnd < 0 {
			if len(accumulated) == cap(buf) {
				conn.Write([]byte("HTTP/1.1 431 Request Header Fields Too Large\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
				return
			}

			if err := conn.SetReadDeadline(time.Now().Add(a.cfg.ReadTimeout)); err != nil {
				return
			}

			n, err := conn.Read(buf[len(accumulated):cap(buf)])
			if n > 0 {
				accumulated = buf[:len(accumulated)+n]
			}
			if err != nil {
				if err != io.EOF {
					a.emitError(err)
				}
				return
			}

			headEnd = findHeaderEnd(accumulated)
		}

		// ── Parse request head ────────────────────────────────────────
		ctx := acquireCtx(conn, a)

		consumed, err := parseRequestLine(accumulated, &ctx.Header)
		if err != nil {
			releaseCtx(ctx)
			conn.Write(serverError400)
			return
		}

		_, err = parseHeaders(accumulated[consumed:headEnd+4], &ctx.Header)
		if err != nil {
			releaseCtx(ctx)
			conn.Write(serverError400)
			return
		}

		bodyStart := headEnd + 4
		bodyLen := ctx.Header.ContentLength

		if bodyLen > 0 {
			available := len(accumulated) - bodyStart
			var bodyBuf []byte
			if bodyStart+bodyLen <= cap(buf) {
				bodyBuf = buf[:bodyStart+bodyLen]
				for available < bodyLen {
					if err := conn.SetReadDeadline(time.Now().Add(a.cfg.ReadTimeout)); err != nil {
						releaseCtx(ctx)
						return
					}
					n, err := conn.Read(bodyBuf[bodyStart+available:])
					if n > 0 {
						available += n
					}
					if err != nil {
						if err != io.EOF {
							a.emitError(err)
						}
						releaseCtx(ctx)
						return
					}
				}
				ctx.body = bodyBuf[bodyStart : bodyStart+bodyLen]
			} else {
				extra := make([]byte, bodyLen)
				copy(extra, accumulated[bodyStart:])
				remaining := bodyLen - available
				if remaining > 0 {
					if _, err := io.ReadFull(conn, extra[available:]); err != nil {
						releaseCtx(ctx)
						return
					}
				}
				ctx.body = extra
				bodyStart += bodyLen
				available = bodyLen
			}
			accumulated = buf[:bodyStart+bodyLen]
		}

		if err := conn.SetWriteDeadline(time.Now().Add(a.cfg.WriteTimeout)); err != nil {
			releaseCtx(ctx)
			return
		}

		a.dispatch(ctx)
		keepAlive := ctx.Header.KeepAlive && !a.cfg.DisableKeepAlive

		releaseCtx(ctx)

		if !keepAlive {
			return
		}

		nextStart := bodyStart + bodyLen
		if nextStart < len(accumulated) {
			copy(buf, accumulated[nextStart:])
			accumulated = buf[:len(accumulated)-nextStart]
		} else {
			accumulated = buf[:0]
		}
	}
}

func (a *App) dispatch(ctx *Ctx) {
	defer func() {
		if r := recover(); r != nil {
			a.logger.Printf("[fasthttp] panic: %v\n%s", r, debug.Stack())
			ctx.Status(500).SendString("Internal Server Error")
		}
	}()

	path := ctx.path()
	handler := a.router.FindBytes(ctx.Header.Method, path, &ctx.params)

	if handler == nil {
		notFound := func(ctx *Ctx) error {
			ctx.Status(404)
			return ctx.SendString("404 Not Found")
		}
		if len(a.middleware) > 0 {
			handler = a.chain([]HandlerFunc{notFound})
		} else {
			handler = notFound
		}
	}

	if err := handler(ctx); err != nil {
		a.cfg.ErrorHandler(ctx, err)
	}
}

// chain combines global middleware + route-specific handlers into one HandlerFunc.
// For the common case (no middleware, single handler), returns handler directly — zero alloc.
func (a *App) chain(handlers []HandlerFunc) HandlerFunc {
	if len(a.middleware) == 0 && len(handlers) == 1 {
		return handlers[0]
	}

	all := make([]HandlerFunc, 0, len(a.middleware)+len(handlers))
	all = append(all, a.middleware...)
	all = append(all, handlers...)

	return func(ctx *Ctx) error {
		i := -1
		var next func() error
		next = func() error {
			i++
			if i < len(all) {
				ctx.Next = next
				return all[i](ctx)
			}
			return nil
		}
		ctx.Next = next
		return next()
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

func findHeaderEnd(b []byte) int {
	for i := 0; i < len(b)-3; i++ {
		if b[i] == '\r' && b[i+1] == '\n' && b[i+2] == '\r' && b[i+3] == '\n' {
			return i
		}
	}
	return -1
}

func (a *App) emitError(err error) {
	for _, fn := range a.hooks.onError {
		fn(err)
	}
}

func defaultErrorHandler(ctx *Ctx, err error) {
	ctx.Status(500).SendString("Internal Server Error: " + err.Error())
}

// Pre-allocated 400 error response
var serverError400 = []byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
var plainTextCT = []byte("text/plain; charset=utf-8")

package fh

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Validator is implemented by request DTOs that can validate themselves.
type Validator interface{ Validate() error }

type StructuredError struct {
	Error     string       `json:"error"`
	Message   string       `json:"message,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
}

// PostTyped registers a typed JSON endpoint. Go does not support generic methods,
// so the handler is supplied as a typed function with this shape:
//
//	func(*fh.Ctx, CreateUserRequest) (UserResponse, error)
//
// fh validates that shape at registration time and builds the parsing/encoding wrapper.
func (a *App) PostTyped(path string, handler any, middleware ...HandlerFunc) *App {
	return a.addTyped("POST", path, handler, middleware...)
}
func (a *App) PutTyped(path string, handler any, middleware ...HandlerFunc) *App {
	return a.addTyped("PUT", path, handler, middleware...)
}
func (a *App) PatchTyped(path string, handler any, middleware ...HandlerFunc) *App {
	return a.addTyped("PATCH", path, handler, middleware...)
}
func (a *App) DeleteTyped(path string, handler any, middleware ...HandlerFunc) *App {
	return a.addTyped("DELETE", path, handler, middleware...)
}

func (a *App) addTyped(method, path string, handler any, middleware ...HandlerFunc) *App {
	hv := reflect.ValueOf(handler)
	ht := hv.Type()
	ctxType := reflect.TypeOf(&Ctx{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	if ht.Kind() != reflect.Func || ht.NumIn() != 2 || ht.In(0) != ctxType || ht.NumOut() != 2 || !ht.Out(1).Implements(errType) {
		panic("fh: typed handler must be func(*fh.Ctx, Req) (Res, error)")
	}
	reqType, resType := ht.In(1), ht.Out(0)
	h := func(c *Ctx) error {
		reqPtr := reflect.New(reqType)
		if len(c.Body()) != 0 {
			if err := json.Unmarshal(c.Body(), reqPtr.Interface()); err != nil {
				return c.Status(StatusBadRequest).JSON(StructuredError{Error: "invalid_json", Message: err.Error(), RequestID: requestIDFromCtx(c)})
			}
		}
		reqVal := reqPtr.Elem()
		if v, ok := reqPtr.Interface().(Validator); ok {
			if err := v.Validate(); err != nil {
				return typedValidationError(c, err)
			}
		} else if reqVal.CanInterface() {
			if v, ok := reqVal.Interface().(Validator); ok {
				if err := v.Validate(); err != nil {
					return typedValidationError(c, err)
				}
			}
		}
		out := hv.Call([]reflect.Value{reflect.ValueOf(c), reqVal})
		if !out[1].IsNil() {
			return out[1].Interface().(error)
		}
		return c.JSON(out[0].Interface())
	}
	handlers := append([]HandlerFunc{}, middleware...)
	handlers = append(handlers, h)
	a.Add(method, path, handlers...)
	a.updateRouteInfo(method, path, func(ri *RouteInfo) {
		ri.Typed = true
		ri.RequestType = niceTypeName(reqType)
		ri.ResponseType = niceTypeName(resType)
		ri.RequestSchema = schemaFromType(reqType, map[reflect.Type]bool{})
		ri.ResponseSchema = schemaFromType(resType, map[reflect.Type]bool{})
	})
	return a
}

func niceTypeName(t reflect.Type) string {
	if t == nil {
		return "any"
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" {
		return t.String()
	}
	return t.Name()
}
func typedValidationError(c *Ctx, err error) error {
	return c.Status(StatusUnprocessableEntity).JSON(StructuredError{Error: "validation_failed", Message: err.Error(), RequestID: requestIDFromCtx(c)})
}
func requestIDFromCtx(c *Ctx) string {
	if v := c.Get(HeaderRequestID); v != "" {
		return v
	}
	return ""
}

type JSONSchema map[string]any

func schemaFromType(t reflect.Type, seen map[reflect.Type]bool) JSONSchema {
	if t == nil {
		return JSONSchema{"type": "object"}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if seen[t] {
		return JSONSchema{"type": "object"}
	}
	switch t.Kind() {
	case reflect.String:
		return JSONSchema{"type": "string"}
	case reflect.Bool:
		return JSONSchema{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return JSONSchema{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return JSONSchema{"type": "number"}
	case reflect.Slice, reflect.Array:
		return JSONSchema{"type": "array", "items": schemaFromType(t.Elem(), seen)}
	case reflect.Map:
		return JSONSchema{"type": "object", "additionalProperties": schemaFromType(t.Elem(), seen)}
	case reflect.Struct:
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return JSONSchema{"type": "string", "format": "date-time"}
		}
		seen[t] = true
		props := map[string]any{}
		req := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fs := schemaFromType(f.Type, seen)
			if tag := f.Tag.Get("format"); tag != "" {
				fs["format"] = tag
			}
			if desc := f.Tag.Get("description"); desc != "" {
				fs["description"] = desc
			}
			props[name] = fs
			if strings.Contains(f.Tag.Get("validate"), "required") {
				req = append(req, name)
			}
		}
		out := JSONSchema{"type": "object", "properties": props}
		if len(req) > 0 {
			out["required"] = req
		}
		return out
	default:
		return JSONSchema{"type": "object"}
	}
}

type RouteInfo struct {
	Method         string     `json:"method"`
	Path           string     `json:"path"`
	Name           string     `json:"name,omitempty"`
	Typed          bool       `json:"typed,omitempty"`
	RequestType    string     `json:"request_type,omitempty"`
	ResponseType   string     `json:"response_type,omitempty"`
	RequestSchema  JSONSchema `json:"request_schema,omitempty"`
	ResponseSchema JSONSchema `json:"response_schema,omitempty"`
	Deprecated     bool       `json:"deprecated,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
}

func (a *App) registerRouteInfo(info RouteInfo) {
	a.routeMetaMu.Lock()
	defer a.routeMetaMu.Unlock()
	for _, r := range a.routeMeta {
		if r.Method == info.Method && r.Path == info.Path {
			return
		}
	}
	a.routeMeta = append(a.routeMeta, info)
}
func (a *App) updateRouteInfo(method, path string, fn func(*RouteInfo)) {
	method = strings.ToUpper(method)
	path = normalizeRoutePath(method, path)
	a.routeMetaMu.Lock()
	defer a.routeMetaMu.Unlock()
	for i := range a.routeMeta {
		if a.routeMeta[i].Method == method && a.routeMeta[i].Path == path {
			fn(&a.routeMeta[i])
			return
		}
	}
	ri := RouteInfo{Method: method, Path: path}
	fn(&ri)
	a.routeMeta = append(a.routeMeta, ri)
}
func (a *App) Routes() []RouteInfo {
	a.routeMetaMu.RLock()
	defer a.routeMetaMu.RUnlock()
	out := append([]RouteInfo(nil), a.routeMeta...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Method < out[j].Method
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// OpenAPIConfig controls native OpenAPI export and docs UI.
type OpenAPIConfig struct {
	Title, Version, Description string
	Servers                     []string
}

func (a *App) EnableOpenAPI(path string, cfg OpenAPIConfig) *App {
	if path == "" {
		path = "/openapi.json"
	}
	a.openapi = cfg
	a.Get(path, func(c *Ctx) error { return c.JSON(a.OpenAPI()) })
	return a
}
func (a *App) EnableDocs(path string) *App {
	if path == "" {
		path = "/docs"
	}
	a.Get(path, func(c *Ctx) error { c.Type("text/html; charset=utf-8"); return c.SendString(docsHTML) })
	return a
}
func (a *App) OpenAPI() map[string]any {
	cfg := a.openapi
	if cfg.Title == "" {
		cfg.Title = "fh API"
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}
	paths := map[string]any{}
	for _, r := range a.Routes() {
		if r.Path == "*" {
			continue
		}
		item, _ := paths[r.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[r.Path] = item
		}
		op := map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}}
		if len(r.Tags) > 0 {
			op["tags"] = r.Tags
		}
		if r.Deprecated {
			op["deprecated"] = true
		}
		if r.RequestSchema != nil {
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": r.RequestSchema}}}
		}
		if r.ResponseSchema != nil {
			op["responses"] = map[string]any{"200": map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{"schema": r.ResponseSchema}}}}
		}
		item[strings.ToLower(r.Method)] = op
	}
	servers := []map[string]string{}
	for _, s := range cfg.Servers {
		servers = append(servers, map[string]string{"url": s})
	}
	return map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": cfg.Title, "version": cfg.Version, "description": cfg.Description}, "servers": servers, "paths": paths}
}

const docsHTML = `<!doctype html><html><head><title>fh API Docs</title><meta name="viewport" content="width=device-width,initial-scale=1"><style>body{font-family:system-ui;margin:2rem;max-width:980px}pre{background:#f6f8fa;padding:1rem;overflow:auto}</style></head><body><h1>fh API Docs</h1><p>OpenAPI JSON is available at <a href="/openapi.json">/openapi.json</a>.</p><pre id="spec">Loading...</pre><script>fetch('/openapi.json').then(r=>r.json()).then(j=>spec.textContent=JSON.stringify(j,null,2))</script></body></html>`

func (a *App) EnableRouteList(path string) *App {
	if path == "" {
		path = "/_fh/routes"
	}
	a.Get(path, func(c *Ctx) error { return c.JSON(a.Routes()) })
	return a
}

/* metrics are implemented in policy.go */

// Request lifecycle hooks.
type RequestHook func(*Ctx)
type RequestHooks struct {
	Before RequestHook
	After  RequestHook
	Error  func(*Ctx, error)
}

func LifecycleHooks(h RequestHooks) HandlerFunc {
	return func(c *Ctx) error {
		if h.Before != nil {
			h.Before(c)
		}
		err := c.Next()
		if err != nil && h.Error != nil {
			h.Error(c, err)
		}
		if h.After != nil {
			h.After(c)
		}
		return err
	}
}

// Security helpers and middleware.
func ConstantTimeEqual(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }
func RedactSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "[REDACTED]"
	}
	return s[:4] + "…" + s[len(s)-4:]
}
func SignCookie(value string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	return value + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func VerifySignedCookie(s string, secret []byte) (string, bool) {
	i := strings.LastIndexByte(s, '.')
	if i < 0 {
		return "", false
	}
	val, sig := s[:i], s[i+1:]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(val))
	exp := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return val, hmac.Equal([]byte(sig), []byte(exp))
}

type APIKeyConfig struct {
	Header string
	Query  string
	Keys   []string
	Lookup func(*Ctx, string) bool
}

func APIKey(cfg APIKeyConfig) HandlerFunc {
	if cfg.Header == "" {
		cfg.Header = "X-API-Key"
	}
	set := map[string]struct{}{}
	for _, k := range cfg.Keys {
		set[k] = struct{}{}
	}
	return func(c *Ctx) error {
		key := c.Get(cfg.Header)
		if key == "" && cfg.Query != "" {
			key = c.Query(cfg.Query)
		}
		ok := false
		if cfg.Lookup != nil {
			ok = cfg.Lookup(c, key)
		} else {
			_, ok = set[key]
		}
		if key == "" || !ok {
			return c.Status(StatusUnauthorized).JSON(Map{"error": "invalid_api_key"})
		}
		return c.Next()
	}
}

type IPListConfig struct {
	Allow []string
	Block []string
}

func IPList(cfg IPListConfig) HandlerFunc {
	allows := compileCIDRs(cfg.Allow)
	blocks := compileCIDRs(cfg.Block)
	return func(c *Ctx) error {
		ip := net.ParseIP(c.IP())
		if ip == nil {
			return c.Status(StatusForbidden).JSON(Map{"error": "invalid_ip"})
		}
		if matchCIDRs(ip, blocks) {
			return c.Status(StatusForbidden).JSON(Map{"error": "ip_blocked"})
		}
		if len(allows) > 0 && !matchCIDRs(ip, allows) {
			return c.Status(StatusForbidden).JSON(Map{"error": "ip_not_allowed"})
		}
		return c.Next()
	}
}
func compileCIDRs(in []string) []*net.IPNet {
	out := []*net.IPNet{}
	for _, s := range in {
		if ip := net.ParseIP(s); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, n, err := net.ParseCIDR(s); err == nil {
			out = append(out, n)
		}
	}
	return out
}
func matchCIDRs(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

type HMACConfig struct {
	Secret          []byte
	Header          string
	TimestampHeader string
	Tolerance       time.Duration
}

func HMACSignedRequest(cfg HMACConfig) HandlerFunc {
	if cfg.Header == "" {
		cfg.Header = "X-Signature"
	}
	if cfg.TimestampHeader == "" {
		cfg.TimestampHeader = "X-Timestamp"
	}
	if cfg.Tolerance <= 0 {
		cfg.Tolerance = 5 * time.Minute
	}
	return func(c *Ctx) error {
		ts := c.Get(cfg.TimestampHeader)
		sig := strings.TrimPrefix(c.Get(cfg.Header), "sha256=")
		if ts == "" || sig == "" {
			return c.Status(StatusUnauthorized).JSON(Map{"error": "missing_signature"})
		}
		when, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || absDuration(time.Since(time.Unix(when, 0))) > cfg.Tolerance {
			return c.Status(StatusUnauthorized).JSON(Map{"error": "stale_signature"})
		}
		mac := hmac.New(sha256.New, cfg.Secret)
		mac.Write([]byte(ts))
		mac.Write([]byte("."))
		mac.Write(c.Body())
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			return c.Status(StatusUnauthorized).JSON(Map{"error": "invalid_signature"})
		}
		return c.Next()
	}
}
func HMACWebhook(secret []byte, header string) HandlerFunc {
	return HMACSignedRequest(HMACConfig{Secret: secret, Header: header, TimestampHeader: "X-Timestamp"})
}
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

type ReplayProtectionStore interface {
	Seen(key string, ttl time.Duration) (bool, error)
}
type MemoryReplayStore struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func NewMemoryReplayStore() *MemoryReplayStore { return &MemoryReplayStore{m: map[string]time.Time{}} }
func (s *MemoryReplayStore) Seen(key string, ttl time.Duration) (bool, error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, exp := range s.m {
		if exp.Before(now) {
			delete(s.m, k)
		}
	}
	if exp, ok := s.m[key]; ok && exp.After(now) {
		return true, nil
	}
	s.m[key] = now.Add(ttl)
	return false, nil
}
func ReplayProtection(store ReplayProtectionStore, header string, ttl time.Duration) HandlerFunc {
	if store == nil {
		store = NewMemoryReplayStore()
	}
	if header == "" {
		header = "X-Nonce"
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return func(c *Ctx) error {
		nonce := c.Get(header)
		if nonce == "" {
			return c.Status(StatusUnauthorized).JSON(Map{"error": "missing_nonce"})
		}
		seen, err := store.Seen(nonce, ttl)
		if err != nil {
			return err
		}
		if seen {
			return c.Status(StatusConflict).JSON(Map{"error": "replay_detected"})
		}
		return c.Next()
	}
}

func RequestTimeout(d time.Duration) HandlerFunc {
	return func(c *Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), d)
		defer cancel()
		c.SetContext(ctx)
		return c.Next()
	}
}
func BodyLimit(n int) HandlerFunc {
	return func(c *Ctx) error {
		if n > 0 && len(c.Body()) > n {
			return c.Status(StatusPayloadTooLarge).JSON(Map{"error": "body_too_large"})
		}
		return c.Next()
	}
}

// Reliability route policy helpers are implemented in reliability.go.
func DeterministicIdempotencyMiddleware(fn func(*Ctx) string) HandlerFunc {
	return func(c *Ctx) error {
		if fn != nil {
			if key := fn(c); key != "" {
				c.Header.Set(HeaderIdempotencyKey, key)
			}
		}
		return c.Next()
	}
}

type AtomicJobOptions struct {
	Type           string
	Body           []byte
	Priority       int
	Delay          time.Duration
	RunAt          time.Time
	ConcurrencyKey string
	Headers        map[string]string
	MaxAttempts    int
}
type AtomicJobResult struct {
	ID string `json:"id"`
}

const (
	PriorityLow    = -10
	PriorityNormal = 0
	PriorityHigh   = 10
)

func AtomicJob(c *Ctx, opt AtomicJobOptions) (*AtomicJobResult, error) {
	if c == nil || c.server == nil || c.server.reliability == nil || c.server.reliability.queue == nil {
		return nil, errors.New("fh: reliability queue is not enabled")
	}
	spec := QueueJob{Type: opt.Type, Payload: opt.Body, Priority: opt.Priority, RunAt: opt.RunAt, VisibleAt: opt.RunAt, ConcurrencyKey: opt.ConcurrencyKey, MaxAttempts: opt.MaxAttempts, Headers: opt.Headers}
	if opt.Delay > 0 {
		spec.VisibleAt = time.Now().UTC().Add(opt.Delay)
		spec.RunAt = spec.VisibleAt
	}
	id, err := c.server.reliability.queue.EnqueueJob(spec, opt.Body, opt.Headers)
	if err != nil {
		return nil, err
	}
	return &AtomicJobResult{ID: id}, nil
}

// StaticAdvanced serves static assets with safe path handling, cache headers,
// ETag, Last-Modified, Range support, SPA fallback, and download disposition.
type StaticAdvancedConfig struct {
	ETag         bool
	LastModified bool
	CacheControl string
	Immutable    bool
	Index        bool
	SPAFallback  string
	Download     bool
}

func StaticAdvanced(root string, cfg StaticAdvancedConfig) HandlerFunc {
	fsroot := filepath.Clean(root)
	return func(c *Ctx) error {
		rel := strings.TrimPrefix(c.Param("*"), "/")
		if rel == "" {
			rel = strings.TrimPrefix(c.Path(), "/")
		}
		clean := filepath.Clean("/" + rel)
		full := filepath.Join(fsroot, strings.TrimPrefix(clean, "/"))
		if !strings.HasPrefix(full, fsroot) {
			return c.Status(StatusForbidden).SendString("forbidden")
		}
		st, err := os.Stat(full)
		if err != nil && cfg.SPAFallback != "" {
			full = filepath.Join(fsroot, cfg.SPAFallback)
			st, err = os.Stat(full)
		}
		if err != nil {
			return c.SendStatus(StatusNotFound)
		}
		if st.IsDir() {
			if !cfg.Index {
				return c.SendStatus(StatusForbidden)
			}
			full = filepath.Join(full, "index.html")
			st, err = os.Stat(full)
			if err != nil {
				return c.SendStatus(StatusNotFound)
			}
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if ct := mime.TypeByExtension(filepath.Ext(full)); ct != "" {
			c.Type(ct)
		}
		if cfg.CacheControl != "" {
			c.Set("Cache-Control", cfg.CacheControl)
		} else if cfg.Immutable {
			c.Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if cfg.LastModified {
			c.Set("Last-Modified", st.ModTime().UTC().Format(http.TimeFormat))
		}
		if cfg.ETag {
			sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", full, st.Size(), st.ModTime().UnixNano())))
			etag := "\"" + hex.EncodeToString(sum[:8]) + "\""
			c.Set("ETag", etag)
			if c.Get("If-None-Match") == etag {
				return c.SendStatus(StatusNotModified)
			}
		}
		if cfg.Download {
			c.Set("Content-Disposition", "attachment; filename=\""+filepath.Base(full)+"\"")
		}
		return c.SendBytes(data)
	}
}

// parseHeadersLimit is app-configurable while preserving parseHeaders for tests.
func parseHeadersLimit(src []byte, h *RequestHeader, maxCount int) (int, error) {
	if maxCount <= 0 || maxCount >= maxHeaders {
		return parseHeaders(src, h)
	}
	n, err := parseHeaders(src, h)
	if err != nil {
		return n, err
	}
	if h.hcount > maxCount {
		return 0, ErrMalformedRequest
	}
	return n, nil
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func rawQuery(c *Ctx) string {
	u := string(c.Header.URI)
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[i+1:]
	}
	return ""
}

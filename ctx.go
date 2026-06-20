package fasthttp

import (
	"encoding/json"
	"net"
	"sync"
)

type Ctx struct {
	conn   net.Conn
	server *App

	Header RequestHeader

	params []Param

	status      int
	customHeaders [4]Header
	chCount     int
	body        []byte
	contentType []byte

	Next func() error

	locals [16]localEntry
	lcount int

	readBuf  *[]byte
	writeBuf *[]byte

	queryParsed bool
	queryParams []Param
	qcount      int
}

type localEntry struct {
	key string
	val any
}

var ctxPool = sync.Pool{
	New: func() any {
		c := &Ctx{
			params:      make([]Param, 0, 8),
			queryParams: make([]Param, 0, 8),
		}
		return c
	},
}

func acquireCtx(conn net.Conn, app *App) *Ctx {
	c := ctxPool.Get().(*Ctx)
	c.conn = conn
	c.server = app
	c.reset()
	return c
}

func releaseCtx(c *Ctx) {
	if c.writeBuf != nil {
		putBytes(c.writeBuf)
		c.writeBuf = nil
	}
	ctxPool.Put(c)
}

func (c *Ctx) reset() {
	c.Header.reset()
	c.params = c.params[:0]
	c.status = 200
	c.chCount = 0
	c.body = nil
	c.contentType = nil
	c.lcount = 0
	c.queryParsed = false
	c.qcount = 0
	c.queryParams = c.queryParams[:0]
}

// ── Request accessors ──────────────────────────────────────────────────────

func (c *Ctx) Method() string { return string(c.Header.Method) }

func (c *Ctx) path() []byte {
	uri := c.Header.URI
	for i, v := range uri {
		if v == '?' {
			return uri[:i]
		}
	}
	return uri
}

func (c *Ctx) Path() string { return string(c.path()) }

func (c *Ctx) Param(name string) string {
	for i := range c.params {
		if c.params[i].Key == name {
			return c.params[i].Value
		}
	}
	return ""
}

func (c *Ctx) Query(name string) string {
	if !c.queryParsed {
		c.parseQuery()
	}
	for i := 0; i < c.qcount; i++ {
		if c.queryParams[i].Key == name {
			return c.queryParams[i].Value
		}
	}
	return ""
}

func (c *Ctx) parseQuery() {
	c.queryParsed = true
	uri := c.Header.URI
	qi := indexByte(uri, '?')
	if qi < 0 {
		return
	}
	qs := uri[qi+1:]
	for len(qs) > 0 {
		var pair []byte
		if i := indexByte(qs, '&'); i >= 0 {
			pair, qs = qs[:i], qs[i+1:]
		} else {
			pair, qs = qs, nil
		}
		if i := indexByte(pair, '='); i >= 0 {
			k := urlDecode(pair[:i])
			v := urlDecode(pair[i+1:])
			if c.qcount < len(c.queryParams) {
				c.queryParams[c.qcount] = Param{Key: k, Value: v}
			} else {
				c.queryParams = append(c.queryParams, Param{Key: k, Value: v})
			}
			c.qcount++
		}
	}
}

func (c *Ctx) Body() []byte { return c.body }

func (c *Ctx) BodyParser(v any) error {
	return json.Unmarshal(c.body, v)
}

func (c *Ctx) Get(name string) string { return c.Header.PeekStr(name) }

func (c *Ctx) Locals(key string, value ...any) any {
	if len(value) > 0 {
		for i := 0; i < c.lcount; i++ {
			if c.locals[i].key == key {
				c.locals[i].val = value[0]
				return value[0]
			}
		}
		if c.lcount < len(c.locals) {
			c.locals[c.lcount] = localEntry{key: key, val: value[0]}
			c.lcount++
		}
		return value[0]
	}
	for i := 0; i < c.lcount; i++ {
		if c.locals[i].key == key {
			return c.locals[i].val
		}
	}
	return nil
}

func (c *Ctx) IP() string {
	addr := c.conn.RemoteAddr().String()
	host, _, _ := splitHostPort(addr)
	return host
}

// ── Response builders ──────────────────────────────────────────────────────

func (c *Ctx) Status(code int) *Ctx {
	c.status = code
	return c
}

func (c *Ctx) Set(key, value string) {
	k := []byte(key)
	v := []byte(value)
	if bytesEqualFold(k, headerContentType) {
		c.contentType = v
		return
	}
	for i := 0; i < c.chCount; i++ {
		if bytesEqualFold(c.customHeaders[i].Key, k) {
			c.customHeaders[i].Value = v
			return
		}
	}
	if c.chCount < len(c.customHeaders) {
		c.customHeaders[c.chCount] = Header{Key: k, Value: v}
		c.chCount++
	}
}

func (c *Ctx) Type(mime string) *Ctx {
	c.contentType = []byte(mime)
	return c
}

func (c *Ctx) SendString(s string) error {
	if c.contentType == nil {
		c.contentType = plainTextCT
	}
	return c.writeResponseString(s)
}

func (c *Ctx) SendBytes(b []byte) error {
	return c.writeResponse(b)
}

func (c *Ctx) Send(b []byte) error { return c.SendBytes(b) }

func (c *Ctx) JSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.contentType = jsonCT
	return c.writeResponse(b)
}

func (c *Ctx) SendStatus(code int) error {
	c.status = code
	return c.writeResponse(nil)
}

func (c *Ctx) Redirect(location string, code ...int) error {
	sc := 302
	if len(code) > 0 {
		sc = code[0]
	}
	c.status = sc
	c.Set("Location", location)
	return c.writeResponse(nil)
}

// writeResponseString writes a response with a string body — zero alloc.
func (c *Ctx) writeResponseString(s string) error {
	if c.writeBuf == nil {
		c.writeBuf = getBytes()
	}
	buf := (*c.writeBuf)[:0]

	// Status line
	buf = appendStatusLine(buf, c.status)

	// Content-Type
	if c.contentType != nil {
		buf = append(buf, "Content-Type: "...)
		buf = append(buf, c.contentType...)
		buf = append(buf, '\r', '\n')
	}

	// Custom headers
	for i := 0; i < c.chCount; i++ {
		h := &c.customHeaders[i]
		buf = append(buf, h.Key...)
		buf = append(buf, ':', ' ')
		buf = append(buf, h.Value...)
		buf = append(buf, '\r', '\n')
	}

	// Content-Length
	buf = append(buf, "Content-Length: "...)
	buf = appendInt(buf, len(s))
	buf = append(buf, '\r', '\n')

	// Connection
	if c.Header.KeepAlive {
		buf = append(buf, "Connection: keep-alive\r\n"...)
	} else {
		buf = append(buf, "Connection: close\r\n"...)
	}

	buf = append(buf, '\r', '\n')

	// Body — append string directly, no []byte conversion
	buf = append(buf, s...)

	*c.writeBuf = buf
	_, err := c.conn.Write(buf)
	return err
}

// writeResponse writes a response with a byte body.
func (c *Ctx) writeResponse(body []byte) error {
	if c.writeBuf == nil {
		c.writeBuf = getBytes()
	}
	buf := (*c.writeBuf)[:0]

	buf = appendStatusLine(buf, c.status)

	if c.contentType != nil {
		buf = append(buf, "Content-Type: "...)
		buf = append(buf, c.contentType...)
		buf = append(buf, '\r', '\n')
	}

	for i := 0; i < c.chCount; i++ {
		h := &c.customHeaders[i]
		buf = append(buf, h.Key...)
		buf = append(buf, ':', ' ')
		buf = append(buf, h.Value...)
		buf = append(buf, '\r', '\n')
	}

	if len(body) > 0 {
		buf = append(buf, "Content-Length: "...)
		buf = appendInt(buf, len(body))
		buf = append(buf, '\r', '\n')
	}

	if c.Header.KeepAlive {
		buf = append(buf, "Connection: keep-alive\r\n"...)
	} else {
		buf = append(buf, "Connection: close\r\n"...)
	}

	buf = append(buf, '\r', '\n')

	if len(body) > 0 {
		buf = append(buf, body...)
	}

	*c.writeBuf = buf
	_, err := c.conn.Write(buf)
	return err
}

// appendStatusLine writes "HTTP/1.1 <code> <text>\r\n" to buf.
func appendStatusLine(buf []byte, code int) []byte {
	switch code {
	case 200:
		return append(buf, "HTTP/1.1 200 OK\r\n"...)
	case 201:
		return append(buf, "HTTP/1.1 201 Created\r\n"...)
	case 204:
		return append(buf, "HTTP/1.1 204 No Content\r\n"...)
	case 301:
		return append(buf, "HTTP/1.1 301 Moved Permanently\r\n"...)
	case 302:
		return append(buf, "HTTP/1.1 302 Found\r\n"...)
	case 304:
		return append(buf, "HTTP/1.1 304 Not Modified\r\n"...)
	case 400:
		return append(buf, "HTTP/1.1 400 Bad Request\r\n"...)
	case 401:
		return append(buf, "HTTP/1.1 401 Unauthorized\r\n"...)
	case 403:
		return append(buf, "HTTP/1.1 403 Forbidden\r\n"...)
	case 404:
		return append(buf, "HTTP/1.1 404 Not Found\r\n"...)
	case 405:
		return append(buf, "HTTP/1.1 405 Method Not Allowed\r\n"...)
	case 409:
		return append(buf, "HTTP/1.1 409 Conflict\r\n"...)
	case 422:
		return append(buf, "HTTP/1.1 422 Unprocessable Entity\r\n"...)
	case 429:
		return append(buf, "HTTP/1.1 429 Too Many Requests\r\n"...)
	case 500:
		return append(buf, "HTTP/1.1 500 Internal Server Error\r\n"...)
	case 502:
		return append(buf, "HTTP/1.1 502 Bad Gateway\r\n"...)
	case 503:
		return append(buf, "HTTP/1.1 503 Service Unavailable\r\n"...)
	default:
		buf = append(buf, "HTTP/1.1 "...)
		buf = appendInt(buf, code)
		buf = append(buf, ' ')
		buf = append(buf, statusText(code)...)
		return append(buf, '\r', '\n')
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

func splitHostPort(addr string) (host, port string, err error) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}
	return addr, "", nil
}

func urlDecode(b []byte) string {
	if indexByte(b, '%') < 0 && indexByte(b, '+') < 0 {
		return string(b)
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		switch b[i] {
		case '+':
			out = append(out, ' ')
			i++
		case '%':
			if i+2 < len(b) {
				h := unhex(b[i+1])
				l := unhex(b[i+2])
				if h >= 0 && l >= 0 {
					out = append(out, byte(h<<4|l))
					i += 3
					continue
				}
			}
			out = append(out, b[i])
			i++
		default:
			out = append(out, b[i])
			i++
		}
	}
	return string(out)
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func statusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 204:
		return "No Content"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 304:
		return "Not Modified"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 409:
		return "Conflict"
	case 422:
		return "Unprocessable Entity"
	case 429:
		return "Too Many Requests"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	default:
		return "Unknown"
	}
}

var jsonCT = []byte("application/json")
var headerLocation = []byte("Location")

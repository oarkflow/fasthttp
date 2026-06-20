package fasthttp

import (
	"bytes"
	"errors"
)

// Common header names as byte slices — avoids repeated string→[]byte conversions.
var (
	headerContentType     = []byte("Content-Type")
	headerContentLength   = []byte("Content-Length")
	headerConnection      = []byte("Connection")
	headerTransferEncoding = []byte("Transfer-Encoding")
	headerHost            = []byte("Host")

	methodGET    = []byte("GET")
	methodPOST   = []byte("POST")
	methodPUT    = []byte("PUT")
	methodDELETE = []byte("DELETE")
	methodPATCH  = []byte("PATCH")
	methodHEAD   = []byte("HEAD")
	methodOPTIONS = []byte("OPTIONS")

	strHTTP11 = []byte("HTTP/1.1")
	strHTTP10 = []byte("HTTP/1.0")
	strCRLF   = []byte("\r\n")
	strColonSpace = []byte(": ")
)

// ErrMalformedRequest is returned when the request cannot be parsed.
var ErrMalformedRequest = errors.New("malformed HTTP request")

// Header is a single HTTP header key/value.
// Both slices point into the read buffer — zero copy.
type Header struct {
	Key   []byte
	Value []byte
}

// RequestHeader holds parsed request metadata. All fields are slices
// into the underlying read buffer — no allocations during parse.
type RequestHeader struct {
	Method        []byte
	URI           []byte
	Proto         []byte
	Host          []byte
	ContentType   []byte
	ContentLength int
	KeepAlive     bool
	Chunked       bool

	headers []Header // raw headers, capped at maxHeaders
	hcount  int
}

const maxHeaders = 64

func (h *RequestHeader) reset() {
	h.Method = h.Method[:0]
	h.URI = h.URI[:0]
	h.Proto = h.Proto[:0]
	h.Host = h.Host[:0]
	h.ContentType = h.ContentType[:0]
	h.ContentLength = 0
	h.KeepAlive = false
	h.Chunked = false
	h.hcount = 0
}

// Peek returns the value of a header by name (case-insensitive).
// Returns nil if not found. Zero allocation.
func (h *RequestHeader) Peek(name []byte) []byte {
	for i := 0; i < h.hcount; i++ {
		if bytesEqualFold(h.headers[i].Key, name) {
			return h.headers[i].Value
		}
	}
	return nil
}

// PeekStr returns header value as string (allocates — use for non-hot paths).
func (h *RequestHeader) PeekStr(name string) string {
	v := h.Peek([]byte(name))
	if v == nil {
		return ""
	}
	return string(v)
}

// parseRequestLine parses "METHOD URI HTTP/1.x\r\n".
// Returns bytes consumed or error.
func parseRequestLine(buf []byte, h *RequestHeader) (int, error) {
	i := bytes.IndexByte(buf, ' ')
	if i < 0 {
		return 0, ErrMalformedRequest
	}
	h.Method = buf[:i]
	buf = buf[i+1:]

	j := bytes.IndexByte(buf, ' ')
	if j < 0 {
		return 0, ErrMalformedRequest
	}
	h.URI = buf[:j]
	buf = buf[j+1:]

	k := bytes.Index(buf, strCRLF)
	if k < 0 {
		return 0, ErrMalformedRequest
	}
	h.Proto = buf[:k]
	h.KeepAlive = bytes.Equal(h.Proto, strHTTP11)
	return i + 1 + j + 1 + k + 2, nil
}

// parseHeaders parses all headers until the blank line.
// All slices point into src — zero allocation.
func parseHeaders(src []byte, h *RequestHeader) (int, error) {
	if cap(h.headers) < maxHeaders {
		h.headers = make([]Header, maxHeaders)
	}
	h.hcount = 0
	pos := 0
	for {
		if pos >= len(src) {
			return 0, ErrMalformedRequest
		}
		// blank line = end of headers
		if src[pos] == '\r' && pos+1 < len(src) && src[pos+1] == '\n' {
			pos += 2
			break
		}
		if src[pos] == '\n' {
			pos++
			break
		}

		// find colon
		colon := bytes.IndexByte(src[pos:], ':')
		if colon < 0 {
			return 0, ErrMalformedRequest
		}
		key := src[pos : pos+colon]
		pos += colon + 1

		// skip optional space
		for pos < len(src) && src[pos] == ' ' {
			pos++
		}

		// find end of value
		end := bytes.Index(src[pos:], strCRLF)
		var val []byte
		if end < 0 {
			// LF only
			end = bytes.IndexByte(src[pos:], '\n')
			if end < 0 {
				return 0, ErrMalformedRequest
			}
			val = trimRight(src[pos : pos+end])
			pos += end + 1
		} else {
			val = trimRight(src[pos : pos+end])
			pos += end + 2
		}

		// store well-known headers directly
		switch {
		case bytesEqualFold(key, headerHost):
			h.Host = val
		case bytesEqualFold(key, headerContentType):
			h.ContentType = val
		case bytesEqualFold(key, headerContentLength):
			h.ContentLength = parseIntFast(val)
		case bytesEqualFold(key, headerConnection):
			h.KeepAlive = bytesEqualFold(val, []byte("keep-alive"))
		case bytesEqualFold(key, headerTransferEncoding):
			h.Chunked = bytes.Contains(val, []byte("chunked"))
		}

		if h.hcount < maxHeaders {
			h.headers[h.hcount] = Header{Key: key, Value: val}
			h.hcount++
		}
	}
	return pos, nil
}

// trimRight removes trailing whitespace/CR.
func trimRight(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\r' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	return b
}

// parseIntFast parses a decimal integer from bytes without allocation.
func parseIntFast(b []byte) int {
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// bytesEqualFold reports whether a and b are equal under ASCII case folding.
func bytesEqualFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		if ca >= 'A' && ca <= 'Z' {
			ca |= 0x20
		}
		if cb >= 'A' && cb <= 'Z' {
			cb |= 0x20
		}
		if ca != cb {
			return false
		}
	}
	return true
}

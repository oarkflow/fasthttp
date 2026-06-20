package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"sync"

	fh "github.com/orgware/fasthttp"
)

var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

func New() fh.HandlerFunc {
	return func(ctx *fh.Ctx) error {
		ae := ctx.Get("Accept-Encoding")
		if !acceptsGzip(ae) {
			return ctx.Next()
		}
		ctx.Set("Content-Encoding", "gzip")
		ctx.Append("Vary", "Accept-Encoding")
		ctx.TransformBody(func(body []byte) ([]byte, error) {
			var dst bytes.Buffer
			w := gzipPool.Get().(*gzip.Writer)
			w.Reset(&dst)
			if _, err := w.Write(body); err != nil {
				gzipPool.Put(w)
				return nil, err
			}
			if err := w.Close(); err != nil {
				gzipPool.Put(w)
				return nil, err
			}
			w.Reset(io.Discard)
			gzipPool.Put(w)
			return dst.Bytes(), nil
		})
		return ctx.Next()
	}
}

func acceptsGzip(header string) bool {
	var foundGzip, gzipOK, foundStar, starOK bool

	i := 0
	for i < len(header) {
		for i < len(header) && (header[i] == ',' || header[i] == ' ') {
			i++
		}
		if i >= len(header) {
			break
		}
		start := i
		for i < len(header) && header[i] != ';' && header[i] != ',' && header[i] != ' ' {
			i++
		}
		name := header[start:i]

		for i < len(header) && header[i] == ' ' {
			i++
		}

		qZero := false
		if i < len(header) && header[i] == ';' {
			i++
			qZero = isQZero(header[i:])
		}

		for i < len(header) && header[i] != ',' {
			i++
		}
		if i < len(header) {
			i++
		}

		if strings.EqualFold(name, "gzip") {
			foundGzip = true
			gzipOK = !qZero
		} else if len(name) == 1 && name[0] == '*' {
			foundStar = true
			starOK = !qZero
		}
	}

	if foundGzip {
		return gzipOK
	}
	if foundStar {
		return starOK
	}
	return false
}

func isQZero(s string) bool {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	if len(s) < 3 || (s[0] != 'q' && s[0] != 'Q') || s[1] != '=' {
		return false
	}
	s = s[2:]
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	if len(s) == 0 {
		return false
	}
	if s[0] != '0' {
		return false
	}
	if len(s) == 1 {
		return true
	}
	if s[1] != '.' {
		return true
	}
	for i := 2; i < len(s); i++ {
		c := s[i]
		if c == ',' || c == ';' || c == ' ' {
			return true
		}
		if c != '0' {
			return false
		}
	}
	return true
}

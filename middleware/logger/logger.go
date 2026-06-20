package logger

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"time"

	fh "github.com/orgware/fasthttp"
)

type Config struct {
	Format string
	Logger *log.Logger
}

func New(config ...Config) fh.HandlerFunc {
	cfg := Config{
		Format: "[${ip}] ${method} ${path} \u2192 ${status} (${latency})\n",
	}
	if len(config) > 0 {
		cfg = config[0]
	}
	l := cfg.Logger
	if l == nil {
		l = log.Default()
	}
	tokens := parseLogFormat(cfg.Format)

	return func(ctx *fh.Ctx) error {
		start := time.Now()
		err := ctx.Next()
		lat := time.Since(start)

		buf := logBufPool.Get().(*bytes.Buffer)
		buf.Reset()
		buf.Grow(len(cfg.Format) + 64)

		for _, t := range tokens {
			switch t.typ {
			case logText:
				buf.WriteString(t.text)
			case logMethod:
				buf.Write(ctx.Header.Method)
			case logPath:
				uri := ctx.Header.URI
				if q := bytes.IndexByte(uri, '?'); q >= 0 {
					buf.Write(uri[:q])
				} else {
					buf.Write(uri)
				}
			case logStatus:
				appendInt(buf, ctx.StatusCode())
			case logLatency:
				us := lat.Microseconds()
				var lb [16]byte
				n := len(lb)
				for us > 0 {
					n--
					lb[n] = byte('0' + us%10)
					us /= 10
				}
				if n == len(lb) {
					lb[len(lb)-1] = '0'
					n = len(lb) - 1
				}
				buf.Write(lb[n:])
				buf.WriteString("\u00b5s")
			case logIP:
				buf.WriteString(ctx.IP())
			}
		}
		l.Output(2, buf.String())
		buf.Reset()
		logBufPool.Put(buf)
		return err
	}
}

type logTokenType uint8

const (
	logText logTokenType = iota
	logMethod
	logPath
	logStatus
	logLatency
	logIP
)

type logToken struct {
	typ  logTokenType
	text string
}

func parseLogFormat(format string) []logToken {
	tokens := make([]logToken, 0, 8)
	i := 0
	for i < len(format) {
		if format[i] == '$' && i+2 < len(format) && format[i+1] == '{' {
			end := strings.IndexByte(format[i:], '}')
			if end < 0 {
				tokens = append(tokens, logToken{typ: logText, text: format[i:]})
				break
			}
			name := format[i+2 : i+end]
			switch name {
			case "method":
				tokens = append(tokens, logToken{typ: logMethod})
			case "path":
				tokens = append(tokens, logToken{typ: logPath})
			case "status":
				tokens = append(tokens, logToken{typ: logStatus})
			case "latency":
				tokens = append(tokens, logToken{typ: logLatency})
			case "ip":
				tokens = append(tokens, logToken{typ: logIP})
			default:
				tokens = append(tokens, logToken{typ: logText, text: format[i : i+end+1]})
			}
			i += end + 1
		} else {
			start := i
			for i < len(format) && !(format[i] == '$' && i+2 < len(format) && format[i+1] == '{') {
				i++
			}
			tokens = append(tokens, logToken{typ: logText, text: format[start:i]})
		}
	}
	return tokens
}

func appendInt(buf *bytes.Buffer, n int) {
	if n < 1000 {
		switch n {
		case 0:
			buf.WriteByte('0')
			return
		case 1:
			buf.WriteByte('1')
			return
		case 2:
			buf.WriteByte('2')
			return
		case 3:
			buf.WriteByte('3')
			return
		case 4:
			buf.WriteByte('4')
			return
		case 5:
			buf.WriteByte('5')
			return
		case 6:
			buf.WriteByte('6')
			return
		case 7:
			buf.WriteByte('7')
			return
		case 8:
			buf.WriteByte('8')
			return
		case 9:
			buf.WriteByte('9')
			return
		}
	}
	var s [10]byte
	i := len(s)
	for n > 0 || i == len(s) {
		i--
		s[i] = byte('0' + n%10)
		n /= 10
	}
	buf.Write(s[i:])
}

var logBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

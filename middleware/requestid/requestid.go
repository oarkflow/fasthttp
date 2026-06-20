package requestid

import (
	"strconv"
	"sync/atomic"
	"time"

	fh "github.com/oarkflow/fasthttp"
)

var counter uint64
var prefix = strconv.FormatInt(time.Now().UnixNano(), 36)

func New() fh.HandlerFunc {
	return func(ctx *fh.Ctx) error {
		id := ctx.Get("X-Request-ID")
		if id == "" {
			n := atomic.AddUint64(&counter, 1)
			var buf [64]byte
			m := copy(buf[:], prefix)
			buf[m] = '-'
			m++
			m += len(strconv.AppendUint(buf[m:m], n, 36))
			id = string(buf[:m])
		}
		ctx.Set("X-Request-ID", id)
		ctx.Locals("requestID", id)
		return ctx.Next()
	}
}

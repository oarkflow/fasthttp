package timeout

import (
	"context"
	"errors"
	"time"

	fh "github.com/orgware/fasthttp"
)

func New(d time.Duration) fh.HandlerFunc {
	return func(ctx *fh.Ctx) error {
		deadline, cancel := context.WithTimeout(ctx.Context(), d)
		defer cancel()
		ctx.SetContext(deadline)
		err := ctx.Next()
		if errors.Is(deadline.Err(), context.DeadlineExceeded) && !ctx.Responded() {
			return ctx.Status(503).SendString("Request Timeout")
		}
		return err
	}
}

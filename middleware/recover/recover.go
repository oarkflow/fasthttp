package recover

import (
	"fmt"

	fh "github.com/orgware/fasthttp"
)

func New() fh.HandlerFunc {
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

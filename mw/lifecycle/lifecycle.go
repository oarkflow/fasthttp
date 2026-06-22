package lifecycle

import "github.com/oarkflow/fh"

type Hooks struct {
	OnRequestStart  func(*fh.Ctx)
	OnBeforeHandler func(*fh.Ctx)
	OnAfterHandler  func(*fh.Ctx)
	OnError         func(*fh.Ctx, error)
	OnRequestEnd    func(*fh.Ctx)
}

func New(h Hooks) fh.HandlerFunc {
	return func(c *fh.Ctx) error {
		if h.OnRequestStart != nil {
			h.OnRequestStart(c)
		}
		if h.OnBeforeHandler != nil {
			h.OnBeforeHandler(c)
		}
		err := c.Next()
		if err != nil && h.OnError != nil {
			h.OnError(c, err)
		}
		if h.OnAfterHandler != nil {
			h.OnAfterHandler(c)
		}
		if h.OnRequestEnd != nil {
			h.OnRequestEnd(c)
		}
		return err
	}
}

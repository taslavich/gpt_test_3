package web

import "sync/atomic"

type WorkController struct{ enabled atomic.Bool }

func NewWorkController(initial bool) *WorkController {
	c := &WorkController{}
	c.enabled.Store(initial)
	return c
}
func (c *WorkController) Enabled() bool    { return c == nil || c.enabled.Load() }
func (c *WorkController) Set(v bool) error { c.enabled.Store(v); return nil }

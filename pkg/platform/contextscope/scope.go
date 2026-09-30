// Package contextscope provides a policy-free, explicitly stoppable child
// context for work whose lifetime can end before its parent's lifetime.
package contextscope

import "context"

// Scope owns one child context. Call Stop when its work finishes, even if the
// parent has already been canceled.
type Scope struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func New(parent context.Context) *Scope {
	ctx, cancel := context.WithCancel(parent)
	return &Scope{ctx: ctx, cancel: cancel}
}

func (s *Scope) Context() context.Context { return s.ctx }

func (s *Scope) Stop() { s.cancel() }

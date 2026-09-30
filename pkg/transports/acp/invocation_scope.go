package acp

import "context"

// InvocationScope is the per-prompt cancellation capability supplied by the
// process composition root. ACP stops only the captured prompt's invocation
// after its Factory Session accepts a cancel control.
type InvocationScope interface {
	Context() context.Context
	Stop()
}

type InvocationScopeFactory func(context.Context) InvocationScope

package workers

import (
	"context"
	"time"
)

// LocalRuntimeHooks records model resource and load lifecycle observations
// produced by Workers. The Models runtime consumes the detached hooks only at
// the canonical process composition boundary.
type LocalRuntimeHooks struct {
	MarkResourceWaitStarted  func(context.Context, time.Time)
	MarkResourceWaitFinished func(context.Context, time.Time, bool)
	MarkLoadRequested        func(context.Context, time.Time)
	MarkLoadFinished         func(context.Context, time.Time)
	MarkLoadReused           func(context.Context)
}

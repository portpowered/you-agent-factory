package support

import "context"

// ControlledDeadlineContext lets a caller expire its request after observing
// attachment to a controlled external effect. Functional tests exercise the
// application's handling of DeadlineExceeded, without racing fixture startup
// against a short wall timer. Standard-library timer behavior needs no duplicate
// functional proof; already-expired deadlines can use context.WithDeadline.
func ControlledDeadlineContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	return controlledDeadlineContext{Context: ctx, values: context.WithoutCancel(parent)}, func() {
		cancel(context.DeadlineExceeded)
	}
}

type controlledDeadlineContext struct {
	context.Context
	values context.Context
}

// Keep child contexts on the public Done/Err contract. Exposing the embedded
// cancellation context's private value would propagate Canceled to children
// before this caller's DeadlineExceeded classification can be observed.
func (ctx controlledDeadlineContext) Value(key any) any {
	return ctx.values.Value(key)
}

func (ctx controlledDeadlineContext) Err() error {
	if ctx.Context.Err() != nil && context.Cause(ctx.Context) == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return ctx.Context.Err()
}

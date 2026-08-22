package work

import "context"

// commandDispatchContextKey carries one detached dispatch across a
// composition-owned command effect. The platform process request remains
// limited to host-process facts; owner-private adapters recover the Work
// correlation from this request-scoped context when they need it.
type commandDispatchContextKey struct{}

// WithCommandDispatch attaches a detached Work dispatch to one command
// execution context.
func WithCommandDispatch(ctx context.Context, dispatch WorkDispatch) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, commandDispatchContextKey{}, CloneWorkDispatch(dispatch))
}

// CommandDispatchFromContext resolves the detached Work dispatch, if one was
// attached to the command execution context.
func CommandDispatchFromContext(ctx context.Context) (WorkDispatch, bool) {
	if ctx == nil {
		return WorkDispatch{}, false
	}
	dispatch, ok := ctx.Value(commandDispatchContextKey{}).(WorkDispatch)
	if !ok {
		return WorkDispatch{}, false
	}
	return CloneWorkDispatch(dispatch), true
}

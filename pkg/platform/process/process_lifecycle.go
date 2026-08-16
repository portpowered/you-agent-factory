package process

import "context"

// ProcessLifecycleObserver receives policy-free facts about the parent
// process started for one command. It is intentionally narrower than a
// command result: an observer may learn that the process is gone while the
// command runner is still waiting for inherited pipes or other cleanup.
// Implementations must return promptly.
type ProcessLifecycleObserver interface {
	ProcessStarted(ProcessInfo)
	ProcessExited(ProcessInfo)
}

// ProcessInfo identifies the exact host process observed by the platform
// effect. PID is useful for diagnostics only; callers must not use it as a
// durable identity after the observation ends.
type ProcessInfo struct {
	PID int
}

type processLifecycleObserverContextKey struct{}

// WithProcessLifecycleObserver attaches one process observer to a command
// context. The value is an optional effect supplied by the caller; process
// execution remains policy-free and does not decide what an exit means.
func WithProcessLifecycleObserver(ctx context.Context, observer ProcessLifecycleObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, processLifecycleObserverContextKey{}, observer)
}

func processLifecycleObserverFromContext(ctx context.Context) ProcessLifecycleObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(processLifecycleObserverContextKey{}).(ProcessLifecycleObserver)
	return observer
}

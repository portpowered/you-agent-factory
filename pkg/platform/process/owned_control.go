package process

import "context"

// OwnedProcessControl is an ephemeral capability issued by a command runner
// after attaching an owned process tree. It never accepts a PID or resolves a
// replacement process. Implementations must expire before releasing the host
// identity that fences signals, and must serialize expiry against signaling.
//
// ForceKill returns true only after signaling the exact owned tree and proving
// that its entire execution has joined. An expired or unsupported capability
// returns false without effects. Signal failure, partial cleanup and an
// unconfirmed join return an error; sending a signal alone is never success.
// The caller's context bounds observation and does not retarget the capability.
type OwnedProcessControl interface {
	ForceKill(context.Context) (bool, error)
}

// OwnedProcessObserver receives the capability for one invocation, before
// provider output or a provider session reference is required. A runner unable
// to establish safe ownership must leave the observer uncalled. Observers must
// return promptly, and the capability must never be persisted or restored.
type OwnedProcessObserver func(OwnedProcessControl)

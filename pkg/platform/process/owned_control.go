package process

import (
	"context"
	"sync"
)

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

// ownedCommandControl serializes a single signal and its host-tree join with
// expiry. The host identity stays retained until expire returns. Command
// completion is a separate witness: it includes pipes and runner cleanup.
type ownedCommandControl struct {
	mu      sync.Mutex
	expired bool
	claimed bool
	stop    func(context.Context) (bool, error)
	done    <-chan struct{}
	waited  <-chan struct{}
}

func (control *ownedCommandControl) ForceKill(ctx context.Context) (bool, error) {
	control.mu.Lock()
	if err := ctx.Err(); err != nil {
		control.mu.Unlock()
		return false, err
	}
	if control.expired || control.claimed {
		control.mu.Unlock()
		return false, nil
	}
	control.claimed = true
	accepted, err := control.stop(ctx)
	control.mu.Unlock()
	if err != nil || !accepted {
		return false, err
	}
	if control.waited != nil {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-control.waited:
		}
	}
	// Do not hold the identity lock here: the runner must expire this
	// capability before closing its retained handles and publishing done.
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-control.done:
		if err := ctx.Err(); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (control *ownedCommandControl) expire() {
	if control == nil {
		return
	}
	control.mu.Lock()
	control.expired = true
	control.mu.Unlock()
}

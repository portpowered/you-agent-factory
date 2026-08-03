package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// errAttemptAlreadyLive reports that another execution already holds the
// exact canonical provider identity plus attempt ID this attempt requested.
var errAttemptAlreadyLive = errors.New("provider attempt id is already live for this provider")

// liveAttemptKey identifies one live execution by canonical provider identity
// plus the exact caller-supplied attempt ID. A control request must match
// both dimensions to reach this attempt and no other.
type liveAttemptKey struct {
	provider  providers.ID
	attemptID string
}

// nativeAttemptControl is the control handle bound for one in-flight native
// (non-ACP) provider attempt. cancel triggers the same context cancellation
// the execution seam already force-terminates its adapter subprocess/PTY
// session on (see the codex, claude, and agy adapters' shared reliance on
// context cancellation reaching pkg/platform/process's immediate kill path,
// and agypty's ctx.Done() select in session_run.go). done is closed only
// after the bound Execute call has returned, so signal can block a control
// caller until the controlled execution has actually observed its terminal
// behavior instead of merely requesting it.
type nativeAttemptControl struct {
	cancel context.CancelFunc
	done   <-chan struct{}
}

// supports reports whether action has a truthful native signal seam. Cancel
// and Terminate both resolve to the one force-kill mechanism every native
// adapter already honors; there is no per-attempt pause seam, so Pause is
// never supported here.
func (control *nativeAttemptControl) supports(action providers.ControlAction) bool {
	switch action {
	case providers.ControlActionCancel, providers.ControlActionTerminate:
		return true
	default:
		return false
	}
}

// signal delivers the already-validated action to the bound attempt and
// blocks until the execution observes its terminal behavior. Callers must
// only invoke signal after supports(action) reported true.
func (control *nativeAttemptControl) signal() {
	control.cancel()
	<-control.done
}

// liveAttemptEntry is the value held for one live identity. control is nil
// for attempts with no truthful exact-attempt signal seam bound at Execute
// time (for example, the ACP path in this packet).
type liveAttemptEntry struct {
	control *nativeAttemptControl
}

// liveAttemptRegistry correlates in-flight Execute calls with their canonical
// provider identity plus exact attempt ID, so a later control request can
// reach only the exact live attempt it names.
type liveAttemptRegistry struct {
	mu   sync.Mutex
	live map[liveAttemptKey]liveAttemptEntry
}

func newLiveAttemptRegistry() *liveAttemptRegistry {
	return &liveAttemptRegistry{live: make(map[liveAttemptKey]liveAttemptEntry)}
}

// bind registers key as live, with an optional control handle a later
// control request can claim, and returns a release function that removes it.
// bind fails with errAttemptAlreadyLive when key is already live, so a second
// execution cannot replace or steal an existing live identity. release is
// idempotent and safe to call from any terminal path, including a panic
// unwind, exactly once per successful bind; it is a no-op once claim has
// already removed key.
func (registry *liveAttemptRegistry) bind(key liveAttemptKey, control *nativeAttemptControl) (func(), error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.live[key]; exists {
		return nil, fmt.Errorf("%w: provider %q attempt %q", errAttemptAlreadyLive, key.provider, key.attemptID)
	}
	registry.live[key] = liveAttemptEntry{control: control}

	var once sync.Once
	release := func() {
		once.Do(func() {
			registry.mu.Lock()
			defer registry.mu.Unlock()
			delete(registry.live, key)
		})
	}
	return release, nil
}

// contains reports whether key is currently live. Exposed for direct
// same-package tests; peers observe liveness only indirectly through bind.
func (registry *liveAttemptRegistry) contains(key liveAttemptKey) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	_, exists := registry.live[key]
	return exists
}

// claim atomically removes key from the live set and returns its control
// handle, but only when key is currently live, bound a control handle, and
// that handle truthfully supports action. It leaves key live and reports
// ok=false for an unknown or already-terminal attempt, an attempt with no
// exact-attempt signal seam (for example ACP in this packet), or an
// unsupported action such as Pause, so no unintended side effect occurs and
// a later valid control for the same identity can still succeed.
//
// Because removal happens under the same mutex bind/release use, at most one
// caller ever wins a given identity: a concurrent duplicate control and a
// racing natural release (registry.bind's returned release) both delete
// under this lock, so exactly one of "a control claims it" or "it is already
// gone" is observable, never both.
func (registry *liveAttemptRegistry) claim(
	key liveAttemptKey,
	action providers.ControlAction,
) (*nativeAttemptControl, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	entry, exists := registry.live[key]
	if !exists || entry.control == nil || !entry.control.supports(action) {
		return nil, false
	}
	delete(registry.live, key)
	return entry.control, true
}

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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

// liveAttemptControl is the control handle bound for one live attempt's
// exact signal seam. Implementations report which actions have a truthful
// signal for the attempt right now (supports) and, once claim has already
// consumed the registration's control seam, deliver the signal and block
// (bounded by ctx) until the attempt's real recorded outcome is known
// (signal). For acpAttemptControl, supports does not merely pre-filter: it
// atomically claims the exact live execution generation (see
// acp.Service.Claim) and captures it for signal to use, so signal never
// re-derives liveness from canonical/attemptID strings - it operates only on
// the generation captured at claim time, which is what keeps a claimed
// control bound to that exact generation even if a later execution reuses
// the identical identity before signal runs. A natural completion racing a
// claimed control is still reported as accepted=false rather than a false
// ControlOutcomeCompleted, because the captured generation's own recorded
// outcome - not the bare fact that a signal was sent - grounds the result.
// signal returns a non-nil error only for a genuine delivery failure (see
// providers.ErrControlSignalFailed) or the caller's ctx ending before the
// outcome could be observed - both distinguishable from accepted=false,
// err=nil (unsupported/lost-the-race). nativeAttemptControl and
// acpAttemptControl are the two implementations; ControlAttempt and the
// registry operate on the interface and do not need to know which kind a
// given live identity bound.
type liveAttemptControl interface {
	supports(action providers.ControlAction) bool
	signal(ctx context.Context) (accepted bool, err error)
}

// nativeAttemptControl is the control handle bound for one in-flight native
// (non-ACP) provider attempt. cancel triggers the same context cancellation
// the execution seam already force-terminates its adapter subprocess/PTY
// session on (see the codex, claude, and agy adapters' shared reliance on
// context cancellation reaching pkg/platform/process's immediate kill path,
// and agypty's ctx.Done() select in session_run.go). finish is called by the
// bound Execute call as soon as it returns, recording cancelled - the
// attempt's real recorded outcome - strictly before done closes, mirroring
// acpAttemptControl/cancelwindow.Session.cancelled: a concurrent claim that
// lands after Execute already returned but before the registry entry is
// released still only ever observes the true recorded outcome once signal
// waits on done, so a natural success can never be misreported as
// ControlOutcomeCompleted merely because it won that race.
type nativeAttemptControl struct {
	cancel         context.CancelFunc
	done           chan struct{}
	cancelled      bool
	mu             sync.Mutex
	allowKill      bool
	finished       bool
	attached       bool
	process        platformprocess.OwnedProcessControl
	claimedProcess platformprocess.OwnedProcessControl
	killResolved   chan struct{}
	killConfirmed  bool
}

var _ liveAttemptControl = (*nativeAttemptControl)(nil)

// supports claims the attached capability for Kill. Cancel and Terminate
// retain the existing context-cancellation seam; Pause remains unsupported.
func (control *nativeAttemptControl) supports(action providers.ControlAction) bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	switch action {
	case providers.ControlActionCancel, providers.ControlActionTerminate:
		control.claimedProcess = nil
		return true
	case providers.ControlActionKill:
		if !control.allowKill || control.finished || control.process == nil {
			return false
		}
		control.claimedProcess = control.process
		control.killResolved = make(chan struct{})
		return true
	default:
		return false
	}
}

// signal invokes the captured kill capability or the existing cancellation
// seam, then joins the authoritative provider execution. Kill never falls
// back to cancellation. Ordinary cancellation remains grounded in the real
// recorded canceled outcome, so a concurrent natural finish is not success.
func (control *nativeAttemptControl) signal(ctx context.Context) (bool, error) {
	control.mu.Lock()
	process := control.claimedProcess
	control.mu.Unlock()
	if process != nil {
		return control.killAndJoin(ctx, process)
	}
	control.cancel()
	select {
	case <-control.done:
		return control.cancelled, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// finish records cancelled - whether the bound Execute call's own result
// reflects genuine cancellation - and then closes done, in that order, so
// any signal call unblocked by done can read cancelled without its own
// synchronization (the channel close/receive already establishes the
// happens-before). Must be called exactly once, synchronously, as soon as
// the bound Execute call returns.
func (control *nativeAttemptControl) finish(cancelled bool) {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.finished = true
	control.process = nil
	control.cancelled = cancelled
	close(control.done)
}

// attachProcess retains only the first capability for this physical attempt.
// A late callback cannot install authority after completion or substitute a
// second process for the handle a concurrent control already captured.
func (control *nativeAttemptControl) attachProcess(process platformprocess.OwnedProcessControl) {
	control.mu.Lock()
	defer control.mu.Unlock()
	if process != nil && control.allowKill && !control.finished && !control.attached {
		control.process = process
		control.attached = true
	}
}

func (control *nativeAttemptControl) killAndJoin(ctx context.Context, process platformprocess.OwnedProcessControl) (accepted bool, err error) {
	defer func() {
		control.mu.Lock()
		defer control.mu.Unlock()
		control.killConfirmed = accepted && err == nil
		close(control.killResolved)
	}()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	accepted, err = process.ForceKill(ctx)
	if err != nil {
		return false, fmt.Errorf("%w: %w", providers.ErrControlSignalFailed, err)
	}
	if !accepted {
		// A declined capability guarantees no effects. Retire it permanently;
		// ordinary cancellation can still control this same live execution.
		control.mu.Lock()
		control.process = nil
		control.claimedProcess = nil
		control.mu.Unlock()
		return false, nil
	}
	// Tree completion and provider completion are independent witnesses. Never
	// turn a successful signal or a parent-only join into a completed attempt.
	select {
	case <-control.done:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// finishExecution publishes the adapter join before waiting for a claimed kill
// to resolve. This lets killAndJoin observe completion without releasing a
// retryable adapter error to Workers while exact-tree confirmation is pending.
// Only a confirmed kill changes the error; refusals and failures retain the
// adapter's natural outcome.
func (control *nativeAttemptControl) finishExecution(err error) error {
	control.finish(errors.Is(err, providers.ErrExecuteCancelled))
	control.mu.Lock()
	resolved := control.killResolved
	control.mu.Unlock()
	if resolved == nil {
		return err
	}
	<-resolved
	control.mu.Lock()
	confirmed := control.killConfirmed
	control.mu.Unlock()
	if confirmed {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled, Message: "owned provider execution force terminated"}
	}
	return err
}

// liveAttemptEntry is the value held for one live identity. control is nil
// only for a live identity with no signal handle bound at all; every
// production Execute path in this packet binds one (see bindLiveAttempt).
type liveAttemptEntry struct {
	control liveAttemptControl
	claimed bool
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
// unwind, exactly once per successful bind. A claimed key stays reserved
// until this release, so the closure cannot erase a replacement execution.
func (registry *liveAttemptRegistry) bind(key liveAttemptKey, control liveAttemptControl) (func(), error) {
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

// contains reports whether key still has an unclaimed control seam. A claimed
// key remains reserved for execution until release, but cannot be claimed again.
func (registry *liveAttemptRegistry) contains(key liveAttemptKey) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry, exists := registry.live[key]
	return exists && !entry.claimed
}

// claim atomically consumes key's control seam and returns its control
// handle, but only when key is currently live, bound a control handle, and
// that handle truthfully supports action right now. It leaves key live and
// reports ok=false for an unknown or already-terminal attempt, an attempt
// with no exact-attempt signal seam, an attempt whose seam is not yet (or no
// longer) truthfully live (for example an ACP attempt before its session/
// prompt turn has started), or an unsupported action such as Pause, so no
// unintended side effect occurs and a later valid control for the same
// identity can still succeed.
//
// Claim and natural release share the registry lock, so at most one caller
// obtains the seam. The owning execution keeps the identity reserved through
// its join, including when the caller stops observing a claimed control.
func (registry *liveAttemptRegistry) claim(
	key liveAttemptKey,
	action providers.ControlAction,
) (liveAttemptControl, bool) {
	return registry.claimMatching(key, action, nil)
}

// A captured handle compares the registration before consuming its signal
// seam. Reusing even the identical provider/attempt tuple grants no authority
// to an observer retained from the previous execution.
func (registry *liveAttemptRegistry) claimMatching(key liveAttemptKey, action providers.ControlAction, expected *nativeAttemptControl) (liveAttemptControl, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	entry, exists := registry.live[key]
	if !exists || entry.claimed || entry.control == nil || (expected != nil && entry.control != expected) || !entry.control.supports(action) {
		return nil, false
	}
	// Keep the identity reserved until the owning Execute call releases it.
	// Otherwise a replacement can bind this key while the claimed signal is
	// still joining, and the old release closure can erase its registration.
	entry.claimed = true
	registry.live[key] = entry
	return entry.control, true
}

type nativeAttemptHandle struct {
	service *Service
	key     liveAttemptKey
	control *nativeAttemptControl
}

func (handle nativeAttemptHandle) ForceKill(ctx context.Context) (bool, error) {
	result, err := handle.service.controlAttempt(ctx, providers.ControlAttemptRequest{
		Provider: handle.key.provider, AttemptID: handle.key.attemptID, Action: providers.ControlActionKill,
	}, handle.control)
	return err == nil && result.Outcome == providers.ControlOutcomeCompleted, err
}

func (s *Service) publishAttemptControl(request providers.ExecuteRequest, control *nativeAttemptControl) {
	if request.AttemptControlObserver != nil && control.allowKill {
		request.AttemptControlObserver(nativeAttemptHandle{
			service: s, key: liveAttemptKey{provider: request.Provider, attemptID: request.AttemptID}, control: control,
		})
	}
}

// restoreDeclinedKill reopens ordinary control only for the registration that
// declined a kill without effects. A natural release and replacement bind must
// never let the old caller reopen the replacement's already-claimed seam.
func (registry *liveAttemptRegistry) restoreDeclinedKill(key liveAttemptKey, control *nativeAttemptControl) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry, exists := registry.live[key]
	if exists && entry.control == control {
		entry.claimed = false
		registry.live[key] = entry
	}
}

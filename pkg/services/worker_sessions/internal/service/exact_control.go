package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// frozenControlTarget stays fixed while publication or another control finishes.
// A retry must never resolve a newer attempt from the stable Worker Session ID.
type frozenControlTarget struct {
	supervision *supervision
	runtime     *runtimeAttempt
	dispatchID  string
	publication *publication
	capture     recordings.WorkerControlTarget
}

func (r *registry) freezeControlTarget(id string) (frozenControlTarget, error) {
	r.mu.RLock()
	if _, exists := r.sessions[id]; !exists {
		r.mu.RUnlock()
		return frozenControlTarget{}, workersessions.ErrSessionNotFound
	}
	target := frozenControlTarget{supervision: r.supervisions[id], runtime: r.runtimeAttemptControls[id], publication: r.publications[id]}
	if target.supervision != nil {
		target.supervision.mu.Lock()
		target.dispatchID = target.supervision.dispatchID
		target.supervision.mu.Unlock()
	} else if target.runtime != nil {
		target.dispatchID = target.runtime.dispatchID
	}
	r.mu.RUnlock()
	if target.publication != nil {
		target.publication.mu.Lock()
		target.capture = target.publication.capture
		target.publication.mu.Unlock()
	}
	return target, nil
}

// claimFrozenCancellation compares and claims under the same registry/attempt
// locks used to replace an attempt during retry or resume. Locks are released
// before any callback, event publication or join.
func (r *registry) claimFrozenCancellation(
	id string, action workersessions.ControlAction, target frozenControlTarget,
) (workersessions.Session, cancellationAttempt, error) {
	unlock, err := r.lockFrozenCapture(id, target)
	if err != nil {
		r.mu.RLock()
		session := cloneSession(r.sessions[id])
		r.mu.RUnlock()
		return session, cancellationAttempt{}, err
	}
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, exists := r.sessions[id]
	if !exists {
		return workersessions.Session{}, cancellationAttempt{}, workersessions.ErrSessionNotFound
	}
	if r.supervisions[id] != target.supervision || r.runtimeAttemptControls[id] != target.runtime {
		return cloneSession(session), cancellationAttempt{}, staleControlTargetError()
	}
	if target.supervision == nil {
		return cloneSession(session), cancellationAttempt{}, nil
	}
	s := target.supervision
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dispatchID != target.dispatchID {
		return cloneSession(session), cancellationAttempt{}, staleControlTargetError()
	}
	if session.Terminal() {
		return cloneSession(session), cancellationAttempt{kind: cancellationAttemptNoop}, nil
	}
	if session.State == workersessions.StatePaused && s.controlAction != "" {
		return cloneSession(session), cancellationAttempt{kind: cancellationAttemptNoop}, nil
	}
	if session.State == workersessions.StatePaused && !s.controlActive && !s.interrupting {
		s.controlAction = action
		return cloneSession(session), cancellationAttempt{kind: cancellationAttemptPaused}, nil
	}
	return cloneSession(session), s.beginCancellationLocked(action), nil
}

// Registry ownership and the attempt's control claim use the admission lock
// order. A wait never grants a stale handle authority over a replacement.
func (r *registry) claimFrozenRuntimeControl(id string, target frozenControlTarget) (bool, <-chan struct{}, <-chan struct{}, error) {
	unlock, err := r.lockFrozenCapture(id, target)
	if err != nil {
		return false, nil, nil, err
	}
	defer unlock()
	attempt := target.runtime
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.frozenRuntimeOwnerLocked(id, attempt); err != nil {
		return false, nil, nil, err
	}
	claimed, wait, completed := attempt.claimControl()
	return claimed, wait, completed, nil
}

// Natural completion removes the live handle before releasing joiners. That
// exact completed handle may still produce NOOP; no live replacement may.
func (r *registry) frozenRuntimeOwnerLocked(id string, attempt *runtimeAttempt) error {
	session, exists := r.sessions[id]
	if !exists {
		return workersessions.ErrSessionNotFound
	}
	if attempt.workerID != id {
		return staleControlTargetError()
	}
	current := r.runtimeAttemptControls[id]
	if current == nil && session.Terminal() {
		attempt.mu.Lock()
		completing := attempt.completing
		attempt.mu.Unlock()
		if completing {
			return nil
		}
	}
	if current != attempt || r.runtimeAttemptOwners[attempt.key] != id ||
		r.latestRuntimeDispatchIDs[id] != attempt.dispatchID {
		return staleControlTargetError()
	}
	return nil
}

// The control claim pins normal completion. Recheck the registry after history
// publication, then release all locks before invoking the external stop edge.
func (r *registry) frozenRuntimeCancel(id string, target frozenControlTarget) (func(context.Context) (workers.WorkstationDispatchCancelOutcome, error), error) {
	unlock, err := r.lockFrozenCapture(id, target)
	if err != nil {
		return nil, err
	}
	defer unlock()
	attempt := target.runtime
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.frozenRuntimeOwnerLocked(id, attempt); err != nil {
		return nil, err
	}
	return attempt.controlCancel(context.Background())
}

func staleControlTargetError() error {
	return fmt.Errorf("%w: accepted control no longer owns the active attempt", workersessions.ErrInvalidState)
}

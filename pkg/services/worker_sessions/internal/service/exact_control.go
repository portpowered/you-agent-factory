package service

import (
	"fmt"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// frozenControlTarget stays fixed while publication or another control finishes.
// A retry must never resolve a newer attempt from the stable Worker Session ID.
type frozenControlTarget struct {
	supervision *supervision
	dispatchID  string
}

func (r *registry) freezeControlTarget(id string) (frozenControlTarget, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, exists := r.sessions[id]; !exists {
		return frozenControlTarget{}, workersessions.ErrSessionNotFound
	}
	target := frozenControlTarget{supervision: r.supervisions[id]}
	if target.supervision != nil {
		target.supervision.mu.Lock()
		target.dispatchID = target.supervision.dispatchID
		target.supervision.mu.Unlock()
	}
	return target, nil
}

// claimFrozenCancellation compares and claims under the same registry/attempt
// locks used to replace an attempt during retry or resume. Locks are released
// before any callback, event publication or join.
func (r *registry) claimFrozenCancellation(
	id string, action workersessions.ControlAction, target frozenControlTarget,
) (workersessions.Session, cancellationAttempt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, exists := r.sessions[id]
	if !exists {
		return workersessions.Session{}, cancellationAttempt{}, workersessions.ErrSessionNotFound
	}
	if r.supervisions[id] != target.supervision {
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

func staleControlTargetError() error {
	return fmt.Errorf("%w: accepted control no longer owns the active attempt", workersessions.ErrInvalidState)
}

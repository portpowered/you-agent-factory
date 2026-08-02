package chatsessions

import "time"

// This file enforces the L1 V0 lifecycle transition tables from
// docs/internal/projects/acp-client/final-proposal.md section 4.2. Every
// CanTransitionTo method is pure: it never mutates its receiver or argument,
// and it reports the state's own Validate error (an invalid-state outcome)
// before ever reporting ErrInvalidTransition (an invalid-transition
// outcome), so callers can distinguish an unrecognized state from a legal
// state pair that the L1 V0 table does not permit.

// CanTransitionTo reports whether moving a Session from s to next is a legal
// L1 V0 transition: CREATED->ACTIVE (first turn admitted), CREATED->CLOSED,
// and ACTIVE->CLOSED. CLOSED is terminal; every other pair, including a
// self-transition, is rejected as an invalid transition.
func (s SessionState) CanTransitionTo(next SessionState) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	switch {
	case s == SessionStateCreated && next == SessionStateActive:
		return nil
	case s == SessionStateCreated && next == SessionStateClosed:
		return nil
	case s == SessionStateActive && next == SessionStateClosed:
		return nil
	default:
		return newTransitionError("SessionState", string(s), string(next))
	}
}

// CanTransitionTo reports whether moving a TargetEpisode from s to next is a
// legal L1 V0 transition: OPEN->CLOSED only. CLOSED is terminal and never
// accepts a transition back to OPEN.
func (s TargetEpisodeState) CanTransitionTo(next TargetEpisodeState) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if s == TargetEpisodeStateOpen && next == TargetEpisodeStateClosed {
		return nil
	}
	return newTransitionError("TargetEpisodeState", string(s), string(next))
}

// CanTransitionTo reports whether moving a Turn from s to next is a legal L1
// V0 transition: ADMITTED->RUNNING, ADMITTED->CANCELED, and
// RUNNING->{COMPLETED,FAILED,CANCELED}. COMPLETED, FAILED, and CANCELED are
// terminal and accept no further transition.
func (s TurnState) CanTransitionTo(next TurnState) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	switch {
	case s == TurnStateAdmitted && next == TurnStateRunning:
		return nil
	case s == TurnStateAdmitted && next == TurnStateCanceled:
		return nil
	case s == TurnStateRunning && next == TurnStateCompleted:
		return nil
	case s == TurnStateRunning && next == TurnStateFailed:
		return nil
	case s == TurnStateRunning && next == TurnStateCanceled:
		return nil
	default:
		return newTransitionError("TurnState", string(s), string(next))
	}
}

// CanTransitionTo reports whether moving a ControlIntent from s to next is a
// legal L1 V0 transition: REQUESTED->COMMITTED, and
// COMMITTED->{COMPLETED,NOOP,SUPERSEDED}. COMPLETED, NOOP, and SUPERSEDED are
// terminal outcomes and remain distinct from each other: NOOP means the
// captured turn was already terminal on arrival, SUPERSEDED means the
// captured turn was no longer current and the intent was never applied.
func (s ControlIntentState) CanTransitionTo(next ControlIntentState) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	switch {
	case s == ControlIntentStateRequested && next == ControlIntentStateCommitted:
		return nil
	case s == ControlIntentStateCommitted && next == ControlIntentStateCompleted:
		return nil
	case s == ControlIntentStateCommitted && next == ControlIntentStateNoop:
		return nil
	case s == ControlIntentStateCommitted && next == ControlIntentStateSuperseded:
		return nil
	default:
		return newTransitionError("ControlIntentState", string(s), string(next))
	}
}

// CloseTargetEpisode returns a new TargetEpisode with prior's OPEN episode
// transitioned to CLOSED at closedAt, without modifying prior. It reports the
// same typed error CanTransitionTo would (an invalid-state outcome for a
// zero/unknown prior.State, or an invalid-transition outcome for a prior that
// is already CLOSED) and returns prior unchanged on any error.
func CloseTargetEpisode(prior TargetEpisode, closedAt time.Time) (TargetEpisode, error) {
	if err := prior.State.CanTransitionTo(TargetEpisodeStateClosed); err != nil {
		return prior, err
	}
	closed := prior
	closed.State = TargetEpisodeStateClosed
	closed.ClosedAt = &closedAt
	return closed, nil
}

// OpenNextTargetEpisode returns a new TargetEpisode selecting target,
// numbered prior.Number+1, in state OPEN. Target episodes are immutable
// historical identities: this never rewrites prior's Target, Number, or
// State, and it requires prior to already be CLOSED -- a target change is
// represented by closing the current episode first (CloseTargetEpisode) and
// then opening the next one, never by mutating the old episode in place. A
// zero or unknown prior.State reports prior's own typed invalid-state error
// (from Validate); a validated but still-OPEN prior reports
// *TargetEpisodeNotClosedError, distinct from that invalid-state outcome.
func OpenNextTargetEpisode(prior TargetEpisode, target ChatTargetRef, startedAt time.Time) (TargetEpisode, error) {
	if err := prior.State.Validate(); err != nil {
		return TargetEpisode{}, err
	}
	if !prior.State.IsTerminal() {
		return TargetEpisode{}, &TargetEpisodeNotClosedError{Number: prior.Number, State: prior.State}
	}
	if err := target.Validate(); err != nil {
		return TargetEpisode{}, err
	}
	return TargetEpisode{
		Number:           prior.Number + 1,
		State:            TargetEpisodeStateOpen,
		Target:           target,
		FactorySessionID: prior.FactorySessionID,
		StartedAt:        startedAt,
	}, nil
}

// ResolveControlIntentOutcome is the pure captured-turn race rule a
// ControlIntent's COMMITTED->{COMPLETED,NOOP,SUPERSEDED} advancement must
// follow. It evaluates only the identities and facts captured when the
// intent was requested (capturedTurnID, capturedTurnState) against the
// Session's currentActiveTurnID at completion time; it never rebinds the
// intent to a different turn.
//
// A captured turn that is no longer the session's current active turn
// resolves to SUPERSEDED regardless of capturedTurnState -- including when
// capturedTurnState is itself already terminal, since "no longer current" is
// evaluated first and takes precedence. Otherwise, an already-terminal
// captured turn (one that finished before the intent completed) resolves to
// NOOP, since there is nothing left to cancel or close; a still-active
// captured turn resolves to COMPLETED.
func ResolveControlIntentOutcome(capturedTurnID string, capturedTurnState TurnState, currentActiveTurnID string) ControlIntentState {
	if capturedTurnID != currentActiveTurnID {
		return ControlIntentStateSuperseded
	}
	if capturedTurnState.IsTerminal() {
		return ControlIntentStateNoop
	}
	return ControlIntentStateCompleted
}

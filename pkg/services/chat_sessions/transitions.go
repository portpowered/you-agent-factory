package chatsessions

import (
	"math"
	"time"
)

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
// transitioned to CLOSED at closedAt, without modifying prior. prior itself
// is fully validated (TargetEpisode.Validate()) before any transition is
// attempted, so a structurally corrupt prior -- for example an OPEN episode
// that already carries a non-nil ClosedAt -- reports prior's own typed error
// instead of being silently repaired by the overwrite below. It then reports
// the same typed error CanTransitionTo would (an invalid-transition outcome
// for a prior that is already CLOSED) and returns prior unchanged on any
// error, including when closedAt is zero or precedes prior.StartedAt -- the
// candidate closed value is validated with TargetEpisode.Validate() before it
// is ever returned, so a caller can never observe a TargetEpisode that fails
// its own invariants.
func CloseTargetEpisode(prior TargetEpisode, closedAt time.Time) (TargetEpisode, error) {
	if err := prior.Validate(); err != nil {
		return prior, err
	}
	if err := prior.State.CanTransitionTo(TargetEpisodeStateClosed); err != nil {
		return prior, err
	}
	closed := prior
	closed.State = TargetEpisodeStateClosed
	closed.ClosedAt = &closedAt
	if err := closed.Validate(); err != nil {
		return prior, err
	}
	return closed, nil
}

// OpenNextTargetEpisode returns a new TargetEpisode selecting target and
// factorySessionID, numbered prior.Number+1, in state OPEN. Target episodes
// are immutable historical identities: this never rewrites prior's Target,
// Number, State, or FactorySessionID, and it requires prior to already be
// CLOSED -- a target change is represented by closing the current episode
// first (CloseTargetEpisode) and then opening the next one, never by
// mutating the old episode in place. factorySessionID is the new episode's
// own Factory Session reference, supplied by the caller rather than copied
// from prior, since a target change can select a different Factory Session
// than the one the prior episode ran against; a caller with no Factory
// Session to associate yet may pass the empty string. A zero or unknown
// prior.State reports prior's own typed invalid-state error (from Validate).
// Before the OPEN/CLOSED precondition is ever checked, prior is validated in
// full (TargetEpisode.Validate()), so a structurally corrupt source episode
// -- in any state -- an inconsistent ClosedAt for its State, an invalid
// Target, a zero StartedAt, or a ClosedAt before StartedAt -- reports prior's
// own typed error rather than being silently accepted or misreported as
// merely "not closed". Only once prior is confirmed fully valid does a
// still-OPEN prior report *TargetEpisodeNotClosedError, distinct from any
// invalid-state or invalid-value outcome. A prior.Number already at
// math.MaxUint64 is rejected with a typed ErrTargetEpisodeNumberExhausted
// validation error before prior.Number+1 is ever computed, so the next
// episode number can never silently wrap to 0. startedAt that is zero, or
// that precedes prior.ClosedAt, and any other invariant
// TargetEpisode.Validate() enforces on the candidate next value (including
// an invalid target) are rejected before the candidate value is ever
// returned.
func OpenNextTargetEpisode(prior TargetEpisode, target ChatTargetRef, factorySessionID string, startedAt time.Time) (TargetEpisode, error) {
	if err := prior.State.Validate(); err != nil {
		return TargetEpisode{}, err
	}
	if err := prior.Validate(); err != nil {
		return TargetEpisode{}, err
	}
	if !prior.State.IsTerminal() {
		return TargetEpisode{}, &TargetEpisodeNotClosedError{Number: prior.Number, State: prior.State}
	}
	if prior.Number == math.MaxUint64 {
		return TargetEpisode{}, newValidationError("TargetEpisode", "Number", ErrTargetEpisodeNumberExhausted)
	}
	next := TargetEpisode{
		Number:           prior.Number + 1,
		State:            TargetEpisodeStateOpen,
		Target:           target,
		FactorySessionID: factorySessionID,
		StartedAt:        startedAt,
	}
	if err := next.Validate(); err != nil {
		return TargetEpisode{}, err
	}
	if prior.ClosedAt != nil && startedAt.Before(*prior.ClosedAt) {
		return TargetEpisode{}, newValidationError("TargetEpisode", "StartedAt", ErrInconsistentValue)
	}
	return next, nil
}

// ResolveControlIntentOutcome is the pure captured-turn race rule a
// ControlIntent's COMMITTED->{COMPLETED,NOOP,SUPERSEDED} advancement must
// follow. It evaluates only the identities and facts captured when the
// intent was requested (capturedTurnID, capturedTurnState) against the
// Session's currentActiveTurnID at completion time; it never rebinds the
// intent to a different turn.
//
// This helper accepts raw captured facts rather than a validated
// ControlIntent, so it independently enforces the same required-value rule
// ControlIntent.Validate() applies to TurnID: a blank capturedTurnID always
// reports a typed required-value error before any outcome is selected --
// completion is only ever valid for a concrete captured turn, and an
// identity comparison against a blank capturedTurnID could otherwise resolve
// a meaningless SUPERSEDED/COMPLETED outcome. capturedTurnState is validated
// next, before any outcome is selected, so a zero or unknown captured state
// always reports a typed invalid-state error -- an identity mismatch
// (capturedTurnID no longer the current active turn) never hides an invalid
// captured state behind a SUPERSEDED outcome. Once both facts are valid, a
// captured turn that is no longer the session's current active turn resolves
// to SUPERSEDED regardless of capturedTurnState -- including when
// capturedTurnState is itself already terminal, since "no longer current" is
// evaluated first and takes precedence. Otherwise, an already-terminal
// captured turn (one that finished before the intent completed) resolves to
// NOOP, since there is nothing left to cancel or close; a still-active
// captured turn resolves to COMPLETED.
func ResolveControlIntentOutcome(capturedTurnID string, capturedTurnState TurnState, currentActiveTurnID string) (ControlIntentState, error) {
	if capturedTurnID == "" {
		return "", newValidationError("ControlIntent", "TurnID", ErrRequiredValue)
	}
	if err := capturedTurnState.Validate(); err != nil {
		return "", err
	}
	if capturedTurnID != currentActiveTurnID {
		return ControlIntentStateSuperseded, nil
	}
	if capturedTurnState.IsTerminal() {
		return ControlIntentStateNoop, nil
	}
	return ControlIntentStateCompleted, nil
}

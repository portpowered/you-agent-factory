package chatsessions

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

package workersessions

import (
	"fmt"
	"strings"
)

// Session is an immutable snapshot of one Worker Session's stable identity,
// current lifecycle state, and — once terminal — its committed TerminalResult.
// Session is a plain value; callers that mutate a returned Session, or its
// Result, never affect registry-owned state.
type Session struct {
	ID    string
	State State
	// Result is non-nil exactly when State is StateCompleted or StateFailed,
	// and carries the exactly-once committed terminal outcome. Result is nil
	// for every non-terminal state, and for the W1 CANCELED/TERMINATED
	// states that W2 does not produce.
	Result *TerminalResult
	// ProviderSessionAssociation is the optional exact Providers-owned
	// reference observed for this session's supervised attempt. A nil value is
	// explicit absence: Worker Sessions never synthesizes it from a runner,
	// model, current provider, or bare session ID.
	ProviderSessionAssociation *ProviderSessionAssociation
}

// Validate reports whether s has a non-empty stable identity, exactly one
// accepted lifecycle state, and a Result that is present if and only if
// State is StateCompleted or StateFailed and, when present, agrees with
// State. Validate is pure and does not mutate s.
func (s Session) Validate() error {
	if !validSessionID(s.ID) {
		return ErrInvalidSessionID
	}
	if !s.State.Valid() {
		return ErrInvalidState
	}
	switch s.State {
	case StateCompleted, StateFailed:
		if s.Result == nil {
			return fmt.Errorf("%w: terminal state requires a non-nil TerminalResult", ErrInvalidTerminalResult)
		}
		if err := s.Result.Validate(); err != nil {
			return err
		}
		if (s.State == StateCompleted) != (s.Result.Outcome == TerminalOutcomeCompleted) {
			return fmt.Errorf("%w: state and TerminalResult outcome disagree", ErrInvalidTerminalResult)
		}
	default:
		if s.Result != nil {
			return fmt.Errorf("%w: non-terminal state must not carry a TerminalResult", ErrInvalidTerminalResult)
		}
	}
	if s.ProviderSessionAssociation != nil {
		if err := s.ProviderSessionAssociation.Validate(); err != nil {
			return err
		}
		if s.ProviderSessionAssociation.WorkerSessionID != s.ID {
			return fmt.Errorf("%w: provider session association worker session id disagrees", ErrInvalidProviderSessionAssociation)
		}
	}
	return nil
}

// Terminal reports whether s is currently in one of the four absorbing
// terminal states.
func (s Session) Terminal() bool {
	return s.State.Terminal()
}

func validSessionID(id string) bool {
	return strings.TrimSpace(id) != ""
}

// State is the exact eight-value Worker Session lifecycle vocabulary. No
// other value, including INTERRUPTED, is accepted anywhere W1 validates a
// state.
type State string

const (
	StateReserved   State = "RESERVED"
	StateStarting   State = "STARTING"
	StateRunning    State = "RUNNING"
	StatePaused     State = "PAUSED"
	StateCompleted  State = "COMPLETED"
	StateFailed     State = "FAILED"
	StateCanceled   State = "CANCELED"
	StateTerminated State = "TERMINATED"
)

// Valid reports whether s is one of the eight accepted lifecycle states.
func (s State) Valid() bool {
	switch s {
	case StateReserved, StateStarting, StateRunning, StatePaused, StateCompleted, StateFailed, StateCanceled, StateTerminated:
		return true
	default:
		return false
	}
}

// Terminal reports whether s is one of the four absorbing terminal states:
// COMPLETED, FAILED, CANCELED, or TERMINATED. Transition rules into or out of
// a terminal state are outside W1.
func (s State) Terminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCanceled, StateTerminated:
		return true
	default:
		return false
	}
}

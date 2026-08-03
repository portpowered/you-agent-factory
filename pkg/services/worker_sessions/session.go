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

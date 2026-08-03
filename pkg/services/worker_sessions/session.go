package workersessions

import "strings"

// Session is an immutable snapshot of one Worker Session's stable identity
// and current lifecycle state. Session is a plain value; callers that mutate
// a returned Session never affect registry-owned state.
type Session struct {
	ID    string
	State State
}

// Validate reports whether s has a non-empty stable identity and exactly one
// accepted lifecycle state. Validate is pure and does not mutate s.
func (s Session) Validate() error {
	if !validSessionID(s.ID) {
		return ErrInvalidSessionID
	}
	if !s.State.Valid() {
		return ErrInvalidState
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

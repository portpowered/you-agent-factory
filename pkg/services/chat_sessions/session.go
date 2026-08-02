package chatsessions

import "time"

// SessionState is the lifecycle state of a Chat Session.
type SessionState string

const (
	// SessionStateCreated is the initial state before any turn is admitted.
	SessionStateCreated SessionState = "CREATED"
	// SessionStateActive means at least one turn has been admitted.
	SessionStateActive SessionState = "ACTIVE"
	// SessionStateClosed is terminal; no further turns are admitted.
	SessionStateClosed SessionState = "CLOSED"
)

// Validate reports whether s is one of the exactly declared SessionState
// values. The zero value and any unknown value are rejected.
func (s SessionState) Validate() error {
	switch s {
	case SessionStateCreated, SessionStateActive, SessionStateClosed:
		return nil
	default:
		return &InvalidSessionStateError{State: s}
	}
}

var legalSessionStateTransitions = map[SessionState]map[SessionState]bool{
	SessionStateCreated: {SessionStateActive: true, SessionStateClosed: true},
	SessionStateActive:  {SessionStateClosed: true},
	SessionStateClosed:  {},
}

// TransitionSessionState validates a proposed SessionState transition and
// returns the next state. On any invalid-value or invalid-transition error it
// returns from unchanged; from and to are never mutated.
func TransitionSessionState(from, to SessionState) (SessionState, error) {
	if err := from.Validate(); err != nil {
		return from, err
	}
	if err := to.Validate(); err != nil {
		return from, err
	}
	if legalSessionStateTransitions[from][to] {
		return to, nil
	}
	return from, &InvalidSessionStateTransitionError{From: from, To: to}
}

// Session is the public, transport-neutral identity and lifecycle fact set
// for a Chat Session. It embeds no store or transport type.
type Session struct {
	ID             string
	State          SessionState
	SelectedTarget ChatTargetRef
	TargetEpisode  uint64
	ActiveTurnID   string
	Version        uint64
	StreamHead     uint64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

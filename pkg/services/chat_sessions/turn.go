package chatsessions

// TurnState is the lifecycle state of one Chat Session turn.
type TurnState string

const (
	// TurnStateAdmitted is the initial state once admission succeeds.
	TurnStateAdmitted TurnState = "ADMITTED"
	// TurnStateRunning means downstream invocation has been accepted.
	TurnStateRunning TurnState = "RUNNING"
	// TurnStateCompleted is terminal success.
	TurnStateCompleted TurnState = "COMPLETED"
	// TurnStateFailed is terminal failure.
	TurnStateFailed TurnState = "FAILED"
	// TurnStateCanceled is terminal cancellation.
	TurnStateCanceled TurnState = "CANCELED"
)

// Validate reports whether s is one of the exactly declared TurnState values.
// The zero value and any unknown value are rejected.
func (s TurnState) Validate() error {
	switch s {
	case TurnStateAdmitted, TurnStateRunning, TurnStateCompleted, TurnStateFailed, TurnStateCanceled:
		return nil
	default:
		return &InvalidTurnStateError{State: s}
	}
}

// terminal reports whether s accepts no further transitions.
func (s TurnState) terminal() bool {
	switch s {
	case TurnStateCompleted, TurnStateFailed, TurnStateCanceled:
		return true
	default:
		return false
	}
}

var legalTurnStateTransitions = map[TurnState]map[TurnState]bool{
	TurnStateAdmitted:  {TurnStateRunning: true, TurnStateCanceled: true},
	TurnStateRunning:   {TurnStateCompleted: true, TurnStateFailed: true, TurnStateCanceled: true},
	TurnStateCompleted: {},
	TurnStateFailed:    {},
	TurnStateCanceled:  {},
}

// TransitionTurnState validates a proposed TurnState transition and returns
// the next state. On any invalid-value or invalid-transition error it returns
// from unchanged; from and to are never mutated.
func TransitionTurnState(from, to TurnState) (TurnState, error) {
	if err := from.Validate(); err != nil {
		return from, err
	}
	if err := to.Validate(); err != nil {
		return from, err
	}
	if legalTurnStateTransitions[from][to] {
		return to, nil
	}
	return from, &InvalidTurnStateTransitionError{From: from, To: to}
}

// Turn is the public, transport-neutral identity and lifecycle fact set for
// one Chat Session turn. It defines no execution or persistence behavior.
type Turn struct {
	ID               string
	Episode          uint64
	State            TurnState
	RequestID        RequestIdentity
	StartSequence    uint64
	TerminalSequence uint64
}

// CheckTurnAdmission reports whether a new turn may be admitted into the
// session identified by sessionID, given the session's prior active turn if
// any. A nil priorActiveTurn, or one whose State is terminal, does not
// produce the busy outcome. A priorActiveTurn in TurnStateAdmitted or
// TurnStateRunning returns a typed TurnBusyError carrying only the session ID
// and the active turn's ID and state.
func CheckTurnAdmission(sessionID string, priorActiveTurn *Turn) error {
	if priorActiveTurn == nil {
		return nil
	}
	if priorActiveTurn.State.terminal() {
		return nil
	}
	return &TurnBusyError{
		SessionID:       sessionID,
		ActiveTurnID:    priorActiveTurn.ID,
		ActiveTurnState: priorActiveTurn.State,
	}
}

// CheckVersion validates that expected matches actual, returning a typed
// VersionConflictError on mismatch. It performs no mutation and is reusable
// across every version-checked Chat Sessions mutation (session, episode, and
// turn state changes all gate on the owning Session's Version).
func CheckVersion(expected, actual uint64) error {
	if expected != actual {
		return &VersionConflictError{Expected: expected, Actual: actual}
	}
	return nil
}

// TransitionTurnStateChecked validates expectedVersion against actualVersion
// before applying the requested TurnState transition. On a version mismatch
// it returns turn unchanged along with a typed VersionConflictError. On a
// matching version it applies TransitionTurnState and, on success, returns a
// copy of turn with State updated to the next value; on transition failure it
// returns turn unchanged along with the typed transition error. turn is never
// mutated.
func TransitionTurnStateChecked(turn Turn, to TurnState, expectedVersion, actualVersion uint64) (Turn, error) {
	if err := CheckVersion(expectedVersion, actualVersion); err != nil {
		return turn, err
	}
	next, err := TransitionTurnState(turn.State, to)
	if err != nil {
		return turn, err
	}
	updated := turn
	updated.State = next
	return updated, nil
}

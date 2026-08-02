package chatsessions

import (
	"strings"
	"time"
)

// ControlAction identifies the operation a Control Intent requests. CANCEL
// and CLOSE are supported in L1; PAUSE, RESUME, and TERMINATE are declared
// for L4 and are valid values but are rejected as unsupported by
// CheckSupported.
type ControlAction string

const (
	// ControlActionCancel requests cancellation of the captured turn.
	ControlActionCancel ControlAction = "CANCEL"
	// ControlActionClose requests the Chat Session close.
	ControlActionClose ControlAction = "CLOSE"
	// ControlActionPause is declared for L4. Not supported in L1.
	ControlActionPause ControlAction = "PAUSE"
	// ControlActionResume is declared for L4. Not supported in L1.
	ControlActionResume ControlAction = "RESUME"
	// ControlActionTerminate is declared for L4. Not supported in L1.
	ControlActionTerminate ControlAction = "TERMINATE"
)

// Validate reports whether a is one of the exactly declared ControlAction
// values. The zero value and any unknown value are rejected. Validate does
// not check L1 support; use CheckSupported for that.
func (a ControlAction) Validate() error {
	switch a {
	case ControlActionCancel, ControlActionClose, ControlActionPause, ControlActionResume, ControlActionTerminate:
		return nil
	default:
		return &InvalidControlActionError{Action: a}
	}
}

var l1SupportedControlActions = map[ControlAction]bool{
	ControlActionCancel: true,
	ControlActionClose:  true,
}

// CheckSupported validates a and additionally rejects declared-but-L4-only
// actions with a typed UnsupportedControlActionError. An unknown or zero
// value returns the InvalidControlActionError from Validate, not the
// unsupported-action error.
func (a ControlAction) CheckSupported() error {
	if err := a.Validate(); err != nil {
		return err
	}
	if !l1SupportedControlActions[a] {
		return &UnsupportedControlActionError{Action: a}
	}
	return nil
}

// ControlIntentState is the lifecycle state of one Control Intent.
type ControlIntentState string

const (
	// ControlIntentStateRequested is the initial state before the target is
	// captured and persisted.
	ControlIntentStateRequested ControlIntentState = "REQUESTED"
	// ControlIntentStateCommitted means the target turn/episode/version was
	// captured and persisted before fan-out.
	ControlIntentStateCommitted ControlIntentState = "COMMITTED"
	// ControlIntentStateCompleted is terminal: fan-out reached the captured
	// turn while it was still current and non-terminal.
	ControlIntentStateCompleted ControlIntentState = "COMPLETED"
	// ControlIntentStateNoop is terminal: the captured turn was already
	// terminal on arrival.
	ControlIntentStateNoop ControlIntentState = "NOOP"
	// ControlIntentStateSuperseded is terminal: the captured turn was no
	// longer current on arrival.
	ControlIntentStateSuperseded ControlIntentState = "SUPERSEDED"
)

// Validate reports whether s is one of the exactly declared
// ControlIntentState values. The zero value and any unknown value are
// rejected.
func (s ControlIntentState) Validate() error {
	switch s {
	case ControlIntentStateRequested, ControlIntentStateCommitted, ControlIntentStateCompleted,
		ControlIntentStateNoop, ControlIntentStateSuperseded:
		return nil
	default:
		return &InvalidControlIntentStateError{State: s}
	}
}

var legalControlIntentStateTransitions = map[ControlIntentState]map[ControlIntentState]bool{
	ControlIntentStateRequested: {ControlIntentStateCommitted: true},
	ControlIntentStateCommitted: {
		ControlIntentStateCompleted:  true,
		ControlIntentStateNoop:       true,
		ControlIntentStateSuperseded: true,
	},
	ControlIntentStateCompleted:  {},
	ControlIntentStateNoop:       {},
	ControlIntentStateSuperseded: {},
}

// TransitionControlIntentState validates a proposed ControlIntentState
// transition and returns the next state. On any invalid-value or
// invalid-transition error it returns from unchanged; from and to are never
// mutated.
func TransitionControlIntentState(from, to ControlIntentState) (ControlIntentState, error) {
	if err := from.Validate(); err != nil {
		return from, err
	}
	if err := to.Validate(); err != nil {
		return from, err
	}
	if legalControlIntentStateTransitions[from][to] {
		return to, nil
	}
	return from, &InvalidControlIntentStateTransitionError{From: from, To: to}
}

// ControlIntent is the public, transport-neutral capture of a race-safe
// control request. It binds a request to the turn, target episode, and
// version observed at capture time, so a completion, no-op, or superseded
// outcome can never be rebound to a later admitted turn.
type ControlIntent struct {
	RequestID       RequestIdentity
	SessionID       string
	TurnID          string
	TargetEpisode   uint64
	ExpectedVersion uint64
	Action          ControlAction
	State           ControlIntentState
	RequestedAt     time.Time
}

// Validate reports whether intent carries every required capture field and a
// supported action. RequestID and Action errors are returned as-is from
// their own Validate/CheckSupported methods; a missing SessionID or TurnID
// returns a typed InvalidControlIntentError.
func (intent ControlIntent) Validate() error {
	if err := intent.RequestID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(intent.SessionID) == "" {
		return &InvalidControlIntentError{Reason: ControlIntentInvalidMissingSessionID}
	}
	if strings.TrimSpace(intent.TurnID) == "" {
		return &InvalidControlIntentError{Reason: ControlIntentInvalidMissingTurnID}
	}
	if err := intent.Action.CheckSupported(); err != nil {
		return err
	}
	return nil
}

// CommitControlIntent validates intent's required capture fields and
// transitions it from REQUESTED to COMMITTED. On any validation or
// transition error it returns intent unchanged; intent is never mutated.
func CommitControlIntent(intent ControlIntent) (ControlIntent, error) {
	if err := intent.Validate(); err != nil {
		return intent, err
	}
	next, err := TransitionControlIntentState(intent.State, ControlIntentStateCommitted)
	if err != nil {
		return intent, err
	}
	committed := intent
	committed.State = next
	return committed, nil
}

// ResolveControlIntentOutcome determines the terminal ControlIntentState for
// a committed intent's captured turn. If currentActiveID no longer equals
// capturedTurnID, the captured turn is no longer current and the outcome is
// always SUPERSEDED, regardless of capturedTurnState. Otherwise, an
// already-terminal capturedTurnState resolves to NOOP and a non-terminal
// capturedTurnState resolves to COMPLETED.
func ResolveControlIntentOutcome(capturedTurnID string, capturedTurnState TurnState, currentActiveID string) (ControlIntentState, error) {
	if err := capturedTurnState.Validate(); err != nil {
		return "", err
	}
	if capturedTurnID != currentActiveID {
		return ControlIntentStateSuperseded, nil
	}
	if capturedTurnState.terminal() {
		return ControlIntentStateNoop, nil
	}
	return ControlIntentStateCompleted, nil
}

// CompleteControlIntent resolves the outcome for intent's captured turn
// (intent.TurnID) against capturedTurnState and currentActiveID, then
// transitions intent from COMMITTED to that outcome. On any resolution or
// transition error it returns intent unchanged; intent is never mutated.
func CompleteControlIntent(intent ControlIntent, capturedTurnState TurnState, currentActiveID string) (ControlIntent, error) {
	outcome, err := ResolveControlIntentOutcome(intent.TurnID, capturedTurnState, currentActiveID)
	if err != nil {
		return intent, err
	}
	next, err := TransitionControlIntentState(intent.State, outcome)
	if err != nil {
		return intent, err
	}
	completed := intent
	completed.State = next
	return completed, nil
}

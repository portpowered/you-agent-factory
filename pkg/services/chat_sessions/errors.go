package chatsessions

import (
	"errors"
	"fmt"
)

// Sentinel value-validation categories. Callers use errors.Is against these
// sentinels (typically through a returned *ValidationError) instead of
// parsing error text.
var (
	// ErrRequiredValue reports a blank or zero value where the L1 V0 contract
	// requires a caller-supplied identity, reference, or timestamp.
	ErrRequiredValue = errors.New("chat sessions: required value is blank or zero")
	// ErrUnknownEnumValue reports an enum value outside its declared members.
	ErrUnknownEnumValue = errors.New("chat sessions: unknown enum value")
	// ErrInconsistentValue reports a value whose fields disagree with each
	// other under the L1 V0 model, such as a terminal sequence or terminal
	// time fact that does not match the value's declared state.
	ErrInconsistentValue = errors.New("chat sessions: structurally inconsistent value")
	// ErrUnsupportedControlAction reports a ControlAction that is declared in
	// the L1 vocabulary for a later lane (PAUSE, RESUME, TERMINATE) but is not
	// an executable action in this L1 V0 slice. It is distinct from
	// ErrUnknownEnumValue, which reports a value outside the declared
	// vocabulary entirely.
	ErrUnsupportedControlAction = errors.New("chat sessions: control action is not supported in L1")
	// ErrInvalidTransition reports a state pair that is not a legal L1 V0
	// lifecycle transition, including any transition attempted from a
	// terminal state. It is distinct from the story-001 value-validation
	// sentinels: both the from and to states are already declared enum
	// members, but the L1 V0 transition table does not permit moving between
	// them.
	ErrInvalidTransition = errors.New("chat sessions: illegal state transition")
	// ErrNotFound reports that a Service operation referenced a session or a
	// subordinate entity (turn, attachment, control intent) that does not
	// exist.
	ErrNotFound = errors.New("chat sessions: not found")
	// ErrBusy reports that a Service operation was rejected because the
	// session already has a non-terminal active turn.
	ErrBusy = errors.New("chat sessions: active turn busy")
	// ErrStaleVersion reports that a Service operation's expected session
	// version no longer matches the session's current version.
	ErrStaleVersion = errors.New("chat sessions: stale expected version")
)

// ValidationError reports one Chat Sessions value-validation failure. Value
// names the owning type, Field names the offending field (empty when the
// failure applies to the whole value, such as an enum receiver), and Err is
// one of the package sentinel errors so callers can use errors.Is/errors.As
// without parsing Error() text.
type ValidationError struct {
	Value string
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("chat sessions: %s: %v", e.Value, e.Err)
	}
	return fmt.Sprintf("chat sessions: %s.%s: %v", e.Value, e.Field, e.Err)
}

// Unwrap exposes the underlying sentinel (or nested *ValidationError) so
// errors.Is/errors.As can classify the failure across a wrapped chain.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

func newValidationError(value, field string, err error) *ValidationError {
	return &ValidationError{Value: value, Field: field, Err: err}
}

// TransitionError reports one illegal Chat Sessions lifecycle transition.
// Value names the owning state machine and From/To record the attempted
// state pair. Err is ErrInvalidTransition unless From or To is itself not a
// declared enum member, in which case Err is the underlying
// ErrUnknownEnumValue *ValidationError so callers can distinguish an
// invalid-state input from a legal-state-but-illegal-transition outcome.
type TransitionError struct {
	Value string
	From  string
	To    string
	Err   error
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("chat sessions: %s: %s -> %s: %v", e.Value, e.From, e.To, e.Err)
}

// Unwrap exposes the underlying sentinel so errors.Is/errors.As can classify
// the failure.
func (e *TransitionError) Unwrap() error {
	return e.Err
}

func newTransitionError(value, from, to string) *TransitionError {
	return &TransitionError{Value: value, From: from, To: to, Err: ErrInvalidTransition}
}

// NotFoundError reports that a Service operation referenced a session or
// subordinate entity that does not exist. Value names the entity kind (for
// example "Session" or "Turn") and ID names the identity that was not found.
type NotFoundError struct {
	Value string
	ID    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("chat sessions: %s %q: %v", e.Value, e.ID, ErrNotFound)
}

// Unwrap exposes ErrNotFound so errors.Is/errors.As can classify the
// failure.
func (e *NotFoundError) Unwrap() error {
	return ErrNotFound
}

// BusyError reports that a Service operation was rejected because the named
// session already has a non-terminal active turn.
type BusyError struct {
	Value string
	ID    string
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("chat sessions: %s %q: %v", e.Value, e.ID, ErrBusy)
}

// Unwrap exposes ErrBusy so errors.Is/errors.As can classify the failure.
func (e *BusyError) Unwrap() error {
	return ErrBusy
}

// ConflictError reports that a Service operation's Expected session version
// no longer matches Actual, the session's current version.
type ConflictError struct {
	Value    string
	ID       string
	Expected uint64
	Actual   uint64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("chat sessions: %s %q: expected version %d, current version %d: %v",
		e.Value, e.ID, e.Expected, e.Actual, ErrStaleVersion)
}

// Unwrap exposes ErrStaleVersion so errors.Is/errors.As can classify the
// failure.
func (e *ConflictError) Unwrap() error {
	return ErrStaleVersion
}

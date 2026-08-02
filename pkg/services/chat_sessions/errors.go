package chatsessions

import "fmt"

// RequestIdentityInvalidReason classifies why a RequestIdentity failed
// validation without exposing the supplied identity values.
type RequestIdentityInvalidReason string

const (
	// RequestIdentityInvalidEmpty reports that no identity mode was supplied.
	RequestIdentityInvalidEmpty RequestIdentityInvalidReason = "EMPTY"
	// RequestIdentityInvalidBareJSONRPCID reports a JSON-RPC id supplied
	// without its pairing connection id.
	RequestIdentityInvalidBareJSONRPCID RequestIdentityInvalidReason = "BARE_JSONRPC_ID"
	// RequestIdentityInvalidIncompleteConnectionPair reports a connection id
	// supplied without its pairing JSON-RPC id.
	RequestIdentityInvalidIncompleteConnectionPair RequestIdentityInvalidReason = "INCOMPLETE_CONNECTION_PAIR"
	// RequestIdentityInvalidMixedIdentityModes reports both a
	// connection-qualified field and a transport-minted opaque id supplied
	// together.
	RequestIdentityInvalidMixedIdentityModes RequestIdentityInvalidReason = "MIXED_IDENTITY_MODES"
)

// InvalidRequestIdentityError reports a RequestIdentity that matches neither
// legal identity mode. Peers branch on Reason; the error never carries the
// supplied identity values, so it is safe to log.
type InvalidRequestIdentityError struct {
	Reason RequestIdentityInvalidReason
}

func (e *InvalidRequestIdentityError) Error() string {
	return fmt.Sprintf("invalid chat session request identity: %s", e.Reason)
}

// InvalidChatTargetKindError reports a ChatTargetKind outside the exact
// declared value set (FACTORY, WORKER).
type InvalidChatTargetKindError struct {
	Kind ChatTargetKind
}

func (e *InvalidChatTargetKindError) Error() string {
	return fmt.Sprintf("invalid chat target kind: %q", string(e.Kind))
}

// InvalidChatTargetRefError reports a ChatTargetRef missing its canonical
// reference.
type InvalidChatTargetRefError struct {
	Kind ChatTargetKind
}

func (e *InvalidChatTargetRefError) Error() string {
	return fmt.Sprintf("invalid chat target ref: empty canonical ref for kind %q", string(e.Kind))
}

// InvalidSessionStateError reports a SessionState outside the exact declared
// value set (CREATED, ACTIVE, CLOSED).
type InvalidSessionStateError struct {
	State SessionState
}

func (e *InvalidSessionStateError) Error() string {
	return fmt.Sprintf("invalid session state: %q", string(e.State))
}

// InvalidSessionStateTransitionError reports a SessionState transition
// outside the declared legal transition table.
type InvalidSessionStateTransitionError struct {
	From SessionState
	To   SessionState
}

func (e *InvalidSessionStateTransitionError) Error() string {
	return fmt.Sprintf("invalid session state transition: %q to %q", string(e.From), string(e.To))
}

// InvalidTargetEpisodeStateError reports a TargetEpisodeState outside the
// exact declared value set (OPEN, CLOSED).
type InvalidTargetEpisodeStateError struct {
	State TargetEpisodeState
}

func (e *InvalidTargetEpisodeStateError) Error() string {
	return fmt.Sprintf("invalid target episode state: %q", string(e.State))
}

// InvalidTargetEpisodeStateTransitionError reports a TargetEpisodeState
// transition outside the declared legal transition table.
type InvalidTargetEpisodeStateTransitionError struct {
	From TargetEpisodeState
	To   TargetEpisodeState
}

func (e *InvalidTargetEpisodeStateTransitionError) Error() string {
	return fmt.Sprintf("invalid target episode state transition: %q to %q", string(e.From), string(e.To))
}

// TargetEpisodeNotClosedError reports an attempt to open the next Target
// Episode while the prior episode is not yet closed.
type TargetEpisodeNotClosedError struct {
	Number uint64
	State  TargetEpisodeState
}

func (e *TargetEpisodeNotClosedError) Error() string {
	return fmt.Sprintf("target episode %d is not closed: state %q", e.Number, string(e.State))
}

// InvalidTurnStateError reports a TurnState outside the exact declared value
// set (ADMITTED, RUNNING, COMPLETED, FAILED, CANCELED).
type InvalidTurnStateError struct {
	State TurnState
}

func (e *InvalidTurnStateError) Error() string {
	return fmt.Sprintf("invalid turn state: %q", string(e.State))
}

// InvalidTurnStateTransitionError reports a TurnState transition outside the
// declared legal transition table.
type InvalidTurnStateTransitionError struct {
	From TurnState
	To   TurnState
}

func (e *InvalidTurnStateTransitionError) Error() string {
	return fmt.Sprintf("invalid turn state transition: %q to %q", string(e.From), string(e.To))
}

// TurnBusyError reports that turn admission was rejected because the session
// already has a non-terminal active turn. It carries only safe identity
// facts: the session ID and the active turn's ID and state.
type TurnBusyError struct {
	SessionID       string
	ActiveTurnID    string
	ActiveTurnState TurnState
}

func (e *TurnBusyError) Error() string {
	return fmt.Sprintf("chat session %q busy: active turn %q in state %q", e.SessionID, e.ActiveTurnID, string(e.ActiveTurnState))
}

// VersionConflictError reports that a version-checked mutation's expected
// version did not match the actual version. It carries only the two version
// numbers; the mutation it guarded is left unchanged.
type VersionConflictError struct {
	Expected uint64
	Actual   uint64
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("version conflict: expected %d, actual %d", e.Expected, e.Actual)
}

// AttachmentInvalidReason classifies why an Attachment failed validation
// without exposing the supplied identity values.
type AttachmentInvalidReason string

const (
	// AttachmentInvalidMissingID reports a missing attachment identity.
	AttachmentInvalidMissingID AttachmentInvalidReason = "MISSING_ID"
	// AttachmentInvalidMissingSessionID reports a missing owning session
	// identity.
	AttachmentInvalidMissingSessionID AttachmentInvalidReason = "MISSING_SESSION_ID"
	// AttachmentInvalidMissingConnectionID reports a missing connection
	// identity.
	AttachmentInvalidMissingConnectionID AttachmentInvalidReason = "MISSING_CONNECTION_ID"
)

// InvalidAttachmentError reports an Attachment missing a required identity.
// Peers branch on Reason; the error never carries the supplied identity
// values, so it is safe to log.
type InvalidAttachmentError struct {
	Reason AttachmentInvalidReason
}

func (e *InvalidAttachmentError) Error() string {
	return fmt.Sprintf("invalid chat session attachment: %s", e.Reason)
}

// InvalidControlActionError reports a ControlAction outside the exact
// declared value set (CANCEL, CLOSE, PAUSE, RESUME, TERMINATE).
type InvalidControlActionError struct {
	Action ControlAction
}

func (e *InvalidControlActionError) Error() string {
	return fmt.Sprintf("invalid control action: %q", string(e.Action))
}

// UnsupportedControlActionError reports a ControlAction that is a declared
// value but is not supported in L1 (PAUSE, RESUME, TERMINATE).
type UnsupportedControlActionError struct {
	Action ControlAction
}

func (e *UnsupportedControlActionError) Error() string {
	return fmt.Sprintf("unsupported control action in L1: %q", string(e.Action))
}

// InvalidControlIntentStateError reports a ControlIntentState outside the
// exact declared value set (REQUESTED, COMMITTED, COMPLETED, NOOP,
// SUPERSEDED).
type InvalidControlIntentStateError struct {
	State ControlIntentState
}

func (e *InvalidControlIntentStateError) Error() string {
	return fmt.Sprintf("invalid control intent state: %q", string(e.State))
}

// InvalidControlIntentStateTransitionError reports a ControlIntentState
// transition outside the declared legal transition table.
type InvalidControlIntentStateTransitionError struct {
	From ControlIntentState
	To   ControlIntentState
}

func (e *InvalidControlIntentStateTransitionError) Error() string {
	return fmt.Sprintf("invalid control intent state transition: %q to %q", string(e.From), string(e.To))
}

// ControlIntentInvalidReason classifies why a ControlIntent failed
// validation without exposing the supplied identity values.
type ControlIntentInvalidReason string

const (
	// ControlIntentInvalidMissingSessionID reports a missing owning session
	// identity.
	ControlIntentInvalidMissingSessionID ControlIntentInvalidReason = "MISSING_SESSION_ID"
	// ControlIntentInvalidMissingTurnID reports a missing captured turn
	// identity.
	ControlIntentInvalidMissingTurnID ControlIntentInvalidReason = "MISSING_TURN_ID"
)

// InvalidControlIntentError reports a ControlIntent missing a required
// capture field. Peers branch on Reason; the error never carries the
// supplied identity values, so it is safe to log.
type InvalidControlIntentError struct {
	Reason ControlIntentInvalidReason
}

func (e *InvalidControlIntentError) Error() string {
	return fmt.Sprintf("invalid chat session control intent: %s", e.Reason)
}

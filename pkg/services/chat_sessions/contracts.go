package chatsessions

import "context"

// Service is the singular L1 V0 Chat Sessions root contract: session
// create/read, target selection, turn admission and advancement, connection
// attachment, and control-intent request/advancement. This package publishes
// only the interface and its detached request/result values; it has no
// implementation, dependency bag, persistence port, or alternate
// peer-facing service contract.
//
// Every method accepts context.Context plus Chat Sessions-owned detached
// request values and returns Chat Sessions-owned detached result values and
// error. Target-changing and control-requesting methods (SetTarget,
// StartTurn, RequestControl) carry an ExpectedVersion against Session.Version
// so a stale caller observes a typed *ConflictError instead of silently
// clobbering a concurrent mutation. RequestControl captures the concrete
// active turn ID and target episode as the control target internally; a
// caller never supplies or observes StreamHead or an Attachment's
// AfterSequence as a control target.
//
// Implementations return the following typed failures, all classifiable with
// errors.Is/errors.As instead of parsing Error() text:
//
//   - *ValidationError (ErrRequiredValue, ErrUnknownEnumValue,
//     ErrInconsistentValue, ErrUnsupportedControlAction) for invalid input.
//   - *NotFoundError (ErrNotFound) when a session or subordinate entity
//     (turn, attachment, control intent) does not exist.
//   - *BusyError (ErrBusy) when a turn-admitting operation is rejected
//     because the session already has a non-terminal active turn.
//   - *ConflictError (ErrStaleVersion) when ExpectedVersion no longer
//     matches the session's current version.
//   - *TransitionError (ErrInvalidTransition) when an advancement requests a
//     state pair the L1 V0 transition table does not permit.
//
// Error text and fields are safe for crossing a service boundary: they carry
// only entity kind, identity, and version facts, and never a prompt,
// credential, raw provider command, filesystem path, dependency object, or
// private topology.
type Service interface {
	// CreateSession creates a new Chat Session in SessionStateCreated with
	// the given initial selected target.
	CreateSession(ctx context.Context, req CreateSessionRequest) (CreateSessionResult, error)

	// GetSession returns the current state of one Chat Session. It reports
	// *NotFoundError when SessionID does not identify an existing session.
	GetSession(ctx context.Context, req GetSessionRequest) (GetSessionResult, error)

	// SetTarget changes a Chat Session's selected target, opening a new
	// TargetEpisode for the next admitted turn. It reports *BusyError while
	// a turn is active and *ConflictError when ExpectedVersion is stale.
	SetTarget(ctx context.Context, req SetTargetRequest) (SetTargetResult, error)

	// StartTurn admits a new Turn in TurnStateAdmitted against the session's
	// current target episode. It reports *BusyError when a non-terminal turn
	// is already active and *ConflictError when ExpectedVersion is stale.
	StartTurn(ctx context.Context, req StartTurnRequest) (StartTurnResult, error)

	// AdvanceTurn moves an admitted or running Turn to Next, enforcing the
	// TurnState transition table. It reports *NotFoundError when TurnID does
	// not identify an existing turn and *TransitionError when Next is not a
	// legal transition from the turn's current state.
	AdvanceTurn(ctx context.Context, req AdvanceTurnRequest) (AdvanceTurnResult, error)

	// Attach registers one connection's delivery position against a Chat
	// Session's event stream. It reports *NotFoundError when SessionID does
	// not identify an existing session.
	Attach(ctx context.Context, req AttachRequest) (AttachResult, error)

	// Detach removes one previously attached connection. It reports
	// *NotFoundError when AttachmentID does not identify an existing
	// attachment on the session.
	Detach(ctx context.Context, req DetachRequest) (DetachResult, error)

	// RequestControl atomically captures the session's current active turn,
	// target episode, and version as a new ControlIntent in
	// ControlIntentStateRequested. It reports *NotFoundError when there is
	// no active turn to target, *ConflictError when ExpectedVersion is
	// stale, and a *ValidationError wrapping ErrUnsupportedControlAction
	// when Action is declared vocabulary not executable in L1.
	RequestControl(ctx context.Context, req RequestControlRequest) (RequestControlResult, error)

	// AdvanceControl moves a control intent to Next, enforcing the
	// ControlIntentState transition table. It reports *NotFoundError when
	// the intent identified by SessionID and RequestID does not exist and
	// *TransitionError when Next is not a legal transition from the
	// intent's current state.
	AdvanceControl(ctx context.Context, req AdvanceControlRequest) (AdvanceControlResult, error)
}

// CreateSessionRequest carries the caller identity and initial target for a
// new Chat Session.
type CreateSessionRequest struct {
	RequestID     RequestIdentity
	InitialTarget ChatTargetRef
}

// CreateSessionResult carries the newly created Session.
type CreateSessionResult struct {
	Session Session
}

// GetSessionRequest identifies the Chat Session to read.
type GetSessionRequest struct {
	SessionID string
}

// GetSessionResult carries the current Session state.
type GetSessionResult struct {
	Session Session
}

// SetTargetRequest carries the caller identity, target session, expected
// version, and new target for a target-change request.
type SetTargetRequest struct {
	RequestID       RequestIdentity
	SessionID       string
	ExpectedVersion uint64
	Target          ChatTargetRef
}

// SetTargetResult carries the Session after the target change.
type SetTargetResult struct {
	Session Session
}

// StartTurnRequest carries the caller identity, target session, and expected
// version for a turn-admission request.
type StartTurnRequest struct {
	RequestID       RequestIdentity
	SessionID       string
	ExpectedVersion uint64
}

// StartTurnResult carries the Session and newly admitted Turn.
type StartTurnResult struct {
	Session Session
	Turn    Turn
}

// AdvanceTurnRequest identifies a Turn and the state it should move to.
type AdvanceTurnRequest struct {
	SessionID string
	TurnID    string
	Next      TurnState
}

// AdvanceTurnResult carries the Turn after the requested advancement.
type AdvanceTurnResult struct {
	Turn Turn
}

// AttachRequest carries the target session, connecting connection identity,
// and whether the attachment is the interactive leader.
type AttachRequest struct {
	SessionID    string
	ConnectionID string
	Interactive  bool
}

// AttachResult carries the newly registered Attachment.
type AttachResult struct {
	Attachment Attachment
}

// DetachRequest identifies the attachment to remove.
type DetachRequest struct {
	SessionID    string
	AttachmentID string
}

// DetachResult is intentionally empty: a successful Detach has no observable
// return value beyond a nil error.
type DetachResult struct{}

// RequestControlRequest carries the caller identity, target session,
// expected version, and requested action for a control request. The concrete
// turn and target episode are captured by the Service, not supplied by the
// caller.
type RequestControlRequest struct {
	RequestID       RequestIdentity
	SessionID       string
	ExpectedVersion uint64
	Action          ControlAction
}

// RequestControlResult carries the newly requested ControlIntent.
type RequestControlResult struct {
	Intent ControlIntent
}

// AdvanceControlRequest identifies a previously requested ControlIntent by
// the session it targets and the RequestIdentity that created it, plus the
// state it should move to.
type AdvanceControlRequest struct {
	SessionID string
	RequestID RequestIdentity
	Next      ControlIntentState
}

// AdvanceControlResult carries the ControlIntent after the requested
// advancement.
type AdvanceControlResult struct {
	Intent ControlIntent
}

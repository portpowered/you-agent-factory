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
//     because the session already has a non-terminal active turn. It carries
//     the blocking turn's ActiveTurnID and ActiveTurnState so a caller does
//     not need a follow-up read.
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
	// the given validated working root (WorkingRoot) and initial selected target. A
	// blank WorkingRoot, an invalid RequestID, or an invalid InitialTarget reports a
	// *ValidationError and creates no observable session.
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
	// when Action is declared vocabulary not executable in L1. Reusing a
	// RequestID that already identifies a requested intent is idempotent: it
	// returns that existing, immutable intent unchanged instead of
	// recapturing the (possibly different) current active turn, target
	// episode, or version.
	RequestControl(ctx context.Context, req RequestControlRequest) (RequestControlResult, error)

	// AdvanceControl moves a control intent to Next, enforcing the
	// ControlIntentState transition table. It reports *NotFoundError when
	// the intent identified by SessionID and RequestID does not exist and
	// *TransitionError when Next is not a legal transition from the
	// intent's current state. When Next resolves a COMMITTED intent to a
	// terminal outcome, an implementation determines that outcome with
	// ResolveControlIntentOutcome against the intent's captured turn, not a
	// caller-selected value, so completion can only ever complete, no-op, or
	// supersede the captured turn -- never a later one.
	AdvanceControl(ctx context.Context, req AdvanceControlRequest) (AdvanceControlResult, error)

	// BindFactorySession commits a returned Factory Session identity onto
	// SessionID's TargetEpisode numbered Episode, but only when SessionID
	// exists, TurnID is still that session's active turn admitted against
	// exactly that episode, and ExpectedVersion still matches the session's
	// current version -- the same session/episode/turn/version snapshot a
	// caller observed when it started the Factory Session it is now
	// binding. A repeat call carrying the exact FactorySessionID the
	// episode already carries is idempotent and succeeds without mutation
	// regardless of ExpectedVersion, so a retried or concurrently-raced
	// binding attempt for the identity that already won converges on that
	// one committed value instead of failing. It reports *NotFoundError for
	// an unknown SessionID, *ValidationError for a blank FactorySessionID or
	// TurnID, *ConflictError when ExpectedVersion no longer matches or the
	// session's active turn/episode has moved on, and
	// *FactorySessionConflictError when the episode already carries a
	// different Factory Session identity -- in every failure case the
	// stored session and episode history are left byte-for-byte unchanged,
	// and no prior (non-current) episode is ever mutated.
	BindFactorySession(ctx context.Context, req BindFactorySessionRequest) (BindFactorySessionResult, error)
}

// CreateSessionRequest carries the caller identity, initial target, and
// working root for a new Chat Session. WorkingRoot is the ACP client's
// validated editor cwd; it is fixed for the Session's lifetime and is the
// root a later target change (SetTarget) revalidates a requested target
// against.
type CreateSessionRequest struct {
	RequestID RequestIdentity
	// WorkingRoot is the caller-supplied ACP editor working root. It must be
	// non-blank; a blank WorkingRoot is rejected with the same typed validation
	// classification as any other required-value failure and creates no
	// session.
	WorkingRoot   string
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

// StartTurnResult carries the Session, newly admitted Turn, and the
// immutable snapshot of the TargetEpisode the turn was admitted into --
// including that episode's Number and any FactorySessionID it already
// carries -- so a caller can decide whether to start or reuse a Factory
// Session without a second read.
type StartTurnResult struct {
	Session Session
	Turn    Turn
	Episode TargetEpisode
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

// BindFactorySessionRequest identifies the exact session/episode/turn/
// version snapshot a caller observed when it started a Factory Session, plus
// the Factory Session identity to commit onto that episode.
type BindFactorySessionRequest struct {
	SessionID string
	// ExpectedVersion is the session's version observed at admission --
	// StartTurnResult.Session.Version -- not a value re-read afterward.
	ExpectedVersion uint64
	// Episode is the TargetEpisode number the admitted turn belongs to --
	// StartTurnResult.Episode.Number.
	Episode uint64
	// TurnID is the admitted turn that started the Factory Session being
	// bound -- StartTurnResult.Turn.ID.
	TurnID string
	// FactorySessionID is the identity returned by the Factory Sessions
	// start call. It must be non-blank.
	FactorySessionID string
}

// BindFactorySessionResult carries the Session after a successful (or
// idempotently converged) binding.
type BindFactorySessionResult struct {
	Session Session
}

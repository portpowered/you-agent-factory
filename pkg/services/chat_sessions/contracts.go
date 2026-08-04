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
	// and no prior (non-current) episode is ever mutated. A successful
	// commit also clears any PendingFactorySessionID recorded via
	// RecordPendingFactorySession, since the pending record's purpose --
	// letting a retry find the started-but-uncommitted identity -- is moot
	// once that identity is durably committed.
	BindFactorySession(ctx context.Context, req BindFactorySessionRequest) (BindFactorySessionResult, error)

	// RecordPendingFactorySession durably records (or, with a blank
	// FactorySessionID, clears) a Factory Session identity a caller started
	// for SessionID's TargetEpisode numbered Episode but has not yet
	// committed via BindFactorySession, under the same
	// session/episode/turn/version guard BindFactorySession itself uses. It
	// exists so "a Factory Session was already started for this
	// still-unbound episode" is durable Chat/Factory Sessions authority
	// state a later retry can observe and reuse (via the episode snapshot's
	// PendingFactorySessionID), not state a caller's own transport instance
	// must remember to survive a post-start failure. Unlike BindFactorySession
	// this never advances Session.Version: it is incidental
	// reconciliation bookkeeping, not a state transition other guarded
	// callers need to observe as invalidating their own ExpectedVersion. It
	// reports *NotFoundError for an unknown SessionID and *ConflictError
	// when ExpectedVersion no longer matches or the session's active
	// turn/episode has moved on.
	RecordPendingFactorySession(ctx context.Context, req RecordPendingFactorySessionRequest) (RecordPendingFactorySessionResult, error)

	// Sequence is SessionID's one session-owned serialization point for
	// committing source-native records onto EventsTopic(SessionID). It
	// assigns a stable, non-empty ItemID before appending the complete
	// envelope (SequencedItem) to Events, in successful commit order --
	// independent of source timestamps, source sequence across producers, or
	// goroutine start order. A non-blank ParentItemID is accepted only when
	// it already identifies an item this exact session has sequenced;
	// otherwise Sequence reports *NotFoundError and commits nothing. Reusing
	// the same (SourceType, SourceID, SourceSequence, SourceEventID) tuple is
	// idempotent: it returns the originally assigned ItemID, ParentItemID,
	// and aggregate position with SequenceOutcomeDuplicate instead of a
	// second committed record. It reports *NotFoundError when SessionID does
	// not identify an existing session.
	Sequence(ctx context.Context, req SequenceRequest) (SequenceResult, error)

	// AdvanceStreamHead advances SessionID's StreamHead to AggregateSequence
	// -- the position a prior accepted Sequence call committed -- under an
	// optimistic-version guard. When StreamHead already stands at or beyond
	// AggregateSequence, the call is an idempotent no-op: it reports
	// AdvanceStreamHeadOutcomeAlreadyCurrent and leaves the session,
	// including its Version, byte-for-byte unchanged, regardless of
	// ExpectedVersion. Otherwise it reports *ConflictError when
	// ExpectedVersion no longer matches the session's current version and
	// exposes no partially committed head update: StreamHead, Version, and
	// UpdatedAt advance together or not at all. A successful advancement
	// never changes SelectedTarget, TargetEpisode, ActiveTurnID, or any
	// attachment, control, or episode state. It reports *NotFoundError when
	// SessionID does not identify an existing session.
	AdvanceStreamHead(ctx context.Context, req AdvanceStreamHeadRequest) (AdvanceStreamHeadResult, error)

	// AcknowledgeAttachment advances one Attachment's own AfterSequence
	// delivery cursor to AfterSequence under an optimistic session-version
	// guard. When AfterSequence already stands at or beyond the requested
	// position, the call is an idempotent no-op: it reports
	// AcknowledgeAttachmentOutcomeAlreadyCurrent and leaves the attachment
	// unchanged, so a retried or stale (including backward-moving)
	// acknowledgement can never regress the cursor. It reports
	// *AttachmentPositionError when the requested position exceeds the
	// session's current StreamHead, *ConflictError when ExpectedVersion no
	// longer matches the session's current version, and
	// *AttachmentRetentionGapError when Events retention has evicted part of
	// the range between the attachment's current position and the requested
	// one -- in every failure case the attachment and session are left
	// byte-for-byte unchanged. A successful acknowledgement never changes
	// any other attachment, Session.StreamHead, Session.Version, or any
	// ControlIntent. It reports *NotFoundError when SessionID does not
	// identify an existing session or AttachmentID does not identify an
	// existing attachment on that session.
	AcknowledgeAttachment(ctx context.Context, req AcknowledgeAttachmentRequest) (AcknowledgeAttachmentResult, error)
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

// RecordPendingFactorySessionRequest identifies the exact session/episode/
// turn/version snapshot a caller observed when it started a Factory Session,
// plus the started (not yet committed) Factory Session identity to record --
// or a blank FactorySessionID to explicitly clear a previously recorded
// pending identity (for example after abandoning it in favor of a
// different, already-committed identity).
type RecordPendingFactorySessionRequest struct {
	SessionID string
	// ExpectedVersion is the session's version observed at admission --
	// StartTurnResult.Session.Version -- not a value re-read afterward.
	ExpectedVersion uint64
	// Episode is the TargetEpisode number the admitted turn belongs to --
	// StartTurnResult.Episode.Number.
	Episode uint64
	// TurnID is the admitted turn that started the Factory Session being
	// recorded -- StartTurnResult.Turn.ID.
	TurnID string
	// FactorySessionID is the started-but-uncommitted identity to record, or
	// blank to clear any previously recorded pending identity.
	FactorySessionID string
}

// RecordPendingFactorySessionResult carries the Session after a successful
// pending-identity record or clear.
type RecordPendingFactorySessionResult struct {
	Session Session
}

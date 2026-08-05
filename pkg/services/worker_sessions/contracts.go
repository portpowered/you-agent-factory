package workersessions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Service is the W1+W2+W3 Worker Session identity, registry, supervision,
// and Events publication foundation: stable identity reservation, immutable
// deterministic inspection, supervised Start with exactly-once terminal
// classification, a before-handoff opening record, and an after-output
// terminal SESSION record, plus PublishRecord for committing source-native
// Worker observations onto that same topic. StartTurn, Runtime persistence,
// and transport behavior are later ACP Worker Events slices. Provider Session
// association and controls are exposed here because this service owns
// one session's supervision state and is the only route to an admitted
// workstation dispatch's explicit cancellation boundary.
type Service interface {
	// Reserve validates req and, when req.ID is not already registered,
	// stores a new session in StateReserved and returns its snapshot.
	// Reserving an identity that already exists returns
	// ErrSessionAlreadyExists and leaves the existing session unchanged; this
	// is the one deterministic outcome for duplicate reservation.
	Reserve(ctx context.Context, req ReserveRequest) (Session, error)

	// Get returns the current immutable snapshot for req.ID. An invalid
	// req.ID returns ErrInvalidSessionID; a valid but unregistered req.ID
	// returns the distinguishable ErrSessionNotFound.
	Get(ctx context.Context, req GetRequest) (Session, error)

	// List returns immutable snapshots matching req.Filter in the documented
	// deterministic order (ascending by ID), independent of insertion order
	// or concurrent access. An empty registry or filter match returns a
	// successful empty ListResult, not a not-found failure. An invalid
	// filter value returns a typed validation error and no partial result.
	List(ctx context.Context, req ListRequest) (ListResult, error)

	// Start validates req, then establishes or reuses one stable Worker
	// Session identity in StateReserved, transitions StateStarting, and hands
	// a detached clone of req.Execution to the one directly injected
	// workers.WorkstationExecutionService. Start is synchronous: it returns
	// only after the attempt commits its exactly-once absorbing terminal
	// outcome, classified from the Workers WorkResult first and the adapter
	// error second. A cancel or terminate that wins after Reserve but before
	// Workers admission returns the established canceled terminal result
	// without starting Workers. Invalid requests and conflicting starts return
	// a typed error before any registry mutation or Workers call. Once
	// the terminal outcome commits, Start also appends one terminal
	// KindSession record (PhaseCompleted or PhaseFailed) to Topic(req.ID); a
	// failure publishing that record is logged and never changes the
	// returned, already-committed Session.
	Start(ctx context.Context, req StartRequest) (StartResult, error)

	// AssociateProviderSession records the exact Providers-owned SessionRef
	// observed by one currently supervised Worker Session attempt. The caller
	// must name the session's exact dispatch identity; Worker Sessions derives
	// the stable Worker Session, turn, and attempt correlation from that
	// supervised attempt rather than accepting caller-supplied substitutes.
	// Repeating the same reference is an idempotent duplicate. A different
	// reference for that attempt is rejected and never replaces the first
	// accepted association.
	AssociateProviderSession(context.Context, ProviderSessionAssociationRequest) (ProviderSessionAssociationResult, error)

	// PublishRecord validates req, then appends req.Draft, detached, as a
	// source-native Worker record onto Topic(req.SessionID) using req's
	// complete Events idempotency identity. PublishRecord only accepts a
	// record while req.SessionID's publication window is open -- after its
	// opening record has committed and before its terminal record has
	// started committing -- and only when req.SourceSequence does not
	// regress behind one already accepted for the same (SourceType,
	// SourceID), unless req's full four-part identity was itself already
	// accepted: an exact retry of a previously accepted identity always
	// reaches Events and resolves to the original record as a duplicate,
	// regardless of any later SourceSequence accepted since. Every accepted
	// call for one session is itself serialized, so its own opening,
	// publication, and terminal records can never interleave. Beyond that
	// window and ordering enforcement, PublishRecord
	// relies on Events for aggregate order, duplicate resolution, cursors,
	// reads, and subscriptions. An invalid Draft, an unopened or closed
	// publication window (ErrPublicationNotOpen), an out-of-order
	// SourceSequence (ErrOutOfOrderPublication), a malformed Events
	// identity, or an Events append failure is returned unchanged and
	// commits no record.
	PublishRecord(ctx context.Context, req PublishRecordRequest) (PublishRecordResult, error)

	// Pause returns an explicit unsupported result unless the supervised
	// execution capability can truthfully pause. It never fabricates PAUSED.
	Pause(ctx context.Context, req ControlRequest) (ControlResult, error)

	// Resume returns an explicit unsupported result until a matching paused
	// execution capability is available. It never fabricates RUNNING.
	Resume(ctx context.Context, req ControlRequest) (ControlResult, error)

	// Cancel explicitly stops an admitted dispatch through the injected
	// WorkstationPoolBoundary. It never uses caller-context cancellation as a
	// substitute for that boundary effect.
	Cancel(ctx context.Context, req ControlRequest) (ControlResult, error)

	// Terminate is Cancel with join semantics: a successful result is returned
	// only after the associated dispatch callback has completed.
	Terminate(ctx context.Context, req ControlRequest) (ControlResult, error)
}

// ReserveRequest asks Service to reserve one new Worker Session identity.
type ReserveRequest struct {
	ID string
}

// Validate reports whether req carries a non-empty stable identity. Validate
// is pure and does not mutate req.
func (req ReserveRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	return nil
}

// GetRequest asks Service to inspect one Worker Session identity.
type GetRequest struct {
	ID string
}

// Validate reports whether req carries a non-empty stable identity. Validate
// is pure and does not mutate req.
func (req GetRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	return nil
}

// Filter narrows List results. The zero-value Filter matches every session.
type Filter struct {
	// States restricts results to sessions whose State is in States. An
	// empty States matches every state.
	States []State
}

// Validate reports whether every state named in f.States is one of the eight
// accepted lifecycle states. Validate is pure and does not mutate f.
func (f Filter) Validate() error {
	for _, state := range f.States {
		if !state.Valid() {
			return ErrInvalidState
		}
	}
	return nil
}

// ListRequest asks Service for the deterministic filtered set of current
// Worker Session snapshots.
type ListRequest struct {
	Filter Filter
}

// Validate reports whether req.Filter is valid. Validate is pure and does not
// mutate req.
func (req ListRequest) Validate() error {
	return req.Filter.Validate()
}

// ListResult is the deterministic immutable snapshot collection returned by
// List. Mutating Sessions, or any element of it, after List returns never
// affects registry-owned state or a later List/Get result.
type ListResult struct {
	Sessions []Session
}

// StartRequest asks Service to supervise one already-resolved Workers
// execution attempt under a stable Worker Session identity.
type StartRequest struct {
	// ID is the stable Worker Session identity. If ID is not yet registered,
	// Start reserves it. If ID is already registered in StateReserved, Start
	// reuses that exact session and never creates a replacement. Any other
	// existing state is a conflicting start.
	ID string
	// Execution is the already-resolved Workers execution request. Start
	// hands a detached clone of Execution to the injected
	// workers.WorkstationExecutionService, so the caller retains exclusive
	// ownership of Execution's reference-backed fields after Start is
	// called. Worker Sessions performs no runner selection, prompt
	// rendering, worktree preparation, provider invocation, or output
	// shaping on this value.
	Execution workers.WorkstationDispatchRequest
}

// Validate reports whether req carries a non-empty stable identity and a
// minimally well-formed resolved execution request: a named workstation
// route, a non-empty attempt (dispatch) identity, and a nested dispatch
// workstation name that is non-empty and matches the top-level route. The
// nested-name checks mirror the same dispatch identity invariant the Workers
// boundary itself enforces (see validDispatch in
// pkg/services/workers/internal/services/workstations/internal/service/service.go),
// so Worker Sessions rejects a malformed resolved request before any
// registry mutation or Workers call instead of allowing Workers to reject it
// after effects have already happened. Validate is pure and does not mutate
// req, the registry, or call Workers.
func (req StartRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	workstationName := strings.TrimSpace(req.Execution.WorkstationName)
	if workstationName == "" {
		return fmt.Errorf("%w: workstation name is required", ErrInvalidExecutionRequest)
	}
	if strings.TrimSpace(req.Execution.Execution.Dispatch.DispatchID) == "" {
		return fmt.Errorf("%w: attempt (dispatch) id is required", ErrInvalidExecutionRequest)
	}
	// The nested comparison intentionally compares the RAW (untrimmed)
	// nested dispatch workstation name against the already-trimmed
	// top-level workstationName, mirroring validDispatch in
	// pkg/services/workers/internal/services/workstations/internal/service/service.go
	// exactly: that function trims only the top-level name and requires
	// dispatch.WorkstationName == name with no trimming applied to the
	// nested value. Trimming both sides here would incorrectly accept a
	// whitespace-padded nested name that the real Workers boundary rejects.
	rawNestedWorkstationName := req.Execution.Execution.Dispatch.WorkstationName
	if strings.TrimSpace(rawNestedWorkstationName) == "" {
		return fmt.Errorf("%w: nested dispatch workstation name is required", ErrInvalidExecutionRequest)
	}
	if rawNestedWorkstationName != workstationName {
		return fmt.Errorf("%w: nested dispatch workstation name must match the top-level workstation name", ErrInvalidExecutionRequest)
	}
	return nil
}

// StartResult is the detached snapshot Start returns once the started
// session's exactly-once terminal outcome has been committed.
type StartResult struct {
	Session Session
	// Dispatch is the raw, detached workers.WorkstationDispatchResult
	// returned by the underlying workers.WorkstationExecutionService.
	// DispatchWorkstation call. It is populated whenever the attempt was
	// actually handed off to Workers, and is the zero value when Start
	// terminalized before handoff (for example
	// FailureCauseEventPublicationFailure). Session.Result carries only the
	// bounded, safe COMPLETED/FAILED classification; Dispatch carries the
	// full Workers-owned payload a trusted caller needs to preserve existing
	// Work materialization and output lineage behavior unchanged.
	Dispatch workers.WorkstationDispatchResult
	// DispatchErr is the raw adapter error returned alongside Dispatch, if
	// any, detached from registry-owned state.
	DispatchErr error
}

// ProviderSessionAssociation is the detached Worker Sessions-owned binding
// from one Worker Session attempt to the exact Providers-owned Provider
// Session reference it observed. DispatchID and AttemptID intentionally both
// retain the existing Workers attempt identity: Workers currently uses the
// dispatch ID as the Providers attempt ID, and keeping both fields makes that
// correlation explicit without reconstructing it later. TurnID is empty only
// for direct Worker Sessions that were not admitted from a Factory turn.
type ProviderSessionAssociation struct {
	WorkerSessionID string
	TurnID          string
	DispatchID      string
	AttemptID       string
	Reference       providers.SessionRef
}

// Validate checks the association's stable identity and exact Provider
// Session reference. It is pure and never canonicalizes, resolves, or
// reconstructs any part of Reference.
func (association ProviderSessionAssociation) Validate() error {
	if !validSessionID(association.WorkerSessionID) {
		return ErrInvalidSessionID
	}
	if strings.TrimSpace(association.DispatchID) == "" || strings.TrimSpace(association.AttemptID) == "" ||
		association.DispatchID != association.AttemptID {
		return ErrInvalidProviderSessionAssociation
	}
	return association.Reference.Validate()
}

// Clone returns a detached association snapshot.
func (association ProviderSessionAssociation) Clone() ProviderSessionAssociation {
	association.Reference = association.Reference.Clone()
	return association
}

// ProviderSessionAssociationRequest reports one exact Provider Session
// reference observed by a Worker attempt. Worker Sessions verifies the
// session and dispatch against its own supervision state before it records
// the association.
type ProviderSessionAssociationRequest struct {
	WorkerSessionID string
	DispatchID      string
	Reference       providers.SessionRef
}

// Validate checks only caller-owned request fields. Registry-owned attempt,
// turn, and Worker Session correlation is checked by AssociateProviderSession.
func (request ProviderSessionAssociationRequest) Validate() error {
	if !validSessionID(request.WorkerSessionID) {
		return ErrInvalidSessionID
	}
	if strings.TrimSpace(request.DispatchID) == "" {
		return ErrInvalidProviderSessionAssociation
	}
	return request.Reference.Validate()
}

// ProviderSessionAssociationOutcome distinguishes a new exact association
// from a retry of the already accepted association for the same attempt.
type ProviderSessionAssociationOutcome string

const (
	ProviderSessionAssociationOutcomeAccepted  ProviderSessionAssociationOutcome = "ACCEPTED"
	ProviderSessionAssociationOutcomeDuplicate ProviderSessionAssociationOutcome = "DUPLICATE"
)

// ProviderSessionAssociationResult is detached evidence for one association
// observation. Association is always the registry-owned accepted snapshot on
// success, including a duplicate observation.
type ProviderSessionAssociationResult struct {
	Association ProviderSessionAssociation
	Outcome     ProviderSessionAssociationOutcome
}

var (
	// ErrInvalidSessionID reports a request or session with an empty or
	// whitespace-only identity.
	ErrInvalidSessionID = errors.New("worker session: invalid session id")
	// ErrInvalidState reports a request, filter, or session naming a value
	// outside the eight accepted lifecycle states.
	ErrInvalidState = errors.New("worker session: invalid state")
	// ErrSessionAlreadyExists reports Reserve called with an identity that is
	// already registered.
	ErrSessionAlreadyExists = errors.New("worker session: already exists")
	// ErrSessionNotFound reports Get called with a valid but unregistered
	// identity.
	ErrSessionNotFound = errors.New("worker session: not found")
	// ErrInvalidExecutionRequest reports a Start request whose resolved
	// Workers execution request is missing required identity fields.
	ErrInvalidExecutionRequest = errors.New("worker session: invalid execution request")
	// ErrSessionNotStartable reports Start called for an identity that is
	// already registered outside StateReserved (already starting, running,
	// paused, or terminal). No Workers call is made and the existing session
	// is left unchanged.
	ErrSessionNotStartable = errors.New("worker session: not startable")
	// ErrPublicationNotOpen reports PublishRecord called for a session whose
	// publication window is not open: the session was only ever reserved,
	// its opening record has not yet committed, or its terminal record has
	// already started committing. No record is committed.
	ErrPublicationNotOpen = errors.New("worker session: publication is not open")
	// ErrOutOfOrderPublication reports PublishRecord called with a
	// SourceSequence that regresses behind one already accepted for the same
	// (SourceType, SourceID). No record is committed.
	ErrOutOfOrderPublication = errors.New("worker session: source sequence is out of order")
	// ErrInvalidProviderSessionAssociation reports a malformed Worker Sessions
	// association correlation. Invalid provider, kind, or opaque Provider
	// Session identity retains the more specific Providers-owned typed error.
	ErrInvalidProviderSessionAssociation = errors.New("worker session: invalid provider session association")
	// ErrProviderSessionAssociationAttemptMismatch reports an observation
	// whose dispatch does not match the Worker Session's supervised attempt.
	ErrProviderSessionAssociationAttemptMismatch = errors.New("worker session: provider session association attempt mismatch")
	// ErrProviderSessionAssociationNotAvailable reports an observation for a
	// session that has not reached a live supervised attempt, or for a
	// terminal attempt that never committed an association.
	ErrProviderSessionAssociationNotAvailable = errors.New("worker session: provider session association is not available")
	// ErrProviderSessionAssociationConflict reports a different exact Provider
	// Session reference observed after one reference was already committed for
	// the same Worker Session attempt.
	ErrProviderSessionAssociationConflict = errors.New("worker session: provider session association conflict")
)

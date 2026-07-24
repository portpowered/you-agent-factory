package work

import (
	"context"
)

// Runtime is the narrow live-session capability consumed by Work operations.
type Runtime interface {
	SubmitWorkRequest(context.Context, WorkRequest) (WorkRequestSubmitResult, error)
	ReadWorkSnapshot(context.Context) (ReadSnapshot, error)
	MoveWork(
		context.Context,
		string,
		string,
		WorkStateChangeSource,
		string,
	) (OperatorMoveResult, error)
}

// RuntimeResolver resolves the Work-facing runtime for one Factory Session.
// Factory Sessions adapts its registry to this consumer-owned port.
type RuntimeResolver interface {
	ResolveWorkRuntime(string) (Runtime, error)
}

// RequestIDGenerator supplies opaque identity components for Work Requests and
// chaining traces. Wire selects the production generator; callers that submit
// fully identified requests do not need to invoke it.
type RequestIDGenerator func() string

// SubmittedFileReader reads a canonical Work Request submitted by path.
// Filesystem policy is selected at the process edge rather than by Work.
type SubmittedFileReader func(string) ([]byte, error)

// Service is the singular Work root contract for cross-service peers.
// Published slices (admission, content staging/materialization, state access,
// and invocation/return policy) are additive methods on this interface and use
// plain Work-owned request, result, value, and typed-error contracts.
// Runtime state and event queries belong to Factory Runtime and Recordings.
type Service interface {
	// SubmitWorkRequestForSession is the published admission slice. Peers submit
	// an already-decoded WorkRequest covering request identity and payload, and
	// receive detached WorkRequestSubmitResult acceptance facts or a typed
	// admission failure (ErrInvalidWorkRequest, ErrWorkRequestConflict, or
	// ErrWorkRequestRejected). Path-backed or protocol decoding is not part of
	// this root domain seam; see FileSubmissionService for file adapters.
	SubmitWorkRequestForSession(context.Context, string, WorkRequest) (WorkRequestSubmitResult, error)

	// MoveWorkForSession is part of the published state-access slice. Peers apply
	// an operator move with Work identity, target state name, and requestId, and
	// receive the detached OperatorMoveResult success shape or a typed failure
	// such as ErrMoveWorkRequestAlreadyApplied.
	MoveWorkForSession(context.Context, string, string, string, string) (OperatorMoveResult, error)
	// ListWork is part of the published state-access slice. Peers supply plain
	// ListOptions and receive detached ListResult ReadModel projections with no
	// token, place, marking, or topology fields.
	ListWork(context.Context, string, ListOptions) (ListResult, error)
	// GetWork is part of the published state-access slice. Peers look up one Work
	// by id and receive a detached ReadModel, or ErrWorkNotFound when missing.
	GetWork(context.Context, string, string) (ReadModel, error)
	// MoveWorkAndRead is the combined state-access outcome peers already rely on:
	// apply an operator move, then return the detached post-move ReadModel (or a
	// typed state-access failure such as ErrWorkNotFound or
	// ErrMoveWorkRequestAlreadyApplied).
	MoveWorkAndRead(context.Context, string, string, string, string) (ReadModel, error)

	// StageContent is part of the published content staging slice. Peers stage
	// already-decoded content bytes through plain StageContentRequest and receive
	// an opaque StageContentResult reference without supplying filesystem effect
	// interfaces on the request shape.
	StageContent(context.Context, StageContentRequest) (StageContentResult, error)
	// PrepareContent resolves staged submission items into detached Work content
	// parts peers can attach to admission requests.
	PrepareContent(context.Context, []StagedSubmissionItem) ([]WorkContentPart, error)
	// ResolveContent resolves an opaque staged reference to a local path and URL,
	// or returns a typed staging failure such as ErrInvalidStagedContentRef or
	// ErrStagedContentExpired.
	ResolveContent(context.Context, string) (ResolvedStagedContent, error)
	// CleanupContent releases resources owned by an opaque staged reference.
	CleanupContent(context.Context, string) error
	// MaterializeContentURL is the published content materialization slice. Peers
	// supply a content URL and receive an immutable local path plus ContentCleanup
	// handle, or a typed materialization failure such as ErrUnsafeContentURL or
	// ErrContentURLInaccessible. Callers do not supply HTTP or filesystem effect
	// interfaces on the request shape.
	MaterializeContentURL(context.Context, string) (string, ContentCleanup, error)

	// PrepareInvocationInput is part of the published invocation/return-policy
	// slice. Peers supply already-collected edge values through plain
	// InvocationInputPreparationRequest and receive detached PreparedInvocationInput,
	// or a typed failure such as ErrInvalidInvocationInput (including structured
	// *InputError values). Peers are not required to construct a separate
	// InvocationInputPreparation helper for this published slice.
	PrepareInvocationInput(context.Context, InvocationInputPreparationRequest) (PreparedInvocationInput, error)
	// ResolvePrimaryResult is part of the published invocation/return-policy
	// slice. Peers supply PrimaryResultSelectionInput (request identity, authored
	// return policy, and Work-owned world-state projection) and receive detached
	// PrimaryResultSelection for SUBMITTED_WORK_TERMINAL and EXPLICIT policies, or
	// a typed failure such as ErrUnsupportedReturnPolicy or *PrimaryResultError.
	ResolvePrimaryResult(context.Context, PrimaryResultSelectionInput) (PrimaryResultSelection, error)
}

// FileSubmissionService is the exact Work role for path-backed submission.
// Path decoding stays adapter-owned; peers that need only the domain root
// should depend on Service rather than this file-submission role.
type FileSubmissionService interface {
	Service
	SubmitFileForSession(context.Context, string, string) (WorkRequestSubmitResult, error)
}

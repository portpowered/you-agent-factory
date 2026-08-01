package factory_visualization

import (
	"context"
	"time"
)

// Service is the singular peer-facing Factory Visualization contract.
//
// Cross-service consumers depend on this named root for request-activated
// lifecycle, live projection, and presentation/drain slices. Collaborator
// ports and construction helpers remain private to the service wire.
type Service interface {
	// Activate leaves the inert constructed state through explicit request
	// parameters and starts retained-then-live Factory event projection.
	Activate(context.Context, ActivateRequest) (ActivateResult, error)
	// Join waits for the live subscription to exit. Calling Join before
	// Activate returns a typed not-activated failure while the root remains
	// inert.
	Join(context.Context, JoinRequest) (JoinResult, error)
	// StopDrain cancels the live subscription and emits one final projected
	// view through the Visualization-owned drain path.
	StopDrain(context.Context, StopDrainRequest) (StopDrainResult, error)
	// Observe returns one detached retained-then-live Factory view projection
	// through Visualization-owned plain contracts.
	Observe(context.Context, ObserveRequest) (ObserveResult, error)
	// OpenPresentation opens one Visualization-owned presentation output using
	// best-effort or lossless drain policy.
	OpenPresentation(context.Context, OpenPresentationRequest) (OpenPresentationResult, error)
	// PresentProgress enqueues ordered progress records onto an opened
	// presentation session.
	PresentProgress(context.Context, PresentProgressRequest) (PresentProgressResult, error)
	// FinalizePresentation drains accepted progress then commits one terminal
	// write owned by Visualization final-write ordering.
	FinalizePresentation(context.Context, FinalizePresentationRequest) (FinalizePresentationResult, error)
	// ClosePresentation closes and drains a presentation session without a
	// terminal write.
	ClosePresentation(context.Context, ClosePresentationRequest) (ClosePresentationResult, error)
}

// Root is the compatibility name for the singular Service contract.
type Root = Service

// ActivateMode selects how visualization leaves the inert constructed state.
type ActivateMode string

const (
	// ActivateModeRetainedThenLive activates retained history projection then
	// live event observation — the existing Visualization Start vocabulary.
	ActivateModeRetainedThenLive ActivateMode = "RETAINED_THEN_LIVE"
)

// LifecycleState is the published request-activated lifecycle vocabulary.
type LifecycleState string

const (
	LifecycleStateInert   LifecycleState = "INERT"
	LifecycleStateStarted LifecycleState = "STARTED"
	LifecycleStateStopped LifecycleState = "STOPPED"
)

// LifecycleErrorKind distinguishes typed Visualization lifecycle outcomes.
type LifecycleErrorKind string

const (
	LifecycleErrorMissingParameters LifecycleErrorKind = "MISSING_PARAMETERS"
	LifecycleErrorAlreadyActivated  LifecycleErrorKind = "ALREADY_ACTIVATED"
	LifecycleErrorNotActivated      LifecycleErrorKind = "NOT_ACTIVATED"
)

// ActivateRequest carries the explicit parameters required to leave the inert
// constructed state. A zero-value request is rejected.
type ActivateRequest struct {
	Mode ActivateMode
}

// ActivateResult is the published outcome of a successful Activate call.
type ActivateResult struct {
	State LifecycleState
}

// JoinRequest carries wait/join parameters for the live subscription.
type JoinRequest struct{}

// JoinResult is the published outcome of a successful Join call.
type JoinResult struct {
	State LifecycleState
}

// StopDrainRequest carries stop-and-drain-final-view parameters.
type StopDrainRequest struct{}

// StopDrainResult is the published outcome of a successful StopDrain call.
type StopDrainResult struct {
	State LifecycleState
}

// LifecycleError is a typed Visualization lifecycle failure peers can branch on.
type LifecycleError struct {
	Kind    LifecycleErrorKind
	Message string
	Cause   error
}

func (e *LifecycleError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *LifecycleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ObserveMode selects how live projection is obtained through the root.
type ObserveMode string

const (
	// ObserveModeRetainedThenLive projects retained history then live
	// observation facts into one detached Visualization view.
	ObserveModeRetainedThenLive ObserveMode = "RETAINED_THEN_LIVE"
)

// ObserveReconnectCursor is the Visualization-owned reconnect observe input.
// Peers supply AfterEventID and/or AfterSequence; both empty is invalid.
type ObserveReconnectCursor struct {
	AfterEventID  string
	AfterSequence *int
}

// ObserveRequest carries explicit live-projection parameters. A zero-value
// request is rejected as invalid input.
type ObserveRequest struct {
	Mode      ObserveMode
	Reconnect *ObserveReconnectCursor
}

// ProjectedView is a Visualization-owned detached live view. Engine-state and
// observation facts are plain values so peers do not import Recordings ledger
// storage or Runtime Petri/JavaScript internals.
type ProjectedView struct {
	TickCount          int
	RetainedEventCount int
	ObservedAt         time.Time
}

// ObserveResult is the published outcome of a successful Observe call.
type ObserveResult struct {
	View ProjectedView
}

// ProjectionErrorKind distinguishes typed Visualization live-projection outcomes.
type ProjectionErrorKind string

const (
	ProjectionErrorInvalidInput         ProjectionErrorKind = "INVALID_INPUT"
	ProjectionErrorSnapshotUnavailable  ProjectionErrorKind = "SNAPSHOT_UNAVAILABLE"
	ProjectionErrorReconstructionFailed ProjectionErrorKind = "RECONSTRUCTION_FAILED"
)

// ProjectionError is a typed Visualization live-projection failure peers can branch on.
type ProjectionError struct {
	Kind    ProjectionErrorKind
	Message string
	Cause   error
}

func (e *ProjectionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *ProjectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// PresentationDeliveryMode selects Visualization-owned drain/backpressure policy.
type PresentationDeliveryMode string

const (
	// PresentationDeliveryBestEffort may reject progress under backlog pressure.
	PresentationDeliveryBestEffort PresentationDeliveryMode = "BEST_EFFORT"
	// PresentationDeliveryLossless retains every accepted progress record until
	// close/finalize drain completes.
	PresentationDeliveryLossless PresentationDeliveryMode = "LOSSLESS"
)

// PresentationSessionID identifies one opened presentation/drain session.
type PresentationSessionID string

// PresentationErrorKind distinguishes typed Visualization presentation/drain outcomes.
type PresentationErrorKind string

const (
	PresentationErrorInvalidInput          PresentationErrorKind = "INVALID_INPUT"
	PresentationErrorEnqueueAfterClose     PresentationErrorKind = "ENQUEUE_AFTER_CLOSE"
	PresentationErrorFinalizeWithoutWriter PresentationErrorKind = "FINALIZE_WITHOUT_WRITER"
	PresentationErrorBackpressureRejected  PresentationErrorKind = "BACKPRESSURE_REJECTED"
)

// PresentationError is a typed Visualization presentation/drain failure peers can branch on.
type PresentationError struct {
	Kind    PresentationErrorKind
	Message string
	Cause   error
}

func (e *PresentationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *PresentationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// OpenPresentationRequest carries explicit presentation open parameters.
type OpenPresentationRequest struct {
	Mode PresentationDeliveryMode
}

// OpenPresentationResult is the published outcome of opening a presentation session.
type OpenPresentationResult struct {
	SessionID PresentationSessionID
	Mode      PresentationDeliveryMode
}

// ProgressRecord is one Visualization-owned progress payload.
type ProgressRecord struct {
	Payload []byte
}

// PresentProgressRequest enqueues ordered progress onto an opened session.
type PresentProgressRequest struct {
	SessionID PresentationSessionID
	Records   []ProgressRecord
}

// PresentProgressResult reports how many progress records were accepted.
type PresentProgressResult struct {
	AcceptedCount int
}

// TerminalWrite is the Visualization-owned terminal payload committed after drain.
type TerminalWrite struct {
	Payload []byte
}

// FinalizePresentationRequest finalizes one presentation session after drain.
// A nil Terminal is the typed finalize-without-writer failure.
type FinalizePresentationRequest struct {
	SessionID PresentationSessionID
	Terminal  *TerminalWrite
}

// FinalizePresentationResult is the published finalize outcome.
type FinalizePresentationResult struct {
	Finalized    bool
	ProgressSeen bool
}

// ClosePresentationRequest closes and drains without a terminal write.
type ClosePresentationRequest struct {
	SessionID PresentationSessionID
}

// ClosePresentationResult reports close-and-drain outcomes peers need.
type ClosePresentationResult struct {
	DroppedCount int
}

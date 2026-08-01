package service

import factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"

// Root contract aliases keep the implementation package private while the
// public request/result vocabulary remains owned by factory_visualization.
type (
	Root = factoryvisualization.Service

	View               = factoryvisualization.View
	RuntimeObservation = factoryvisualization.RuntimeObservation
	Sink               = factoryvisualization.Sink
	SinkFunc           = factoryvisualization.SinkFunc
	Clock              = factoryvisualization.Clock
	Source             = factoryvisualization.Source
	ErrorReporter      = factoryvisualization.ErrorReporter
	RuntimeReader      = factoryvisualization.RuntimeReader

	ActivateMode       = factoryvisualization.ActivateMode
	LifecycleState     = factoryvisualization.LifecycleState
	LifecycleErrorKind = factoryvisualization.LifecycleErrorKind
	ActivateRequest    = factoryvisualization.ActivateRequest
	ActivateResult     = factoryvisualization.ActivateResult
	JoinRequest        = factoryvisualization.JoinRequest
	JoinResult         = factoryvisualization.JoinResult
	StopDrainRequest   = factoryvisualization.StopDrainRequest
	StopDrainResult    = factoryvisualization.StopDrainResult
	LifecycleError     = factoryvisualization.LifecycleError

	ObserveMode            = factoryvisualization.ObserveMode
	ObserveReconnectCursor = factoryvisualization.ObserveReconnectCursor
	ObserveRequest         = factoryvisualization.ObserveRequest
	ProjectedView          = factoryvisualization.ProjectedView
	ObserveResult          = factoryvisualization.ObserveResult
	ProjectionErrorKind    = factoryvisualization.ProjectionErrorKind
	ProjectionError        = factoryvisualization.ProjectionError

	PresentationDeliveryMode    = factoryvisualization.PresentationDeliveryMode
	PresentationSessionID       = factoryvisualization.PresentationSessionID
	PresentationErrorKind       = factoryvisualization.PresentationErrorKind
	PresentationError           = factoryvisualization.PresentationError
	OpenPresentationRequest     = factoryvisualization.OpenPresentationRequest
	OpenPresentationResult      = factoryvisualization.OpenPresentationResult
	ProgressRecord              = factoryvisualization.ProgressRecord
	PresentProgressRequest      = factoryvisualization.PresentProgressRequest
	PresentProgressResult       = factoryvisualization.PresentProgressResult
	TerminalWrite               = factoryvisualization.TerminalWrite
	FinalizePresentationRequest = factoryvisualization.FinalizePresentationRequest
	FinalizePresentationResult  = factoryvisualization.FinalizePresentationResult
	ClosePresentationRequest    = factoryvisualization.ClosePresentationRequest
	ClosePresentationResult     = factoryvisualization.ClosePresentationResult
)

const (
	ActivateModeRetainedThenLive           = factoryvisualization.ActivateModeRetainedThenLive
	LifecycleStateStarted                  = factoryvisualization.LifecycleStateStarted
	LifecycleErrorMissingParameters        = factoryvisualization.LifecycleErrorMissingParameters
	LifecycleErrorAlreadyActivated         = factoryvisualization.LifecycleErrorAlreadyActivated
	LifecycleErrorNotActivated             = factoryvisualization.LifecycleErrorNotActivated
	LifecycleStateStopped                  = factoryvisualization.LifecycleStateStopped
	ObserveModeRetainedThenLive            = factoryvisualization.ObserveModeRetainedThenLive
	ProjectionErrorInvalidInput            = factoryvisualization.ProjectionErrorInvalidInput
	ProjectionErrorSnapshotUnavailable     = factoryvisualization.ProjectionErrorSnapshotUnavailable
	ProjectionErrorReconstructionFailed    = factoryvisualization.ProjectionErrorReconstructionFailed
	PresentationDeliveryBestEffort         = factoryvisualization.PresentationDeliveryBestEffort
	PresentationDeliveryLossless           = factoryvisualization.PresentationDeliveryLossless
	PresentationErrorInvalidInput          = factoryvisualization.PresentationErrorInvalidInput
	PresentationErrorEnqueueAfterClose     = factoryvisualization.PresentationErrorEnqueueAfterClose
	PresentationErrorFinalizeWithoutWriter = factoryvisualization.PresentationErrorFinalizeWithoutWriter
	PresentationErrorBackpressureRejected  = factoryvisualization.PresentationErrorBackpressureRejected
)

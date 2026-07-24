package factorysessionexecution

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// Type ownership lives on the Factory Sessions root. This package aliases
// root contracts so nested execution code keeps local names without being
// the peer-facing source of truth.

type (
	ApproveRequest               = factorysessions.ApproveRequest
	ArtifactDetail               = factorysessions.ArtifactDetail
	ArtifactRedactionCounts      = factorysessions.ArtifactRedactionCounts
	ArtifactRefSummary           = factorysessions.ArtifactRefSummary
	ArtifactRetrievalRef         = factorysessions.ArtifactRetrievalRef
	ArtifactSummary              = factorysessions.ArtifactSummary
	AsyncStartResult             = factorysessions.AsyncStartResult
	CheckpointRef                = factorysessions.CheckpointRef
	ControlError                 = factorysessions.ControlError
	ControlRequest               = factorysessions.ControlRequest
	DispatchDetail               = factorysessions.DispatchDetail
	DispatchFailureDetail        = factorysessions.DispatchFailureDetail
	DispatchFilters              = factorysessions.DispatchFilters
	DispatchJavaScriptProjection = factorysessions.DispatchJavaScriptProjection
	DispatchPetriProjection      = factorysessions.DispatchPetriProjection
	DispatchQueryRequest         = factorysessions.DispatchQueryRequest
	DispatchStatus               = factorysessions.DispatchStatus
	DispatchSummary              = factorysessions.DispatchSummary
	DispatchUsage                = factorysessions.DispatchUsage
	DispatchWarning              = factorysessions.DispatchWarning
	DurableSessionListSummary    = factorysessions.DurableSessionListSummary
	EventReadResult              = factorysessions.EventReadResult
	EventReconnectRequest        = factorysessions.EventReconnectRequest
	ExecutionProvider            = factorysessions.ExecutionProvider
	FactoryEventConsumer         = factorysessions.FactoryEventConsumer
	FailureSummary               = factorysessions.FailureSummary
	InlineWorkflowSource         = factorysessions.InlineWorkflowSource
	InspectionLinks              = factorysessions.InspectionLinks
	InterruptDispatchRequest     = factorysessions.InterruptDispatchRequest
	LifecycleControlKind         = factorysessions.LifecycleControlKind
	LifecycleControlLinks        = factorysessions.LifecycleControlLinks
	LifecycleControlOutcome      = factorysessions.LifecycleControlOutcome
	LifecycleControlResult       = factorysessions.LifecycleControlResult
	LifecycleStatus              = factorysessions.LifecycleStatus
	LifecycleTimestamps          = factorysessions.LifecycleTimestamps
	ListArtifactsResult          = factorysessions.ListArtifactsResult
	ListDispatchesResult         = factorysessions.ListDispatchesResult
	ListSessionsRequest          = factorysessions.ListSessionsRequest
	ListSessionsResult           = factorysessions.ListSessionsResult
	LiveSessionSummary           = factorysessions.LiveSessionSummary
	OrchestratorOverride         = factorysessions.OrchestratorOverride
	PersistencePolicy            = factorysessions.PersistencePolicy
	PhaseSummary                 = factorysessions.PhaseSummary
	PolicyProjection             = factorysessions.PolicyProjection
	ProgressCounts               = factorysessions.ProgressCounts
	ProviderSessionRef           = factorysessions.ProviderSessionRef
	ResolvedSource               = factorysessions.ResolvedSource
	ResourceUsage                = factorysessions.ResourceUsage
	ResultAvailabilityDetail     = factorysessions.ResultAvailabilityDetail
	ResultMode                   = factorysessions.ResultMode
	ResultReadResult             = factorysessions.ResultReadResult
	ResultRequest                = factorysessions.ResultRequest
	ResultStatus                 = factorysessions.ResultStatus
	ResultSummary                = factorysessions.ResultSummary
	ResumeError                  = factorysessions.ResumeError
	ResumeOutcome                = factorysessions.ResumeOutcome
	ResumeSessionRequest         = factorysessions.ResumeSessionRequest
	RetryDispatchRequest         = factorysessions.RetryDispatchRequest
	RuntimeOptions               = factorysessions.RuntimeOptions
	Service                      = factorysessions.ExecutionService
	SessionActionAvailability    = factorysessions.SessionActionAvailability
	SessionBudgets               = factorysessions.SessionBudgets
	SessionListFilters           = factorysessions.SessionListFilters
	SessionListScope             = factorysessions.SessionListScope
	SessionReadResult            = factorysessions.SessionReadResult
	SessionUsage                 = factorysessions.SessionUsage
	Source                       = factorysessions.Source
	StartRequest                 = factorysessions.StartRequest
	SyncOutcome                  = factorysessions.SyncOutcome
	SyncStartResult              = factorysessions.SyncStartResult
	ValidationError              = factorysessions.ValidationError
	WaitOptions                  = factorysessions.WaitOptions
)

const (
	ChildExecutorModeFake                  = factorysessions.ChildExecutorModeFake
	ChildExecutorModeLive                  = factorysessions.ChildExecutorModeLive
	DefaultSessionListScope                = factorysessions.DefaultSessionListScope
	ExecutionProviderFake                  = factorysessions.ExecutionProviderFake
	ExecutionProviderJavaScriptRuntime     = factorysessions.ExecutionProviderJavaScriptRuntime
	LifecycleControlApprove                = factorysessions.LifecycleControlApprove
	LifecycleControlCancel                 = factorysessions.LifecycleControlCancel
	LifecycleControlInterruptDispatch      = factorysessions.LifecycleControlInterruptDispatch
	LifecycleControlOutcomeAccepted        = factorysessions.LifecycleControlOutcomeAccepted
	LifecycleControlOutcomeClassNotFound   = factorysessions.LifecycleControlOutcomeClassNotFound
	LifecycleControlOutcomeConflict        = factorysessions.LifecycleControlOutcomeConflict
	LifecycleControlOutcomeInvalidState    = factorysessions.LifecycleControlOutcomeInvalidState
	LifecycleControlOutcomeNoOp            = factorysessions.LifecycleControlOutcomeNoOp
	LifecycleControlOutcomeTerminalSession = factorysessions.LifecycleControlOutcomeTerminalSession
	LifecycleControlPause                  = factorysessions.LifecycleControlPause
	LifecycleControlResume                 = factorysessions.LifecycleControlResume
	LifecycleControlRetryDispatch          = factorysessions.LifecycleControlRetryDispatch
	LifecycleControlTerminate              = factorysessions.LifecycleControlTerminate
	LifecycleStatusAwaitingApproval        = factorysessions.LifecycleStatusAwaitingApproval
	LifecycleStatusCanceled                = factorysessions.LifecycleStatusCanceled
	LifecycleStatusCanceling               = factorysessions.LifecycleStatusCanceling
	LifecycleStatusFailed                  = factorysessions.LifecycleStatusFailed
	LifecycleStatusInterrupted             = factorysessions.LifecycleStatusInterrupted
	LifecycleStatusPaused                  = factorysessions.LifecycleStatusPaused
	LifecycleStatusQueued                  = factorysessions.LifecycleStatusQueued
	LifecycleStatusResuming                = factorysessions.LifecycleStatusResuming
	LifecycleStatusRunning                 = factorysessions.LifecycleStatusRunning
	LifecycleStatusSucceeded               = factorysessions.LifecycleStatusSucceeded
	LifecycleStatusTerminated              = factorysessions.LifecycleStatusTerminated
	LifecycleStatusTimedOut                = factorysessions.LifecycleStatusTimedOut
	ResultModeFinal                        = factorysessions.ResultModeFinal
	ResultModePartial                      = factorysessions.ResultModePartial
	ResultStatusFailedWithPartial          = factorysessions.ResultStatusFailedWithPartial
	ResultStatusFinal                      = factorysessions.ResultStatusFinal
	ResultStatusNotReady                   = factorysessions.ResultStatusNotReady
	ResultStatusPartial                    = factorysessions.ResultStatusPartial
	ResultStatusUnavailable                = factorysessions.ResultStatusUnavailable
	ResumeOutcomeCorruptedPersistence      = factorysessions.ResumeOutcomeCorruptedPersistence
	ResumeOutcomeInvalidState              = factorysessions.ResumeOutcomeInvalidState
	ResumeOutcomeMissingCheckpoint         = factorysessions.ResumeOutcomeMissingCheckpoint
	SessionListScopeAll                    = factorysessions.SessionListScopeAll
	SessionListScopeLive                   = factorysessions.SessionListScopeLive
	SessionListScopePersisted              = factorysessions.SessionListScopePersisted
)

var (
	ErrArtifactNotFound           = factorysessions.ErrArtifactNotFound
	ErrDispatchNotFound           = factorysessions.ErrDispatchNotFound
	ErrExecutionRequestIDConflict = factorysessions.ErrExecutionRequestIDConflict
	ErrReconnectCursorNotFound    = factorysessions.ErrReconnectCursorNotFound
	ErrServiceNotConfigured       = factorysessions.ErrExecutionServiceNotConfigured
	ErrSessionNotFound            = factorysessions.ErrDurableSessionNotFound
)

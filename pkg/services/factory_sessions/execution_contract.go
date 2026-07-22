package factorysessions

import (
	"io/fs"

	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
)

// ExecutionOpeningFileSystem is the exact host-filesystem capability used to
// resolve omitted durable-execution project and fixture-catalog paths.
type ExecutionOpeningFileSystem interface {
	Getwd() (string, error)
	Stat(string) (fs.FileInfo, error)
}

// Durable Factory Session execution contracts are exposed from the owning
// service root. The concrete runtime implementation remains in execution.
type (
	ExecutionValidationError      = execution.ValidationError
	ApproveRequest                = execution.ApproveRequest
	ArtifactDetail                = execution.ArtifactDetail
	ArtifactRedactionCounts       = execution.ArtifactRedactionCounts
	ArtifactRefSummary            = execution.ArtifactRefSummary
	ArtifactRetrievalRef          = execution.ArtifactRetrievalRef
	ArtifactSummary               = execution.ArtifactSummary
	AsyncStartResult              = execution.AsyncStartResult
	CheckpointRef                 = execution.CheckpointRef
	ControlError                  = execution.ControlError
	ControlRequest                = execution.ControlRequest
	DispatchDetail                = execution.DispatchDetail
	DispatchFailureDetail         = execution.DispatchFailureDetail
	DispatchFilters               = execution.DispatchFilters
	DispatchJavaScriptProjection  = execution.DispatchJavaScriptProjection
	DispatchQueryRequest          = execution.DispatchQueryRequest
	DispatchPetriProjection       = execution.DispatchPetriProjection
	DispatchStatus                = execution.DispatchStatus
	DispatchSummary               = execution.DispatchSummary
	DispatchUsage                 = execution.DispatchUsage
	DispatchWarning               = execution.DispatchWarning
	DurableSessionListSummary     = execution.DurableSessionListSummary
	EventReadResult               = execution.EventReadResult
	EventReconnectRequest         = execution.EventReconnectRequest
	ExecutionFactoryEventConsumer = execution.FactoryEventConsumer
	ExecutionProvider             = execution.ExecutionProvider
	ExecutionService              = execution.Service
	FailureSummary                = execution.FailureSummary
	InlineWorkflowSource          = execution.InlineWorkflowSource
	InspectionLinks               = execution.InspectionLinks
	InterruptDispatchRequest      = execution.InterruptDispatchRequest
	LifecycleControlKind          = execution.LifecycleControlKind
	LifecycleControlLinks         = execution.LifecycleControlLinks
	LifecycleControlOutcome       = execution.LifecycleControlOutcome
	LifecycleControlResult        = execution.LifecycleControlResult
	LifecycleStatus               = execution.LifecycleStatus
	LifecycleTimestamps           = execution.LifecycleTimestamps
	ListArtifactsResult           = execution.ListArtifactsResult
	ListDispatchesResult          = execution.ListDispatchesResult
	ListSessionsRequest           = execution.ListSessionsRequest
	ListSessionsResult            = execution.ListSessionsResult
	LiveSessionSummary            = execution.LiveSessionSummary
	OrchestratorOverride          = execution.OrchestratorOverride
	PhaseSummary                  = execution.PhaseSummary
	PolicyProjection              = execution.PolicyProjection
	PersistencePolicy             = execution.PersistencePolicy
	ProgressCounts                = execution.ProgressCounts
	ProviderSessionRef            = execution.ProviderSessionRef
	ResourceUsage                 = execution.ResourceUsage
	ResolvedSource                = execution.ResolvedSource
	ResultAvailabilityDetail      = execution.ResultAvailabilityDetail
	ResultMode                    = execution.ResultMode
	ResultReadResult              = execution.ResultReadResult
	ResultRequest                 = execution.ResultRequest
	ResultStatus                  = execution.ResultStatus
	ResultSummary                 = execution.ResultSummary
	ResumeError                   = execution.ResumeError
	ResumeSessionRequest          = execution.ResumeSessionRequest
	RetryDispatchRequest          = execution.RetryDispatchRequest
	RuntimeOptions                = execution.RuntimeOptions
	SessionActionAvailability     = execution.SessionActionAvailability
	SessionBudgets                = execution.SessionBudgets
	SessionListFilters            = execution.SessionListFilters
	SessionListScope              = execution.SessionListScope
	SessionReadResult             = execution.SessionReadResult
	SessionUsage                  = execution.SessionUsage
	Source                        = execution.Source
	StartRequest                  = execution.StartRequest
	SyncOutcome                   = execution.SyncOutcome
	SyncStartResult               = execution.SyncStartResult
	WaitOptions                   = execution.WaitOptions
)

const (
	LifecycleControlOutcomeAccepted        = execution.LifecycleControlOutcomeAccepted
	LifecycleControlOutcomeInvalidState    = execution.LifecycleControlOutcomeInvalidState
	LifecycleControlOutcomeNoOp            = execution.LifecycleControlOutcomeNoOp
	LifecycleControlOutcomeTerminalSession = execution.LifecycleControlOutcomeTerminalSession

	LifecycleControlPause  = execution.LifecycleControlPause
	LifecycleControlResume = execution.LifecycleControlResume

	LifecycleStatusCanceling = execution.LifecycleStatusCanceling
	LifecycleStatusFailed    = execution.LifecycleStatusFailed
	LifecycleStatusPaused    = execution.LifecycleStatusPaused
	LifecycleStatusRunning   = execution.LifecycleStatusRunning
	LifecycleStatusSucceeded = execution.LifecycleStatusSucceeded

	ResultModeFinal      = execution.ResultModeFinal
	ResultModePartial    = execution.ResultModePartial
	ResultStatusNotReady = execution.ResultStatusNotReady
	ResultStatusFinal    = execution.ResultStatusFinal

	SessionListScopeAll                = execution.SessionListScopeAll
	SessionListScopeLive               = execution.SessionListScopeLive
	SessionListScopePersisted          = execution.SessionListScopePersisted
	DefaultSessionListScope            = execution.DefaultSessionListScope
	ExecutionProviderFake              = execution.ExecutionProviderFake
	ExecutionProviderJavaScriptRuntime = execution.ExecutionProviderJavaScriptRuntime
	ChildExecutorModeFake              = execution.ChildExecutorModeFake
	ChildExecutorModeLive              = execution.ChildExecutorModeLive

	// ContractFixtureCatalogRelativePath is the repository-relative durable
	// session fixture catalog used by explicit fake execution entrypoints.
	ContractFixtureCatalogRelativePath = "pkg/transports/http/testdata/durable-session-contract-fixtures.json"
)

var (
	ErrArtifactNotFound              = execution.ErrArtifactNotFound
	ErrDispatchNotFound              = execution.ErrDispatchNotFound
	ErrDurableSessionNotFound        = execution.ErrSessionNotFound
	ErrExecutionRequestIDConflict    = execution.ErrExecutionRequestIDConflict
	ErrReconnectCursorNotFound       = execution.ErrReconnectCursorNotFound
	ErrExecutionServiceNotConfigured = execution.ErrServiceNotConfigured

	ApplySessionListScope                  = execution.ApplySessionListScope
	DeriveSessionActionAvailability        = execution.DeriveSessionActionAvailability
	EmptySessionUsage                      = execution.EmptySessionUsage
	EvaluateLifecycleControl               = execution.EvaluateLifecycleControl
	IsRecoverableSession                   = execution.IsRecoverableSession
	LifecycleControlOutcomeClass           = execution.LifecycleControlOutcomeClass
	LifecycleStatusFromFactoryRuntimeState = execution.LifecycleStatusFromFactoryRuntimeState
	LifecycleControlLinksForSession        = execution.LifecycleControlLinksForSession
	LiveLifecycleControlLogFields          = execution.LiveLifecycleControlLogFields
	LiveLifecycleControlLinksForSession    = execution.LiveLifecycleControlLinksForSession
	NormalizeApproveRequest                = execution.NormalizeApproveRequest
	NormalizeControlRequest                = execution.NormalizeControlRequest
	NormalizeEventReconnectRequest         = execution.NormalizeEventReconnectRequest
	MaterializeEventReadStream             = execution.MaterializeEventReadStream
	NormalizeInterruptDispatchRequest      = execution.NormalizeInterruptDispatchRequest
	NormalizeListSessionsRequest           = execution.NormalizeListSessionsRequest
	NormalizeResultRequest                 = execution.NormalizeResultRequest
	NormalizeRetryDispatchRequest          = execution.NormalizeRetryDispatchRequest
	NormalizeStartRequest                  = execution.NormalizeStartRequest
)

const LifecycleControlOutcomeClassNotFound = execution.LifecycleControlOutcomeClassNotFound

// NewExecutionValidationError builds a durable-execution request validation
// error without colliding with Factory Session discovery validation.
func NewExecutionValidationError(field, message string) *ExecutionValidationError {
	return execution.NewValidationError(field, message)
}

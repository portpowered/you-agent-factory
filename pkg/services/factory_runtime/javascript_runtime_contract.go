package factory

import workflowruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/runtime"

type (
	JavaScriptRuntimeRequest             = workflowruntime.Request
	JavaScriptWorkerSettings             = workflowruntime.WorkerSettingsConfig
	JavaScriptWorkerPreset               = workflowruntime.WorkerPreset
	JavaScriptChildRecordSink            = workflowruntime.ChildRecordSink
	JavaScriptChildExecutorFactory       = workflowruntime.ChildExecutorFactory
	JavaScriptRuntimeHooks               = workflowruntime.Hooks
	JavaScriptRuntimeFailure             = workflowruntime.Failure
	JavaScriptRuntimeOutcome             = workflowruntime.Outcome
	JavaScriptRuntimeRecord              = workflowruntime.RuntimeRecord
	JavaScriptPhaseRecord                = workflowruntime.PhaseRecord
	JavaScriptLogRecord                  = workflowruntime.LogRecord
	JavaScriptArtifactRecord             = workflowruntime.ArtifactRecord
	JavaScriptCheckpointRecord           = workflowruntime.CheckpointRecord
	JavaScriptChildDispatchRecord        = workflowruntime.ChildDispatchRecord
	JavaScriptBudgetRecord               = workflowruntime.BudgetRecord
	JavaScriptChildExecutionRequest      = workflowruntime.ChildExecutionRequest
	JavaScriptChildDispatchIdentity      = workflowruntime.ChildDispatchIdentity
	JavaScriptChildExecutionResult       = workflowruntime.ChildExecutionResult
	JavaScriptChildExecutor              = workflowruntime.ChildExecutor
	JavaScriptFakeChildExecutor          = workflowruntime.FakeChildExecutor
	JavaScriptResumeContext              = workflowruntime.ResumeContext
	JavaScriptResumingChildExecutor      = workflowruntime.ResumingChildExecutor
	JavaScriptCompletedCheckpointSummary = workflowruntime.CompletedCheckpointSummary
)

const (
	JavaScriptRuntimeCodeCanceled            = workflowruntime.CodeCanceled
	JavaScriptRuntimeCodeTimeout             = workflowruntime.CodeTimeout
	JavaScriptRuntimeCodeScriptError         = workflowruntime.CodeScriptError
	JavaScriptRuntimeCodeUnresolvedFinal     = workflowruntime.CodeUnresolvedFinal
	JavaScriptRuntimeCodeDeniedCapability    = workflowruntime.CodeDeniedCapability
	JavaScriptRuntimeCodeUnsupportedFinal    = workflowruntime.CodeUnsupportedFinal
	JavaScriptRuntimeCodePreExecutionInvalid = workflowruntime.CodePreExecutionInvalid
	JavaScriptRuntimeCodeInvalidResult       = workflowruntime.CodeInvalidResult

	JavaScriptRecordKindPhase         = workflowruntime.RecordKindPhase
	JavaScriptRecordKindLog           = workflowruntime.RecordKindLog
	JavaScriptRecordKindArtifact      = workflowruntime.RecordKindArtifact
	JavaScriptRecordKindCheckpoint    = workflowruntime.RecordKindCheckpoint
	JavaScriptRecordKindBudget        = workflowruntime.RecordKindBudget
	JavaScriptRecordKindChildDispatch = workflowruntime.RecordKindChildDispatch

	JavaScriptChildDispatchStatusQueued    = workflowruntime.ChildDispatchStatusQueued
	JavaScriptChildDispatchStatusRunning   = workflowruntime.ChildDispatchStatusRunning
	JavaScriptChildDispatchStatusCompleted = workflowruntime.ChildDispatchStatusCompleted
	JavaScriptChildDispatchStatusFailed    = workflowruntime.ChildDispatchStatusFailed

	JavaScriptChildExecutionModeFake      = workflowruntime.ChildExecutionModeFake
	JavaScriptChildExecutionModeLive      = workflowruntime.ChildExecutionModeLive
	JavaScriptChildExecutionFailureReason = workflowruntime.ChildExecutionFailureReason
)

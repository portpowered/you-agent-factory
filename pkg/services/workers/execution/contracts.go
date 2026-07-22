// Package workerexecution retains same-service compatibility names while the
// canonical Worker execution contracts live at pkg/services/workers.
package workerexecution

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

type WorkerState = workers.WorkerState
type InferenceResponse = workers.InferenceResponse
type InferenceEventKind = workers.InferenceEventKind
type InferenceEvent = workers.InferenceEvent
type InferenceRequestEventPayload = workers.InferenceRequestEventPayload
type InferenceOutcome = workers.InferenceOutcome
type InferenceResponseEventPayload = workers.InferenceResponseEventPayload
type InferenceResponseFailureDetail = workers.InferenceResponseFailureDetail
type ModelEventKind = workers.ModelEventKind
type ModelEvent = workers.ModelEvent
type ModelResourceSummary = workers.ModelResourceSummary
type ModelRequestEventPayload = workers.ModelRequestEventPayload
type ModelResponseEventPayload = workers.ModelResponseEventPayload
type AgentRunResponseEvent = workers.AgentRunResponseEvent
type AgentRunResponseEventPayload = workers.AgentRunResponseEventPayload
type ScriptEventKind = workers.ScriptEventKind
type ScriptExecutionOutcome = workers.ScriptExecutionOutcome
type ScriptFailureType = workers.ScriptFailureType
type ScriptEvent = workers.ScriptEvent
type ScriptRequestEventPayload = workers.ScriptRequestEventPayload
type ScriptResponseEventPayload = workers.ScriptResponseEventPayload
type DispatchResponseEventPayload = workers.DispatchResponseEventPayload
type DispatchResourceEventRef = workers.DispatchResourceEventRef
type WorkMetricsEventPayload = workers.WorkMetricsEventPayload
type WorkResult = workers.WorkResult
type ProviderSessionMetadata = workers.ProviderSessionMetadata
type WorkOutcome = workers.WorkOutcome
type WorkMetrics = workers.WorkMetrics
type WorkDiagnostics = workers.WorkDiagnostics
type RenderedPromptDiagnostic = workers.RenderedPromptDiagnostic
type ProviderDiagnostic = workers.ProviderDiagnostic
type InvocationDiagnostic = workers.InvocationDiagnostic
type InvocationParameterDiagnostic = workers.InvocationParameterDiagnostic
type CommandDiagnostic = workers.CommandDiagnostic
type PanicDiagnostic = workers.PanicDiagnostic
type WorkFailureFamily = workers.WorkFailureFamily
type WorkFailureType = workers.WorkFailureType
type FailureDetail = workers.FailureDetail
type WorkFailureDecision = workers.WorkFailureDecision
type WorkFailureMetadata = workers.WorkFailureMetadata

const (
	InferenceEventKindRequest  = workers.InferenceEventKindRequest
	InferenceEventKindResponse = workers.InferenceEventKindResponse
	InferenceOutcomeSucceeded  = workers.InferenceOutcomeSucceeded
	InferenceOutcomeFailed     = workers.InferenceOutcomeFailed
	ModelEventKindRequest      = workers.ModelEventKindRequest
	ModelEventKindResponse     = workers.ModelEventKindResponse
	ScriptEventKindRequest     = workers.ScriptEventKindRequest
	ScriptEventKindResponse    = workers.ScriptEventKindResponse

	ScriptExecutionOutcomeSucceeded      = workers.ScriptExecutionOutcomeSucceeded
	ScriptExecutionOutcomeFailedExitCode = workers.ScriptExecutionOutcomeFailedExitCode
	ScriptExecutionOutcomeTimedOut       = workers.ScriptExecutionOutcomeTimedOut
	ScriptExecutionOutcomeProcessError   = workers.ScriptExecutionOutcomeProcessError
	ScriptFailureTypeTimeout             = workers.ScriptFailureTypeTimeout
	ScriptFailureTypeProcessError        = workers.ScriptFailureTypeProcessError

	OutcomeAccepted = workers.OutcomeAccepted
	OutcomeContinue = workers.OutcomeContinue
	OutcomeRejected = workers.OutcomeRejected
	OutcomeFailed   = workers.OutcomeFailed

	ProviderResponseMetadataDurationMS    = workers.ProviderResponseMetadataDurationMS
	ProviderResponseMetadataDurationAPIMS = workers.ProviderResponseMetadataDurationAPIMS
	ProviderResponseMetadataInputTokens   = workers.ProviderResponseMetadataInputTokens
	ProviderResponseMetadataOutputTokens  = workers.ProviderResponseMetadataOutputTokens

	WorkFailureFamilyTerminal  = workers.WorkFailureFamilyTerminal
	WorkFailureFamilyRetryable = workers.WorkFailureFamilyRetryable
	WorkFailureFamilyThrottle  = workers.WorkFailureFamilyThrottle

	WorkFailureTypeAuthFailure         = workers.WorkFailureTypeAuthFailure
	WorkFailureTypePermanentBadRequest = workers.WorkFailureTypePermanentBadRequest
	WorkFailureTypeThrottled           = workers.WorkFailureTypeThrottled
	WorkFailureTypeInternalServerError = workers.WorkFailureTypeInternalServerError
	WorkFailureTypeTimeout             = workers.WorkFailureTypeTimeout
	WorkFailureTypeUnknown             = workers.WorkFailureTypeUnknown
	WorkFailureTypeMisconfigured       = workers.WorkFailureTypeMisconfigured
	WorkFailureTypeCommandLineTooLong  = workers.WorkFailureTypeCommandLineTooLong
	WorkFailureTypeMissingExecutable   = workers.WorkFailureTypeMissingExecutable
)

var CanonicalProviderSessionProvider = workers.CanonicalProviderSessionProvider
var FailureDecisionFromMetadata = workers.FailureDecisionFromMetadata
var CloneProviderSessionMetadata = workers.CloneProviderSessionMetadata
var CloneWorkFailureMetadata = workers.CloneWorkFailureMetadata
var CloneFailureDetail = workers.CloneFailureDetail
var CloneWorkDiagnostics = workers.CloneWorkDiagnostics
var CloneInvocationDiagnostic = workers.CloneInvocationDiagnostic

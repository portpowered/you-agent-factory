// Package workerexecution is a transitional compile shim that re-exports the
// workstations execution compatibility surface from the private destination.
// Canonical contracts live at pkg/services/workers; baseline deletion of this
// path is owned by DEL-WRK.
package workerexecution

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution"
)

type (
	WorkerState                      = private.WorkerState
	InferenceResponse                = private.InferenceResponse
	InferenceEventKind               = private.InferenceEventKind
	InferenceEvent                   = private.InferenceEvent
	InferenceRequestEventPayload     = private.InferenceRequestEventPayload
	InferenceOutcome                 = private.InferenceOutcome
	InferenceResponseEventPayload    = private.InferenceResponseEventPayload
	InferenceResponseFailureDetail   = private.InferenceResponseFailureDetail
	ModelEventKind                   = private.ModelEventKind
	ModelEvent                       = private.ModelEvent
	ModelResourceSummary             = private.ModelResourceSummary
	ModelRequestEventPayload         = private.ModelRequestEventPayload
	ModelResponseEventPayload        = private.ModelResponseEventPayload
	AgentRunResponseEvent            = private.AgentRunResponseEvent
	AgentRunResponseEventPayload     = private.AgentRunResponseEventPayload
	ScriptEventKind                  = private.ScriptEventKind
	ScriptExecutionOutcome           = private.ScriptExecutionOutcome
	ScriptFailureType                = private.ScriptFailureType
	ScriptEvent                      = private.ScriptEvent
	ScriptRequestEventPayload        = private.ScriptRequestEventPayload
	ScriptResponseEventPayload       = private.ScriptResponseEventPayload
	DispatchResponseEventPayload     = private.DispatchResponseEventPayload
	DispatchResourceEventRef         = private.DispatchResourceEventRef
	WorkMetricsEventPayload          = private.WorkMetricsEventPayload
	WorkResult                       = private.WorkResult
	ProviderSessionMetadata          = private.ProviderSessionMetadata
	WorkOutcome                      = private.WorkOutcome
	WorkMetrics                      = private.WorkMetrics
	WorkDiagnostics                  = private.WorkDiagnostics
	RenderedPromptDiagnostic         = private.RenderedPromptDiagnostic
	ProviderDiagnostic               = private.ProviderDiagnostic
	InvocationDiagnostic             = private.InvocationDiagnostic
	InvocationParameterDiagnostic    = private.InvocationParameterDiagnostic
	CommandDiagnostic                = private.CommandDiagnostic
	PanicDiagnostic                  = private.PanicDiagnostic
	WorkFailureFamily                = private.WorkFailureFamily
	WorkFailureType                  = private.WorkFailureType
	FailureDetail                    = private.FailureDetail
	WorkFailureDecision              = private.WorkFailureDecision
	WorkFailureMetadata              = private.WorkFailureMetadata
	DataType                         = private.DataType
	Color                            = private.Color
	Token                            = private.Token
	History                          = private.History
	Failure                          = private.Failure
	RunnerToolExecutionMode          = private.RunnerToolExecutionMode
	RunnerBaselineCapability         = private.RunnerBaselineCapability
	RunnerOptionalCapability         = private.RunnerOptionalCapability
	RunnerOptionalCapabilityStatus   = private.RunnerOptionalCapabilityStatus
	RunnerOptionalCapabilitySupport  = private.RunnerOptionalCapabilitySupport
	RunnerCapabilities               = private.RunnerCapabilities
	RunnerMetadata                   = private.RunnerMetadata
	RunnerSelectionSource            = private.RunnerSelectionSource
	ResolvedRunnerSelection          = private.ResolvedRunnerSelection
	ResolvedModelOperationBinding    = private.ResolvedModelOperationBinding
	ModelOperationBindingSource      = private.ModelOperationBindingSource
	WorkstationExecutionRequest      = private.WorkstationExecutionRequest
	ProviderInferenceRequest         = private.ProviderInferenceRequest
	RunnerExecutionRequest           = private.RunnerExecutionRequest
	RunnerExecutionResult            = private.RunnerExecutionResult
	SubprocessExecutionRequest       = private.SubprocessExecutionRequest
	Context                          = private.Context
)

const (
	InferenceEventKindRequest  = private.InferenceEventKindRequest
	InferenceEventKindResponse = private.InferenceEventKindResponse
	InferenceOutcomeSucceeded  = private.InferenceOutcomeSucceeded
	InferenceOutcomeFailed     = private.InferenceOutcomeFailed
	ModelEventKindRequest      = private.ModelEventKindRequest
	ModelEventKindResponse     = private.ModelEventKindResponse
	ScriptEventKindRequest     = private.ScriptEventKindRequest
	ScriptEventKindResponse    = private.ScriptEventKindResponse

	ScriptExecutionOutcomeSucceeded      = private.ScriptExecutionOutcomeSucceeded
	ScriptExecutionOutcomeFailedExitCode = private.ScriptExecutionOutcomeFailedExitCode
	ScriptExecutionOutcomeCanceled       = private.ScriptExecutionOutcomeCanceled
	ScriptExecutionOutcomeTimedOut       = private.ScriptExecutionOutcomeTimedOut
	ScriptExecutionOutcomeProcessError   = private.ScriptExecutionOutcomeProcessError
	ScriptFailureTypeCanceled            = private.ScriptFailureTypeCanceled
	ScriptFailureTypeTimeout             = private.ScriptFailureTypeTimeout
	ScriptFailureTypeProcessError        = private.ScriptFailureTypeProcessError

	OutcomeAccepted = private.OutcomeAccepted
	OutcomeContinue = private.OutcomeContinue
	OutcomeRejected = private.OutcomeRejected
	OutcomeFailed   = private.OutcomeFailed

	ProviderResponseMetadataDurationMS    = private.ProviderResponseMetadataDurationMS
	ProviderResponseMetadataDurationAPIMS = private.ProviderResponseMetadataDurationAPIMS
	ProviderResponseMetadataInputTokens   = private.ProviderResponseMetadataInputTokens
	ProviderResponseMetadataOutputTokens  = private.ProviderResponseMetadataOutputTokens

	WorkFailureFamilyTerminal  = private.WorkFailureFamilyTerminal
	WorkFailureFamilyRetryable = private.WorkFailureFamilyRetryable
	WorkFailureFamilyThrottle  = private.WorkFailureFamilyThrottle

	WorkFailureTypeAuthFailure         = private.WorkFailureTypeAuthFailure
	WorkFailureTypePermanentBadRequest = private.WorkFailureTypePermanentBadRequest
	WorkFailureTypeThrottled           = private.WorkFailureTypeThrottled
	WorkFailureTypeInternalServerError = private.WorkFailureTypeInternalServerError
	WorkFailureTypeTimeout             = private.WorkFailureTypeTimeout
	WorkFailureTypeUnknown             = private.WorkFailureTypeUnknown
	WorkFailureTypeMisconfigured       = private.WorkFailureTypeMisconfigured
	WorkFailureTypeCommandLineTooLong  = private.WorkFailureTypeCommandLineTooLong
	WorkFailureTypeMissingExecutable   = private.WorkFailureTypeMissingExecutable

	DataTypeResource = private.DataTypeResource
	DataTypeWork     = private.DataTypeWork

	RunnerToolExecutionModeRequired = private.RunnerToolExecutionModeRequired
	RunnerToolExecutionModeDisabled = private.RunnerToolExecutionModeDisabled

	RunnerBaselineCapabilityPromptSubmission = private.RunnerBaselineCapabilityPromptSubmission
	RunnerBaselineCapabilityToolExecution    = private.RunnerBaselineCapabilityToolExecution

	RunnerOptionalCapabilityImageInput       = private.RunnerOptionalCapabilityImageInput
	RunnerOptionalCapabilitySessionResume    = private.RunnerOptionalCapabilitySessionResume
	RunnerOptionalCapabilityStructuredOutput = private.RunnerOptionalCapabilityStructuredOutput
	RunnerOptionalCapabilityWorkingDirectory = private.RunnerOptionalCapabilityWorkingDirectory
	RunnerOptionalCapabilityWorktree         = private.RunnerOptionalCapabilityWorktree

	RunnerOptionalCapabilityStatusSupported   = private.RunnerOptionalCapabilityStatusSupported
	RunnerOptionalCapabilityStatusUnsupported = private.RunnerOptionalCapabilityStatusUnsupported

	RunnerIDCodex     = private.RunnerIDCodex
	RunnerIDGemini    = private.RunnerIDGemini
	RunnerIDKiro      = private.RunnerIDKiro
	RunnerIDCursorCLI = private.RunnerIDCursorCLI
	RunnerIDOpenCode  = private.RunnerIDOpenCode
	RunnerIDPi        = private.RunnerIDPi
	RunnerIDAgy       = private.RunnerIDAgy

	RunnerSelectionSourceWorkstation    = private.RunnerSelectionSourceWorkstation
	RunnerSelectionSourceFactory        = private.RunnerSelectionSourceFactory
	RunnerSelectionSourceLegacyProvider = private.RunnerSelectionSourceLegacyProvider
	RunnerSelectionSourceDefault        = private.RunnerSelectionSourceDefault

	ModelOperationBindingSourceInput   = private.ModelOperationBindingSourceInput
	ModelOperationBindingSourceConfig  = private.ModelOperationBindingSourceConfig
	ModelOperationBindingSourceDefault = private.ModelOperationBindingSourceDefault
	ModelOperationBindingSourceOmitted = private.ModelOperationBindingSourceOmitted

	ProjectTagKey    = private.ProjectTagKey
	DefaultProjectID = private.DefaultProjectID
	DefaultSessionID = private.DefaultSessionID
)

var (
	CanonicalProviderSessionProvider      = private.CanonicalProviderSessionProvider
	FailureDecisionFromMetadata           = private.FailureDecisionFromMetadata
	CloneProviderSessionMetadata          = private.CloneProviderSessionMetadata
	CloneWorkFailureMetadata              = private.CloneWorkFailureMetadata
	CloneFailureDetail                    = private.CloneFailureDetail
	CloneWorkDiagnostics                  = private.CloneWorkDiagnostics
	CloneInvocationDiagnostic             = private.CloneInvocationDiagnostic
	PreviousChainingTraceIDs              = private.PreviousChainingTraceIDs
	PreviousChainingTraceIDsFromColors      = private.PreviousChainingTraceIDsFromColors
	CurrentChainingTraceID                  = private.CurrentChainingTraceID
	CurrentChainingTraceIDFromColors          = private.CurrentChainingTraceIDFromColors
	ChainingTraceDepthFromColors            = private.ChainingTraceDepthFromColors
	CloneWorkstationExecutionRequest        = private.CloneWorkstationExecutionRequest
	CloneProviderInferenceRequest           = private.CloneProviderInferenceRequest
	CloneSubprocessExecutionRequest         = private.CloneSubprocessExecutionRequest
	CloneResolvedModelOperationBindings     = private.CloneResolvedModelOperationBindings
	ResolveProjectID                        = private.ResolveProjectID
)

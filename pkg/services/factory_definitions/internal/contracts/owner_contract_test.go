package factorycontracts

import (
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerdiagnosticsmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerdiagnostics"
)

// Historical Factory contract tests use unqualified names so their assertions
// remain focused on the enclosing Factory snapshots. These aliases are compiled
// only into tests; production contracts expose the domain owners directly.
type (
	AgentRunToolDiagnostic          = workerexecution.AgentRunToolDiagnostic
	AgentRunTranscriptEntry         = workerexecution.AgentRunTranscriptEntry
	ExecutionMetadata               = work.ExecutionMetadata
	FactoryRelation                 = work.FactoryRelation
	FactoryWorkItem                 = work.FactoryWorkItem
	FailureDetail                   = workerexecution.FailureDetail
	InvocationArgument              = work.InvocationArgument
	InvocationArguments             = work.InvocationArguments
	InvocationArgumentSource        = work.InvocationArgumentSource
	InvocationDiagnostic            = workerexecution.InvocationDiagnostic
	InvocationParameterDiagnostic   = workerexecution.InvocationParameterDiagnostic
	ProviderSessionMetadata         = workerexecution.ProviderSessionMetadata
	ProviderDiagnostic              = workerexecution.ProviderDiagnostic
	RenderedPromptDiagnostic        = workerexecution.RenderedPromptDiagnostic
	CommandDiagnostic               = workerexecution.CommandDiagnostic
	PanicDiagnostic                 = workerexecution.PanicDiagnostic
	Relation                        = work.Relation
	RunnerBaselineCapability        = workerexecution.RunnerBaselineCapability
	RunnerOptionalCapability        = workerexecution.RunnerOptionalCapability
	RunnerOptionalCapabilitySupport = workerexecution.RunnerOptionalCapabilitySupport
	RunnerSelectionSource           = workerexecution.RunnerSelectionSource
	SafeAgentRunDiagnostic          = workerexecution.SafeAgentRunDiagnostic
	SafeProviderDiagnostic          = workerexecution.SafeProviderDiagnostic
	SafeRenderedPromptDiagnostic    = workerexecution.SafeRenderedPromptDiagnostic
	SafeWorkDiagnostics             = workerexecution.SafeWorkDiagnostics
	SubprocessExecutionRequest      = workerexecution.SubprocessExecutionRequest
	ProviderInferenceRequest        = workerexecution.ProviderInferenceRequest
	WorkstationExecutionRequest     = workerexecution.WorkstationExecutionRequest
	ResolvedModelOperationBinding   = workerexecution.ResolvedModelOperationBinding
	WorkContentPart                 = work.WorkContentPart
	WorkDiagnostics                 = workerexecution.WorkDiagnostics
	WorkDispatch                    = work.WorkDispatch
	WorkFailureMetadata             = workerexecution.WorkFailureMetadata
	WorkPayloadRef                  = work.WorkPayloadRef
	WorkPayloadLineageProjection    = work.WorkPayloadLineageProjection
	WorkResult                      = workerexecution.WorkResult
	WorkerConfig                    = workerconfig.Config
)

const (
	AgentRunExecutionBehavior                 = workerexecution.AgentRunExecutionBehavior
	AgentRunMetadataExecutionBehavior         = workerexecution.AgentRunMetadataExecutionBehavior
	AgentRunMetadataFailureClass              = workerexecution.AgentRunMetadataFailureClass
	AgentRunMetadataRecoveryAction            = workerexecution.AgentRunMetadataRecoveryAction
	AgentRunMetadataToolCallCount             = workerexecution.AgentRunMetadataToolCallCount
	AgentRunMetadataToolDiagnostics           = workerexecution.AgentRunMetadataToolDiagnostics
	AgentRunMetadataToolPolicy                = workerexecution.AgentRunMetadataToolPolicy
	AgentWorkerToolPolicyDisabled             = workerconfig.AgentToolPolicyDisabled
	AgentWorkerToolPolicyReadOnly             = workerconfig.AgentToolPolicyReadOnly
	ModelOperationBindingSourceInput          = workerexecution.ModelOperationBindingSourceInput
	OutcomeAccepted                           = workerexecution.OutcomeAccepted
	OutcomeContinue                           = workerexecution.OutcomeContinue
	OutcomeFailed                             = workerexecution.OutcomeFailed
	OutcomeRejected                           = workerexecution.OutcomeRejected
	RelationDependsOn                         = work.RelationDependsOn
	RelationParentChild                       = work.RelationParentChild
	RunnerIDCodex                             = workerexecution.RunnerIDCodex
	RunnerIDCursorCLI                         = workerexecution.RunnerIDCursorCLI
	RunnerIDGemini                            = workerexecution.RunnerIDGemini
	RunnerIDKiro                              = workerexecution.RunnerIDKiro
	RunnerIDOpenCode                          = workerexecution.RunnerIDOpenCode
	RunnerIDPi                                = workerexecution.RunnerIDPi
	RunnerOptionalCapabilityImageInput        = workerexecution.RunnerOptionalCapabilityImageInput
	RunnerOptionalCapabilitySessionResume     = workerexecution.RunnerOptionalCapabilitySessionResume
	RunnerOptionalCapabilityStructuredOutput  = workerexecution.RunnerOptionalCapabilityStructuredOutput
	RunnerOptionalCapabilityStatusSupported   = workerexecution.RunnerOptionalCapabilityStatusSupported
	RunnerOptionalCapabilityStatusUnsupported = workerexecution.RunnerOptionalCapabilityStatusUnsupported
	RunnerOptionalCapabilityWorktree          = workerexecution.RunnerOptionalCapabilityWorktree
	RunnerBaselineCapabilityPromptSubmission  = workerexecution.RunnerBaselineCapabilityPromptSubmission
	RunnerBaselineCapabilityToolExecution     = workerexecution.RunnerBaselineCapabilityToolExecution
	RunnerSelectionSourceDefault              = workerexecution.RunnerSelectionSourceDefault
	RunnerSelectionSourceFactory              = workerexecution.RunnerSelectionSourceFactory
	RunnerSelectionSourceLegacyProvider       = workerexecution.RunnerSelectionSourceLegacyProvider
	RunnerSelectionSourceWorkstation          = workerexecution.RunnerSelectionSourceWorkstation
	RunnerToolExecutionModeRequired           = workerexecution.RunnerToolExecutionModeRequired
	WorkContentPartTypeText                   = work.WorkContentPartTypeText
	WorkFailureFamilyRetryable                = workerexecution.WorkFailureFamilyRetryable
	WorkFailureFamilyTerminal                 = workerexecution.WorkFailureFamilyTerminal
	WorkFailureTypeInternalServerError        = workerexecution.WorkFailureTypeInternalServerError
	WorkFailureTypeTimeout                    = workerexecution.WorkFailureTypeTimeout
	WorkPayloadResolutionResolved             = work.WorkPayloadResolutionResolved
)

var (
	CloneProviderInferenceRequest             = workerexecution.CloneProviderInferenceRequest
	CloneProviderSessionMetadata              = workerexecution.CloneProviderSessionMetadata
	CloneSubprocessExecutionRequest           = workerexecution.CloneSubprocessExecutionRequest
	CloneWorkDispatch                         = work.CloneWorkDispatch
	CloneWorkstationExecutionRequest          = workerexecution.CloneWorkstationExecutionRequest
	CloneWorkFailureMetadata                  = workerexecution.CloneWorkFailureMetadata
	CanonicalProviderSessionProvider          = workerexecution.CanonicalProviderSessionProvider
	BuiltInRunnerMetadata                     = workerexecution.BuiltInRunnerMetadata
	GeneratedSafeWorkDiagnostics              = workerdiagnosticsmapping.GeneratedSafeWorkDiagnostics
	NewRunnerCapabilities                     = workerexecution.NewCapabilities
	ResolveRunnerSelection                    = workerexecution.ResolveRunnerSelection
	SafeAgentRunDiagnosticFromWorkDiagnostics = workerexecution.SafeAgentRunDiagnosticFromWorkDiagnostics
	SafeWorkDiagnosticsFromGenerated          = workerdiagnosticsmapping.SafeWorkDiagnosticsFromGenerated
	SafeWorkDiagnosticsFromWorkDiagnostics    = workerexecution.SafeWorkDiagnosticsFromWorkDiagnostics
	V1RunnerBaselineCapabilities              = workerexecution.V1BaselineCapabilities
	WorkDiagnosticsFromSafeWorkDiagnostics    = workerexecution.WorkDiagnosticsFromSafeWorkDiagnostics
)

package factorycontracts

import (
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/work"
	workercompatibility "github.com/portpowered/infinite-you/pkg/workers/compatibility"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerdiagnosticsmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerdiagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"
)

// Historical Factory contract tests use unqualified names so their assertions
// remain focused on the enclosing Factory snapshots. These aliases are compiled
// only into tests; production contracts expose the domain owners directly.
type (
	AgentRunToolDiagnostic          = workerdiagnostics.AgentRunToolDiagnostic
	AgentRunTranscriptEntry         = workerdiagnostics.AgentRunTranscriptEntry
	ExecutionMetadata               = work.ExecutionMetadata
	FactoryRelation                 = work.FactoryRelation
	FactoryWorkItem                 = work.FactoryWorkItem
	FailureDetail                   = workerexecution.FailureDetail
	InvocationArgument              = work.InvocationArgument
	InvocationArguments             = work.InvocationArguments
	InvocationArgumentSource        = work.InvocationArgumentSource
	InvocationDiagnostic            = workerexecution.InvocationDiagnostic
	InvocationParameterDiagnostic   = workerexecution.InvocationParameterDiagnostic
	ModelProvider                   = modelprovider.ID
	ProviderSessionMetadata         = workerexecution.ProviderSessionMetadata
	ProviderDiagnostic              = workerexecution.ProviderDiagnostic
	RenderedPromptDiagnostic        = workerexecution.RenderedPromptDiagnostic
	CommandDiagnostic               = workerexecution.CommandDiagnostic
	PanicDiagnostic                 = workerexecution.PanicDiagnostic
	Relation                        = work.Relation
	ResourceConfig                  = factoryresource.Config
	RunnerBaselineCapability        = workerexecution.RunnerBaselineCapability
	RunnerOptionalCapability        = workerexecution.RunnerOptionalCapability
	RunnerOptionalCapabilitySupport = workerexecution.RunnerOptionalCapabilitySupport
	RunnerSelectionSource           = workerexecution.RunnerSelectionSource
	SafeAgentRunDiagnostic          = workerdiagnostics.SafeAgentRunDiagnostic
	SafeProviderDiagnostic          = workerdiagnostics.SafeProviderDiagnostic
	SafeRenderedPromptDiagnostic    = workerdiagnostics.SafeRenderedPromptDiagnostic
	SafeWorkDiagnostics             = workerdiagnostics.SafeWorkDiagnostics
	SubprocessExecutionRequest      = workerexecution.SubprocessExecutionRequest
	ProviderInferenceRequest        = workerexecution.ProviderInferenceRequest
	WorkstationExecutionRequest     = workerexecution.WorkstationExecutionRequest
	ResolvedModelOperationBinding   = workerexecution.ResolvedModelOperationBinding
	Token                           = factorytoken.Token
	TokenColor                      = factorytoken.Color
	TokenHistory                    = factorytoken.History
	WorkContentPart                 = work.WorkContentPart
	WorkDiagnostics                 = workerexecution.WorkDiagnostics
	WorkDispatch                    = work.WorkDispatch
	WorkFailureMetadata             = workerexecution.WorkFailureMetadata
	FailureRecord                   = factorytoken.Failure
	WorkPayloadRef                  = work.WorkPayloadRef
	WorkPayloadLineageProjection    = work.WorkPayloadLineageProjection
	WorkResult                      = workerexecution.WorkResult
	WorkerConfig                    = workerconfig.Config
	WorkerWorkstationBehaviorClass  = workercompatibility.WorkerWorkstationBehaviorClass
)

const (
	AgentRunExecutionBehavior                 = workerdiagnostics.AgentRunExecutionBehavior
	AgentRunMetadataExecutionBehavior         = workerdiagnostics.AgentRunMetadataExecutionBehavior
	AgentRunMetadataFailureClass              = workerdiagnostics.AgentRunMetadataFailureClass
	AgentRunMetadataRecoveryAction            = workerdiagnostics.AgentRunMetadataRecoveryAction
	AgentRunMetadataToolCallCount             = workerdiagnostics.AgentRunMetadataToolCallCount
	AgentRunMetadataToolDiagnostics           = workerdiagnostics.AgentRunMetadataToolDiagnostics
	AgentRunMetadataToolPolicy                = workerdiagnostics.AgentRunMetadataToolPolicy
	AgentWorkerToolPolicyDisabled             = workerconfig.AgentToolPolicyDisabled
	AgentWorkerToolPolicyReadOnly             = workerconfig.AgentToolPolicyReadOnly
	DataTypeResource                          = factorytoken.DataTypeResource
	DataTypeWork                              = factorytoken.DataTypeWork
	ModelProviderAgy                          = modelprovider.Agy
	ModelProviderClaude                       = modelprovider.Claude
	ModelProviderCodex                        = modelprovider.Codex
	ModelProviderCursor                       = modelprovider.Cursor
	ModelProviderGemini                       = modelprovider.Gemini
	ModelProviderKiro                         = modelprovider.Kiro
	ModelProviderOpenCode                     = modelprovider.OpenCode
	ModelProviderPi                           = modelprovider.Pi
	ModelLocalityLocal                        = workerconfig.ModelLocalityLocal
	ModelOperationBindingSourceInput          = workerexecution.ModelOperationBindingSourceInput
	OutcomeAccepted                           = workerexecution.OutcomeAccepted
	OutcomeContinue                           = workerexecution.OutcomeContinue
	OutcomeFailed                             = workerexecution.OutcomeFailed
	OutcomeRejected                           = workerexecution.OutcomeRejected
	RelationDependsOn                         = work.RelationDependsOn
	RelationParentChild                       = work.RelationParentChild
	ResourceTypeModel                         = factoryresource.TypeModel
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
	WorkerWorkstationBehaviorAgent            = workercompatibility.WorkerWorkstationBehaviorAgent
	WorkerWorkstationBehaviorInference        = workercompatibility.WorkerWorkstationBehaviorInference
	WorkerWorkstationBehaviorPoller           = workercompatibility.WorkerWorkstationBehaviorPoller
	WorkerWorkstationBehaviorScript           = workercompatibility.WorkerWorkstationBehaviorScript
)

var (
	CloneProviderInferenceRequest             = workerexecution.CloneProviderInferenceRequest
	CloneProviderSessionMetadata              = workerexecution.CloneProviderSessionMetadata
	CloneSubprocessExecutionRequest           = workerexecution.CloneSubprocessExecutionRequest
	CloneToken                                = factorytoken.Clone
	CloneWorkDispatch                         = work.CloneWorkDispatch
	CloneWorkstationExecutionRequest          = workerexecution.CloneWorkstationExecutionRequest
	CloneWorkFailureMetadata                  = workerexecution.CloneWorkFailureMetadata
	CanonicalProviderSessionProvider          = workerexecution.CanonicalProviderSessionProvider
	BuiltInRunnerMetadata                     = workerrunner.BuiltInRunnerMetadata
	GeneratedSafeWorkDiagnostics              = workerdiagnosticsmapping.GeneratedSafeWorkDiagnostics
	NewRunnerCapabilities                     = workerrunner.NewCapabilities
	ProviderSessionMetadataFromGenerated      = workerdiagnosticsmapping.ProviderSessionMetadataFromGenerated
	ResolveRunnerSelection                    = workerrunner.ResolveRunnerSelection
	SafeAgentRunDiagnosticFromWorkDiagnostics = workerdiagnostics.SafeAgentRunDiagnosticFromWorkDiagnostics
	SafeWorkDiagnosticsFromGenerated          = workerdiagnosticsmapping.SafeWorkDiagnosticsFromGenerated
	SafeWorkDiagnosticsFromWorkDiagnostics    = workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics
	V1RunnerBaselineCapabilities              = workerrunner.V1BaselineCapabilities
	ClearGuardBlockingFields                  = factorytoken.ClearGuardBlockingFields
	WorkDiagnosticsFromSafeWorkDiagnostics    = workerdiagnostics.WorkDiagnosticsFromSafeWorkDiagnostics
)

func testCompatibilityWorkstation(value FactoryWorkstationConfig) workercompatibility.Workstation {
	return workercompatibility.Workstation{
		Name: value.Name, Type: value.Type, Kind: workertaxonomy.WorkstationKind(value.Kind), WorkerTypeName: value.WorkerTypeName,
	}
}

func PublicWorkerTypeForFactoryUsage(worker workerconfig.Config, values []FactoryWorkstationConfig) string {
	workstations := make([]workercompatibility.Workstation, len(values))
	for i := range values {
		workstations[i] = testCompatibilityWorkstation(values[i])
	}
	return workercompatibility.PublicWorkerTypeForFactoryUsage(worker, workstations)
}

func EffectiveWorkstationBehaviorClass(workstationType string, kind WorkstationKind, hasWorker bool) string {
	return workercompatibility.EffectiveWorkstationBehaviorClass(workstationType, workertaxonomy.WorkstationKind(kind), hasWorker)
}

func IsLegacyGrandfatheredWorkerWorkstationPair(workerType, workstationType string, kind WorkstationKind) bool {
	return workercompatibility.IsLegacyGrandfatheredWorkerWorkstationPair(workerType, workstationType, workertaxonomy.WorkstationKind(kind))
}

func RequiresWorkerWorkstationBehaviorCompatibility(workstationType string, kind WorkstationKind, workerTypeName string) bool {
	return workercompatibility.RequiresWorkerWorkstationBehaviorCompatibility(workstationType, workertaxonomy.WorkstationKind(kind), workerTypeName)
}

func CompatibleWorkerWorkstationBehavior(workerType, workstationType string, kind WorkstationKind) bool {
	return workercompatibility.CompatibleWorkerWorkstationBehavior(workerType, workstationType, workertaxonomy.WorkstationKind(kind))
}

func RuntimeBehaviorClassLabel(value string) string {
	return workercompatibility.RuntimeBehaviorClassLabel(value)
}

func WorkerWorkstationBehaviorMismatchMessage(workstationName, workstationType string, kind WorkstationKind, workerName, workerType string) string {
	return workercompatibility.WorkerWorkstationBehaviorMismatchMessage(workstationName, workstationType, workertaxonomy.WorkstationKind(kind), workerName, workerType)
}

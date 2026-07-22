package workerexecution

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

type RunnerToolExecutionMode = workers.RunnerToolExecutionMode
type RunnerBaselineCapability = workers.RunnerBaselineCapability
type RunnerOptionalCapability = workers.RunnerOptionalCapability
type RunnerOptionalCapabilityStatus = workers.RunnerOptionalCapabilityStatus
type RunnerOptionalCapabilitySupport = workers.RunnerOptionalCapabilitySupport
type RunnerCapabilities = workers.RunnerCapabilities
type RunnerMetadata = workers.RunnerMetadata
type RunnerSelectionSource = workers.RunnerSelectionSource
type ResolvedRunnerSelection = workers.ResolvedRunnerSelection
type ResolvedModelOperationBinding = workers.ResolvedModelOperationBinding
type ModelOperationBindingSource = workers.ModelOperationBindingSource
type WorkstationExecutionRequest = workers.WorkstationExecutionRequest
type ProviderInferenceRequest = workers.ProviderInferenceRequest
type RunnerExecutionRequest = workers.RunnerExecutionRequest
type RunnerExecutionResult = workers.RunnerExecutionResult
type SubprocessExecutionRequest = workers.SubprocessExecutionRequest

const (
	RunnerToolExecutionModeRequired = workers.RunnerToolExecutionModeRequired
	RunnerToolExecutionModeDisabled = workers.RunnerToolExecutionModeDisabled

	RunnerBaselineCapabilityPromptSubmission = workers.RunnerBaselineCapabilityPromptSubmission
	RunnerBaselineCapabilityToolExecution    = workers.RunnerBaselineCapabilityToolExecution

	RunnerOptionalCapabilityImageInput       = workers.RunnerOptionalCapabilityImageInput
	RunnerOptionalCapabilitySessionResume    = workers.RunnerOptionalCapabilitySessionResume
	RunnerOptionalCapabilityStructuredOutput = workers.RunnerOptionalCapabilityStructuredOutput
	RunnerOptionalCapabilityWorkingDirectory = workers.RunnerOptionalCapabilityWorkingDirectory
	RunnerOptionalCapabilityWorktree         = workers.RunnerOptionalCapabilityWorktree

	RunnerOptionalCapabilityStatusSupported   = workers.RunnerOptionalCapabilityStatusSupported
	RunnerOptionalCapabilityStatusUnsupported = workers.RunnerOptionalCapabilityStatusUnsupported

	RunnerIDCodex     = workers.RunnerIDCodex
	RunnerIDGemini    = workers.RunnerIDGemini
	RunnerIDKiro      = workers.RunnerIDKiro
	RunnerIDCursorCLI = workers.RunnerIDCursorCLI
	RunnerIDOpenCode  = workers.RunnerIDOpenCode
	RunnerIDPi        = workers.RunnerIDPi
	RunnerIDAgy       = workers.RunnerIDAgy

	RunnerSelectionSourceWorkstation    = workers.RunnerSelectionSourceWorkstation
	RunnerSelectionSourceFactory        = workers.RunnerSelectionSourceFactory
	RunnerSelectionSourceLegacyProvider = workers.RunnerSelectionSourceLegacyProvider
	RunnerSelectionSourceDefault        = workers.RunnerSelectionSourceDefault

	ModelOperationBindingSourceInput   = workers.ModelOperationBindingSourceInput
	ModelOperationBindingSourceConfig  = workers.ModelOperationBindingSourceConfig
	ModelOperationBindingSourceDefault = workers.ModelOperationBindingSourceDefault
	ModelOperationBindingSourceOmitted = workers.ModelOperationBindingSourceOmitted
)

var CloneWorkstationExecutionRequest = workers.CloneWorkstationExecutionRequest
var CloneProviderInferenceRequest = workers.CloneProviderInferenceRequest
var CloneSubprocessExecutionRequest = workers.CloneSubprocessExecutionRequest
var CloneResolvedModelOperationBindings = workers.CloneResolvedModelOperationBindings

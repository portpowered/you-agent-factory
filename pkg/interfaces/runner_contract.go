package interfaces

// RunnerExecutionRequest is the shared runner-owned execution request contract
// used by standard orchestration flows.
type RunnerExecutionRequest = ProviderInferenceRequest

// RunnerExecutionResult is the shared runner-owned execution result contract
// used by standard orchestration flows.
type RunnerExecutionResult = InferenceResponse

// RunnerSessionMetadata aliases the stable session metadata returned by runner
// executions.
type RunnerSessionMetadata = ProviderSessionMetadata

// RunnerFailureMetadata aliases the normalized failure metadata carried on work
// results after runner execution.
type RunnerFailureMetadata = ProviderFailureMetadata

// RunnerErrorType aliases the normalized runner failure type contract.
type RunnerErrorType = ProviderErrorType

// RunnerToolExecutionMode describes whether a runner invocation is expected to
// permit tool execution during the request lifecycle.
type RunnerToolExecutionMode string

const (
	// RunnerToolExecutionModeRequired means the orchestration path expects the
	// selected runner to support tool execution for this request.
	RunnerToolExecutionModeRequired RunnerToolExecutionMode = "required"
	// RunnerToolExecutionModeDisabled means the request does not require tool
	// execution support.
	RunnerToolExecutionModeDisabled RunnerToolExecutionMode = "disabled"
)

// RunnerBaselineCapability is a v1 baseline runner behavior that every built-in
// runner must support for standard orchestration participation.
type RunnerBaselineCapability string

const (
	RunnerBaselineCapabilityPromptSubmission RunnerBaselineCapability = "prompt_submission"
	RunnerBaselineCapabilityToolExecution    RunnerBaselineCapability = "tool_execution"
)

var runnerBaselineCapabilitiesV1 = []RunnerBaselineCapability{
	RunnerBaselineCapabilityPromptSubmission,
	RunnerBaselineCapabilityToolExecution,
}

// V1RunnerBaselineCapabilities returns the explicit v1 baseline capability set.
func V1RunnerBaselineCapabilities() []RunnerBaselineCapability {
	return append([]RunnerBaselineCapability(nil), runnerBaselineCapabilitiesV1...)
}

// RunnerOptionalCapability identifies an execution behavior that may be
// supported by some runners without being required for baseline participation.
type RunnerOptionalCapability string

const (
	RunnerOptionalCapabilityImageInput       RunnerOptionalCapability = "image_input"
	RunnerOptionalCapabilitySessionResume    RunnerOptionalCapability = "session_resume"
	RunnerOptionalCapabilityStructuredOutput RunnerOptionalCapability = "structured_output"
	RunnerOptionalCapabilityWorkingDirectory RunnerOptionalCapability = "working_directory"
	RunnerOptionalCapabilityWorktree         RunnerOptionalCapability = "worktree"
)

// RunnerOptionalCapabilityStatus reports whether a runner can satisfy one
// optional capability.
type RunnerOptionalCapabilityStatus string

const (
	RunnerOptionalCapabilityStatusSupported   RunnerOptionalCapabilityStatus = "supported"
	RunnerOptionalCapabilityStatusUnsupported RunnerOptionalCapabilityStatus = "unsupported"
)

// RunnerOptionalCapabilitySupport is the machine-readable capability status
// shape intended for backend, CLI, API, and UI consumption.
type RunnerOptionalCapabilitySupport struct {
	Capability RunnerOptionalCapability       `json:"capability"`
	Status     RunnerOptionalCapabilityStatus `json:"status"`
	Detail     string                         `json:"detail,omitempty"`
}

// RunnerCapabilities describes one runner's baseline and optional capability
// support in a product-surface-friendly shape.
type RunnerCapabilities struct {
	Baseline []RunnerBaselineCapability        `json:"baseline"`
	Optional []RunnerOptionalCapabilitySupport `json:"optional,omitempty"`
}

// NewRunnerCapabilities creates a capability payload with the explicit v1
// baseline and detached optional capability support entries.
func NewRunnerCapabilities(optional ...RunnerOptionalCapabilitySupport) RunnerCapabilities {
	return RunnerCapabilities{
		Baseline: V1RunnerBaselineCapabilities(),
		Optional: cloneRunnerOptionalCapabilitySupport(optional),
	}
}

// RunnerMetadata is the canonical metadata shape that product surfaces can use
// to inspect one runner's identity and capability support.
type RunnerMetadata struct {
	ID           string             `json:"id"`
	DisplayName  string             `json:"display_name,omitempty"`
	Capabilities RunnerCapabilities `json:"capabilities"`
}

// CloneRunnerExecutionRequest returns a detached copy of the shared runner
// execution request contract.
func CloneRunnerExecutionRequest(request RunnerExecutionRequest) RunnerExecutionRequest {
	return CloneProviderInferenceRequest(request)
}

func cloneRunnerOptionalCapabilitySupport(values []RunnerOptionalCapabilitySupport) []RunnerOptionalCapabilitySupport {
	if len(values) == 0 {
		return nil
	}
	clone := make([]RunnerOptionalCapabilitySupport, len(values))
	copy(clone, values)
	return clone
}

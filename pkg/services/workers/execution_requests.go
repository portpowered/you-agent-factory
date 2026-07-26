package workers

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type RunnerToolExecutionMode string

const (
	RunnerToolExecutionModeRequired RunnerToolExecutionMode = "required"
	RunnerToolExecutionModeDisabled RunnerToolExecutionMode = "disabled"
)

type RunnerBaselineCapability string

const (
	RunnerBaselineCapabilityPromptSubmission RunnerBaselineCapability = "prompt_submission"
	RunnerBaselineCapabilityToolExecution    RunnerBaselineCapability = "tool_execution"
)

type RunnerOptionalCapability string

const (
	RunnerOptionalCapabilityImageInput       RunnerOptionalCapability = "image_input"
	RunnerOptionalCapabilitySessionResume    RunnerOptionalCapability = "session_resume"
	RunnerOptionalCapabilityStructuredOutput RunnerOptionalCapability = "structured_output"
	RunnerOptionalCapabilityWorkingDirectory RunnerOptionalCapability = "working_directory"
	RunnerOptionalCapabilityWorktree         RunnerOptionalCapability = "worktree"
)

type RunnerOptionalCapabilityStatus string

const (
	RunnerOptionalCapabilityStatusSupported   RunnerOptionalCapabilityStatus = "supported"
	RunnerOptionalCapabilityStatusUnsupported RunnerOptionalCapabilityStatus = "unsupported"
)

type RunnerOptionalCapabilitySupport struct {
	Capability RunnerOptionalCapability       `json:"capability"`
	Status     RunnerOptionalCapabilityStatus `json:"status"`
	Detail     string                         `json:"detail,omitempty"`
}

type RunnerCapabilities struct {
	Baseline []RunnerBaselineCapability        `json:"baseline"`
	Optional []RunnerOptionalCapabilitySupport `json:"optional,omitempty"`
}

type RunnerMetadata struct {
	ID           string             `json:"id"`
	DisplayName  string             `json:"display_name,omitempty"`
	Capabilities RunnerCapabilities `json:"capabilities"`
}

const (
	RunnerIDCodex     = "codex"
	RunnerIDGemini    = "gemini"
	RunnerIDKiro      = "kiro"
	RunnerIDCursorCLI = "cursor-cli"
	RunnerIDOpenCode  = "opencode"
	RunnerIDPi        = "pi"
	RunnerIDAgy       = "agy"
)

type RunnerSelectionSource string

const (
	RunnerSelectionSourceWorkstation    RunnerSelectionSource = "workstation"
	RunnerSelectionSourceFactory        RunnerSelectionSource = "factory"
	RunnerSelectionSourceLegacyProvider RunnerSelectionSource = "legacy_provider"
	RunnerSelectionSourceDefault        RunnerSelectionSource = "default"
)

type ResolvedRunnerSelection struct {
	RunnerID string                `json:"runner_id,omitempty"`
	Source   RunnerSelectionSource `json:"source,omitempty"`
}

// RunnerSelectionResolver resolves configured provider precedence into the
// stable native runner contract.
type RunnerSelectionResolver func(
	workstationRunner string,
	factoryRunner string,
	workerModelProvider string,
) (ResolvedRunnerSelection, error)

// ProviderIdentityResolver validates a concrete provider identity or alias and
// returns the authoritative canonical identity.
type ProviderIdentityResolver func(identity string) (string, error)

type ResolvedModelOperationBinding struct {
	Slot    string                      `json:"slot"`
	Source  ModelOperationBindingSource `json:"source"`
	Content []work.WorkContentPart      `json:"content,omitempty"`
}

type ModelOperationBindingSource string

const (
	ModelOperationBindingSourceInput   ModelOperationBindingSource = "INPUT"
	ModelOperationBindingSourceConfig  ModelOperationBindingSource = "CONFIG"
	ModelOperationBindingSourceDefault ModelOperationBindingSource = "DEFAULT"
	ModelOperationBindingSourceOmitted ModelOperationBindingSource = "OMITTED"
)

type WorkstationExecutionRequest struct {
	Dispatch                 work.WorkDispatch               `json:"dispatch"`
	WorkerType               string                          `json:"worker_type,omitempty"`
	WorkstationType          string                          `json:"workstation_type,omitempty"`
	RunnerID                 string                          `json:"runner_id,omitempty"`
	RunnerSelectionSource    RunnerSelectionSource           `json:"runner_selection_source,omitempty"`
	ProjectID                string                          `json:"project_id,omitempty"`
	FactorySessionID         string                          `json:"factory_session_id,omitempty"`
	InputTokens              []any                           `json:"input_tokens,omitempty"`
	ModelOperation           string                          `json:"model_operation,omitempty"`
	ModelBindings            []ResolvedModelOperationBinding `json:"model_bindings,omitempty"`
	Model                    string                          `json:"model,omitempty"`
	ModelProvider            string                          `json:"model_provider,omitempty"`
	SystemPrompt             string                          `json:"system_prompt,omitempty"`
	UserMessage              string                          `json:"user_message,omitempty"`
	OutputSchema             string                          `json:"output_schema,omitempty"`
	EnvVars                  map[string]string               `json:"env_vars,omitempty"`
	ProcessEnvironment       []string                        `json:"-"`
	Worktree                 string                          `json:"worktree,omitempty"`
	WorkingDirectory         string                          `json:"working_directory,omitempty"`
	WorkingDirectoryAuthored bool                            `json:"working_directory_authored,omitempty"`
}

type ProviderInferenceRequest struct {
	Dispatch                     work.WorkDispatch               `json:"dispatch"`
	WorkerType                   string                          `json:"worker_type,omitempty"`
	WorkstationType              string                          `json:"workstation_type,omitempty"`
	RunnerID                     string                          `json:"runner_id,omitempty"`
	ProjectID                    string                          `json:"project_id,omitempty"`
	InputTokens                  []any                           `json:"input_tokens,omitempty"`
	ModelOperation               string                          `json:"model_operation,omitempty"`
	ModelBindings                []ResolvedModelOperationBinding `json:"model_bindings,omitempty"`
	SystemPrompt                 string                          `json:"system_prompt,omitempty"`
	UserMessage                  string                          `json:"user_message,omitempty"`
	OutputSchema                 string                          `json:"output_schema,omitempty"`
	ToolExecutionMode            RunnerToolExecutionMode         `json:"tool_execution_mode,omitempty"`
	RequiredOptionalCapabilities []RunnerOptionalCapability      `json:"required_optional_capabilities,omitempty"`
	EnvVars                      map[string]string               `json:"env_vars,omitempty"`
	ProcessEnvironment           []string                        `json:"-"`
	Worktree                     string                          `json:"worktree,omitempty"`
	WorkingDirectory             string                          `json:"working_directory,omitempty"`
	Model                        string                          `json:"model,omitempty"`
	ModelProvider                string                          `json:"model_provider,omitempty"`
	ModelLocality                string                          `json:"model_locality,omitempty"`
	SessionID                    string                          `json:"session_id,omitempty"`
	OpenCodeAgent                string                          `json:"open_code_agent,omitempty"`
	// SkipPermissions is the invocation-effective worker policy. Construction
	// resolves persisted configuration and invocation overrides before the
	// request reaches either the native runner or neutral conductor.
	SkipPermissions bool `json:"skip_permissions,omitempty"`
}

type RunnerExecutionRequest = ProviderInferenceRequest
type RunnerExecutionResult = InferenceResponse

type SubprocessExecutionRequest = CommandRequest

func CloneWorkstationExecutionRequest(request WorkstationExecutionRequest) WorkstationExecutionRequest {
	clone := request
	clone.Dispatch = work.CloneWorkDispatch(request.Dispatch)
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.ModelBindings = CloneResolvedModelOperationBindings(request.ModelBindings)
	clone.EnvVars = cloneStringMap(request.EnvVars)
	clone.ProcessEnvironment = append([]string(nil), request.ProcessEnvironment...)
	return clone
}

func CloneProviderInferenceRequest(request ProviderInferenceRequest) ProviderInferenceRequest {
	clone := request
	clone.Dispatch = work.CloneWorkDispatch(request.Dispatch)
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.ModelBindings = CloneResolvedModelOperationBindings(request.ModelBindings)
	clone.RequiredOptionalCapabilities = append([]RunnerOptionalCapability(nil), request.RequiredOptionalCapabilities...)
	clone.EnvVars = cloneStringMap(request.EnvVars)
	clone.ProcessEnvironment = append([]string(nil), request.ProcessEnvironment...)
	return clone
}

func CloneSubprocessExecutionRequest(request SubprocessExecutionRequest) SubprocessExecutionRequest {
	clone := request
	clone.Args = append([]string(nil), request.Args...)
	clone.Stdin = append([]byte(nil), request.Stdin...)
	clone.Env = append([]string(nil), request.Env...)
	clone.PreviousChainingTraceIDs = append([]string(nil), request.PreviousChainingTraceIDs...)
	clone.Execution = work.CloneExecutionMetadata(request.Execution)
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.InputBindings = cloneStringSliceMap(request.InputBindings)
	return clone
}

func CloneResolvedModelOperationBindings(values []ResolvedModelOperationBinding) []ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]ResolvedModelOperationBinding, len(values))
	for i, value := range values {
		cloned[i] = ResolvedModelOperationBinding{Slot: value.Slot, Source: value.Source, Content: work.CloneWorkContentParts(value.Content)}
	}
	return cloned
}

func cloneAnySlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	return append([]any(nil), values...)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(values))
	for key, items := range values {
		clone[key] = append([]string(nil), items...)
	}
	return clone
}

package workers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// ModelInvocationService is the narrow Models operation used by one managed
// inference attempt. Runtime-scoped replay can replace this effect without
// replacing the process-wide Models root or its live backend.
type ModelInvocationService interface {
	InvokeModel(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error)
}

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
	RunnerIDCodex       = "codex"
	RunnerIDClaude      = "claude"
	RunnerIDAntigravity = "antigravity"
	// Retired runner IDs remain available for persisted-data decoding only.
	RunnerIDGemini   = "gemini"
	RunnerIDKiro     = "kiro"
	RunnerIDOpenCode = "opencode"
	RunnerIDPi       = "pi"
)

type RunnerSelectionSource string

const (
	// ExecutorProviderACP is the authored execution-mechanism marker for an
	// ACP worker. The concrete Providers identity is carried separately in
	// ModelProvider and is resolved by Providers.
	ExecutorProviderACP                                       = "ACP"
	RunnerSelectionSourceWorkstation    RunnerSelectionSource = "workstation"
	RunnerSelectionSourceFactory        RunnerSelectionSource = "factory"
	RunnerSelectionSourceLegacyProvider RunnerSelectionSource = "legacy_provider"
	RunnerSelectionSourceDefault        RunnerSelectionSource = "default"
)

type ResolvedRunnerSelection struct {
	RunnerID string                `json:"runner_id,omitempty"`
	Source   RunnerSelectionSource `json:"source,omitempty"`
}

// ResolvedExecutionPolicy is the Workers-owned value boundary for one
// invocation-effective worker/workstation pair. It contains no executor,
// provider, model, process, pool, or session object; those are selected by a
// later attempt operation.
type ResolvedExecutionPolicy struct {
	WorkerName                  string                           `json:"worker_name"`
	WorkerType                  string                           `json:"worker_type,omitempty"`
	WorkstationName             string                           `json:"workstation_name"`
	WorkstationType             string                           `json:"workstation_type,omitempty"`
	RunnerID                    string                           `json:"runner_id"`
	RunnerSelectionSource       RunnerSelectionSource            `json:"runner_selection_source,omitempty"`
	Provider                    string                           `json:"provider,omitempty"`
	Model                       string                           `json:"model,omitempty"`
	ModelProvider               string                           `json:"model_provider,omitempty"`
	ModelLocality               string                           `json:"model_locality,omitempty"`
	ReasoningEffort             string                           `json:"reasoning_effort,omitempty"`
	ExecutorProvider            string                           `json:"executor_provider,omitempty"`
	Command                     string                           `json:"command,omitempty"`
	Args                        []string                         `json:"args,omitempty"`
	StopToken                   string                           `json:"stop_token,omitempty"`
	AgentToolPolicy             string                           `json:"agent_tool_policy,omitempty"`
	SkipPermissions             bool                             `json:"skip_permissions,omitempty"`
	PromptFile                  string                           `json:"prompt_file,omitempty"`
	Prompt                      string                           `json:"prompt,omitempty"`
	PromptTemplate              string                           `json:"prompt_template,omitempty"`
	OutputSchema                string                           `json:"output_schema,omitempty"`
	OutputContract              string                           `json:"output_contract,omitempty"`
	OutputFormat                string                           `json:"output_format,omitempty"`
	DecisionEnvelope            bool                             `json:"decision_envelope,omitempty"`
	GoalRoutingDecisionEnvelope bool                             `json:"goal_routing_decision_envelope,omitempty"`
	FormatInvocationSummary     bool                             `json:"format_invocation_summary,omitempty"`
	FormatInvocationResponse    bool                             `json:"format_invocation_response,omitempty"`
	FormatTTSMetadata           bool                             `json:"format_tts_metadata,omitempty"`
	Environment                 map[string]string                `json:"environment,omitempty"`
	WorkingDirectory            string                           `json:"working_directory,omitempty"`
	Worktree                    string                           `json:"worktree,omitempty"`
	Timeout                     time.Duration                    `json:"timeout,omitempty"`
	WorkPropagation             string                           `json:"work_propagation,omitempty"`
	Operation                   string                           `json:"operation,omitempty"`
	OperationBindings           []ResolvedExecutionPolicyBinding `json:"operation_bindings,omitempty"`
	StopWords                   []string                         `json:"stop_words,omitempty"`
	RuntimeStopWords            []string                         `json:"runtime_stop_words,omitempty"`
}

// ExecutionPolicy is retained as the concise Workers vocabulary for callers
// that do not need the longer resolved-value name.
type ExecutionPolicy = ResolvedExecutionPolicy

// ResolvedExecutionPolicyBinding preserves authored operation-input policy
// without exposing Factory Definitions types to Workers.
type ResolvedExecutionPolicyBinding struct {
	Slot           string                 `json:"slot"`
	SelectorSlot   string                 `json:"selector_slot,omitempty"`
	SelectorLabel  string                 `json:"selector_label,omitempty"`
	SelectorType   string                 `json:"selector_type,omitempty"`
	SelectorRole   string                 `json:"selector_role,omitempty"`
	Config         []work.WorkContentPart `json:"config,omitempty"`
	DefaultContent []work.WorkContentPart `json:"default_content,omitempty"`
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
	Dispatch              work.WorkDispatch     `json:"dispatch"`
	WorkerName            string                `json:"worker_name,omitempty"`
	WorkerType            string                `json:"worker_type,omitempty"`
	WorkstationType       string                `json:"workstation_type,omitempty"`
	RunnerID              string                `json:"runner_id,omitempty"`
	RunnerSelectionSource RunnerSelectionSource `json:"runner_selection_source,omitempty"`
	ExecutorProvider      string                `json:"executor_provider,omitempty"`
	ProjectID             string                `json:"project_id,omitempty"`
	FactorySessionID      string                `json:"factory_session_id,omitempty"`
	RuntimeID             string                `json:"runtime_id,omitempty"`
	RecordingID           string                `json:"recording_id,omitempty"`
	GenerationID          string                `json:"generation_id,omitempty"`
	// WorkflowContext carries the detached environment selected for this
	// attempt when a compatibility workstation boundary forwards the request.
	WorkflowContext             *Context                                 `json:"-"`
	Capabilities                *Capabilities                            `json:"capabilities,omitempty"`
	InputTokens                 []any                                    `json:"input_tokens,omitempty"`
	ModelOperation              string                                   `json:"model_operation,omitempty"`
	ModelBindings               []ResolvedModelOperationBinding          `json:"model_bindings,omitempty"`
	Model                       string                                   `json:"model,omitempty"`
	ModelProvider               string                                   `json:"model_provider,omitempty"`
	ReasoningEffort             string                                   `json:"reasoning_effort,omitempty"`
	Command                     string                                   `json:"command,omitempty"`
	Args                        []string                                 `json:"args,omitempty"`
	FactoryDirectory            string                                   `json:"factory_directory,omitempty"`
	OutputFormat                string                                   `json:"output_format,omitempty"`
	StopToken                   string                                   `json:"stop_token,omitempty"`
	DecisionEnvelope            bool                                     `json:"decision_envelope,omitempty"`
	GoalRoutingDecisionEnvelope bool                                     `json:"goal_routing_decision_envelope,omitempty"`
	SystemPrompt                string                                   `json:"system_prompt,omitempty"`
	UserMessage                 string                                   `json:"user_message,omitempty"`
	PromptRedaction             *PromptRedaction                         `json:"-"`
	OutputSchema                string                                   `json:"output_schema,omitempty"`
	OutputContract              string                                   `json:"output_contract,omitempty"`
	Timeout                     time.Duration                            `json:"timeout,omitempty"`
	EnvVars                     map[string]string                        `json:"env_vars,omitempty"`
	ProcessEnvironment          []string                                 `json:"-"`
	ProcessLifecycleObserver    platformprocess.ProcessLifecycleObserver `json:"-"`
	Worktree                    string                                   `json:"worktree,omitempty"`
	WorkingDirectory            string                                   `json:"working_directory,omitempty"`
	WorkingDirectoryAuthored    bool                                     `json:"working_directory_authored,omitempty"`
	// Continuation is the opaque Providers-owned continuation. Workers carries
	// it across execution boundaries without reconstructing provider-session
	// state or retaining a typed session object.
	Continuation *ProviderContinuationRef `json:"-"`
	// SkipPermissions is the invocation-effective worker policy for a Worker
	// whose caller resolved it, rather than a workstation definition. A
	// workstation-backed Worker leaves this false and takes the policy its
	// runtime was constructed with.
	SkipPermissions bool `json:"skip_permissions,omitempty"`
	// DeclaredSecretInvocationParameters is retained for detached-request
	// compatibility only. Recording must not use this work-level list to infer
	// prompt sensitivity; callers carry dispatch-specific decisions in
	// PromptRedaction instead.
	DeclaredSecretInvocationParameters []string `json:"-"`
}

type ProviderInferenceRequest struct {
	Dispatch                     work.WorkDispatch                        `json:"dispatch"`
	Correlation                  ExecutionCorrelation                     `json:"-"`
	WorkerName                   string                                   `json:"worker_name,omitempty"`
	WorkerType                   string                                   `json:"worker_type,omitempty"`
	WorkstationType              string                                   `json:"workstation_type,omitempty"`
	RunnerID                     string                                   `json:"runner_id,omitempty"`
	ExecutorProvider             string                                   `json:"executor_provider,omitempty"`
	ProjectID                    string                                   `json:"project_id,omitempty"`
	InputTokens                  []any                                    `json:"input_tokens,omitempty"`
	ModelOperation               string                                   `json:"model_operation,omitempty"`
	ModelBindings                []ResolvedModelOperationBinding          `json:"model_bindings,omitempty"`
	SystemPrompt                 string                                   `json:"system_prompt,omitempty"`
	UserMessage                  string                                   `json:"user_message,omitempty"`
	PromptRedaction              *PromptRedaction                         `json:"-"`
	OutputSchema                 string                                   `json:"output_schema,omitempty"`
	ToolExecutionMode            RunnerToolExecutionMode                  `json:"tool_execution_mode,omitempty"`
	RequiredOptionalCapabilities []RunnerOptionalCapability               `json:"required_optional_capabilities,omitempty"`
	EnvVars                      map[string]string                        `json:"env_vars,omitempty"`
	ProcessEnvironment           []string                                 `json:"-"`
	ProcessLifecycleObserver     platformprocess.ProcessLifecycleObserver `json:"-"`
	Worktree                     string                                   `json:"worktree,omitempty"`
	WorkingDirectory             string                                   `json:"working_directory,omitempty"`
	Model                        string                                   `json:"model,omitempty"`
	ModelProvider                string                                   `json:"model_provider,omitempty"`
	ReasoningEffort              string                                   `json:"reasoning_effort,omitempty"`
	Command                      string                                   `json:"command,omitempty"`
	Args                         []string                                 `json:"args,omitempty"`
	FactoryDirectory             string                                   `json:"factory_directory,omitempty"`
	OutputContract               string                                   `json:"output_contract,omitempty"`
	OutputFormat                 string                                   `json:"output_format,omitempty"`
	StopToken                    string                                   `json:"stop_token,omitempty"`
	DecisionEnvelope             bool                                     `json:"decision_envelope,omitempty"`
	GoalRoutingDecisionEnvelope  bool                                     `json:"goal_routing_decision_envelope,omitempty"`
	PrintTimeout                 time.Duration                            `json:"-"`
	ModelLocality                string                                   `json:"model_locality,omitempty"`
	// ModelRuntime is the explicit request-owned Models projection for a
	// managed inference attempt. It is intentionally excluded from provider
	// payloads and is copied through the private runner boundary.
	ModelRuntime            *ModelRuntimeInput     `json:"-"`
	ModelInvocationOverride ModelInvocationService `json:"-"`
	SessionID               string                 `json:"session_id,omitempty"`
	WorkflowContext         *Context               `json:"-"`
	// Continuation is the opaque Providers-owned continuation. Providers owns
	// decoding it and deciding whether the referenced attempt can continue.
	Continuation *ProviderContinuationRef `json:"-"`
	// SkipPermissions is the invocation-effective worker policy. Construction
	// resolves persisted configuration and invocation overrides before the
	// request reaches either the native runner or neutral conductor.
	SkipPermissions bool `json:"skip_permissions,omitempty"`
	// DeclaredSecretInvocationParameters is retained for detached-request
	// compatibility only. The provider recording decorator ignores this
	// work-level list and consumes PromptRedaction instead.
	DeclaredSecretInvocationParameters []string `json:"-"`
	// TemporaryFiles is a request-scoped effect installed by Workers Execute.
	// It is intentionally excluded from serialized provider payloads.
	TemporaryFiles TemporaryFileSystem `json:"-"`
	// ExecutionLogger is the request-scoped command log sink installed by
	// Workers Execute. Runners forward it to the Providers boundary so a
	// process-scoped command runner can write this attempt's diagnostics to
	// the opened Runtime's log. Like TemporaryFiles it is a detached
	// request-scoped effect and is excluded from serialized provider payloads.
	ExecutionLogger logging.Logger `json:"-"`
}

type RunnerExecutionRequest = ProviderInferenceRequest
type RunnerExecutionResult = InferenceResponse

func CloneWorkstationExecutionRequest(request WorkstationExecutionRequest) WorkstationExecutionRequest {
	clone := request
	clone.Dispatch = work.CloneWorkDispatch(request.Dispatch)
	clone.WorkerName = request.WorkerName
	if request.Capabilities != nil {
		capabilities := *request.Capabilities
		clone.Capabilities = &capabilities
	}
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.Args = append([]string(nil), request.Args...)
	clone.ModelBindings = CloneResolvedModelOperationBindings(request.ModelBindings)
	clone.EnvVars = cloneStringMap(request.EnvVars)
	clone.ProcessEnvironment = append([]string(nil), request.ProcessEnvironment...)
	clone.Continuation = cloneContinuation(request.Continuation)
	clone.WorkflowContext = request.WorkflowContext.Clone()
	clone.PromptRedaction = request.PromptRedaction.Clone()
	clone.DeclaredSecretInvocationParameters = append([]string(nil), request.DeclaredSecretInvocationParameters...)
	return clone
}

func CloneProviderInferenceRequest(request ProviderInferenceRequest) ProviderInferenceRequest {
	clone := request
	clone.Dispatch = work.CloneWorkDispatch(request.Dispatch)
	clone.WorkerName = request.WorkerName
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.Args = append([]string(nil), request.Args...)
	clone.ModelBindings = CloneResolvedModelOperationBindings(request.ModelBindings)
	clone.RequiredOptionalCapabilities = append([]RunnerOptionalCapability(nil), request.RequiredOptionalCapabilities...)
	clone.EnvVars = cloneStringMap(request.EnvVars)
	clone.ProcessEnvironment = append([]string(nil), request.ProcessEnvironment...)
	clone.Continuation = cloneContinuation(request.Continuation)
	clone.WorkflowContext = request.WorkflowContext.Clone()
	clone.ModelRuntime = request.ModelRuntime.Clone()
	clone.PromptRedaction = request.PromptRedaction.Clone()
	clone.DeclaredSecretInvocationParameters = append([]string(nil), request.DeclaredSecretInvocationParameters...)
	clone.TemporaryFiles = request.TemporaryFiles
	clone.ExecutionLogger = request.ExecutionLogger
	return clone
}

func cloneContinuation(reference *ProviderContinuationRef) *ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	cloned := reference.Clone()
	return &cloned
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
	clone := make([]any, len(values))
	for index, value := range values {
		clone[index] = cloneAnyValue(value)
	}
	return clone
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case []any:
		return cloneAnySlice(typed)
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, nested := range typed {
			clone[key] = cloneAnyValue(nested)
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case map[string]string:
		return cloneStringMap(typed)
	case map[string][]string:
		return cloneStringSliceMap(typed)
	default:
		return value
	}
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

var ErrInvalidExecuteRequest = errors.New("invalid Workers execute request")

var ErrExecuteUnavailable = errors.New("Workers execute capability unavailable")

// ErrExecuteCleanupFailed identifies a request-resource cleanup failure. A
// started attempt reports this as a normalized failed ExecuteResult so the
// returned result remains the sole completion authority.
var ErrExecuteCleanupFailed = errors.New("Workers execute cleanup failed")

type ExecutionOutcome string

const (
	ExecutionOutcomeAccepted ExecutionOutcome = "ACCEPTED"
	ExecutionOutcomeContinue ExecutionOutcome = "CONTINUE"
	ExecutionOutcomeRejected ExecutionOutcome = "REJECTED"
	ExecutionOutcomeFailed   ExecutionOutcome = "FAILED"
	ExecutionOutcomeCanceled ExecutionOutcome = "CANCELED"
)

// ExecuteRequest is the complete, detached input for one Workers attempt.
type ExecuteRequest struct {
	Correlation ExecutionCorrelation
	Target      ExecutionTarget
	Input       ExecutionInput
	Attempt     AttemptContext
}

type ExecutionCorrelation struct {
	FactorySessionID string
	RuntimeID        string
	GenerationID     string
	DispatchID       string
	AttemptID        string
	RequestID        string
	TraceID          string
}

type ExecutionTarget struct {
	WorkerName      string
	WorkerType      string
	WorkstationName string
	RunnerID        string
	// ExecutorProvider preserves the authored execution mechanism separately
	// from Provider, whose ID may be canonicalized to a concrete catalog ID.
	ExecutorProvider string
	// Noop accepts a worker dispatch without invoking a provider, model, or
	// command runner. Runtime sets this only for an authored worker that has no
	// runtime definition, preserving topology-only factories without retaining
	// session-specific executor state in Workers.
	Noop             bool
	Capabilities     *Capabilities
	Command          string
	Args             []string
	FactoryDirectory string
	Provider         ProviderReference
	Model            ModelReference
	Prompt           PromptPolicy
	Tools            ToolPolicy
	Output           OutputPolicy
	Environment      EnvironmentPolicy
	Workspace        WorkspacePolicy
	Permissions      PermissionPolicy
	Timeout          time.Duration
}

type ProviderReference struct {
	ID    string
	Alias string
}

type ModelReference struct {
	Name            string
	Provider        string
	ReasoningEffort string
	Locality        string
}

type PromptPolicy struct {
	SystemPrompt string
	UserMessage  string
	OutputSchema string
	// Redaction is an in-memory, recording-only projection of prompt
	// provenance. Workers executes the real prompt fields above; recording
	// decorators use this explicit safe projection when a declared-sensitive
	// invocation binding contributed to either field.
	Redaction *PromptRedaction
}

// PromptRedaction carries safe prompt text produced from explicit invocation
// interpolation provenance. It never carries the original sensitive value.
// When FailClosed is true, a recorder must redact the affected complete field
// instead of attempting a partial transformation.
type PromptRedaction struct {
	SystemPrompt       string
	UserMessage        string
	RedactSystemPrompt bool
	RedactUserMessage  bool
	FailClosed         bool
}

func (redaction *PromptRedaction) Clone() *PromptRedaction {
	if redaction == nil {
		return nil
	}
	clone := *redaction
	return &clone
}

type ToolPolicy struct {
	ExecutionMode RunnerToolExecutionMode
	// AgentLoop runs the attempt through the Workers agent-run harness instead
	// of one provider attempt. Runtime owns the Factory definitions that decide
	// it -- an AGENT_RUN workstation staffed by an agent worker -- and carries
	// the decision in the detached request so Workers never reads a definition
	// to route.
	AgentLoop bool
	// AgentToolPolicy is the authored agent tool policy the harness applies
	// when AgentLoop is set. An empty value disables tool execution.
	AgentToolPolicy              string
	RequiredOptionalCapabilities []RunnerOptionalCapability
}

type OutputPolicy struct {
	Contract                    string
	Format                      string
	StopToken                   string
	DecisionEnvelope            bool
	GoalRoutingDecisionEnvelope bool
	// Classifier marks a classifier workstation dispatch. The Runtime-owned
	// dispatch adapter validates the produced label before route matching so a
	// malformed classifier output fails distinctly instead of reaching route
	// matching as an unmatched label.
	Classifier bool
	// ScriptClassifier asks the Runtime-owned dispatch adapter to reduce a
	// script classifier's stdout to its final label before route matching.
	ScriptClassifier bool
}

type EnvironmentPolicy struct {
	Vars                   map[string]string
	ProcessEnvironment     []string
	WorkingDirectory       string
	WorkingDirectorySet    bool
	SkipProcessInheritance bool
}

type WorkspacePolicy struct {
	Worktree         string
	WorkingDirectory string
	PrepareWorktree  bool
	// RetainWorktree leaves a runtime-owned prepared checkout in place after
	// the attempt. Stateless callers keep the default cleanup behavior; the
	// Factory Runtime compatibility path owns its checkout beyond one attempt.
	RetainWorktree     bool `json:"-"`
	FactoryDirectory   string
	CheckoutIdentifier string
}

type PermissionPolicy struct {
	SkipPermissions bool
}

type ExecutionInput struct {
	Work []WorkInput
	// Dispatch preserves detached routing and replay facts that the Runtime
	// must carry through an execution attempt without exposing executor state.
	Dispatch work.WorkDispatch
	// RecordingID remains the optional Worker Sessions recording identity. It
	// is distinct from Correlation.RuntimeID, which identifies the live Runtime.
	RecordingID    string
	Invocation     work.InvocationArguments
	ModelBindings  []ResolvedModelOperationBinding
	ModelOperation string
	// ModelRuntime carries the opened Models scope and the detached local
	// worker/resource projection for a managed inference attempt. It is an
	// explicit request input, not a context side channel or a second Execute
	// operation.
	ModelRuntime     *ModelRuntimeInput
	PreviousAttempts []AttemptSummary
	Resume           *ProviderContinuationRef
	// WorkflowContext is the complete detached context selected for this
	// attempt. Workers never recovers it from a Factory Session or Runtime.
	WorkflowContext *Context
	// MockWorkers carries an optional request-scoped testing override. It is
	// detached at Execute ingress and is consumed only by command-boundary
	// adapters; it is never stored on the process Workers root.
	MockWorkers *MockWorkersConfig
	// SkipBuiltInPrerequisiteValidation carries the Factory Runtime opening
	// policy into the request-scoped execution owner. It is deliberately an
	// execution input rather than process-scoped Workers state.
	SkipBuiltInPrerequisiteValidation bool `json:"-"`
	// InvocationSkipPermissionsOverride carries the invocation-scoped
	// permission policy selected while opening the Factory Runtime.
	InvocationSkipPermissionsOverride *bool `json:"-"`
	// ProviderOverride and CommandRunnerOverride carry runtime-scoped effect
	// ports for detached execution, such as Recordings replay. They are never
	// serialized or retained by the process-scoped Workers service.
	ProviderOverride      providers.Service             `json:"-"`
	CommandRunnerOverride platformprocess.CommandRunner `json:"-"`
	// ModelInvocationOverride carries the runtime-scoped managed-model effect
	// used by deterministic replay. It is request-scoped so replay never
	// replaces the process-wide Models service or its live backend.
	ModelInvocationOverride ModelInvocationService `json:"-"`
	// PreparedRequestObserver receives the detached request after Workers has
	// prepared request-scoped resources and before the runner starts. Runtime
	// uses it to record the effective execution target without moving resource
	// preparation into the runtime boundary.
	PreparedRequestObserver func(ExecuteRequest) `json:"-"`
	// ProcessLifecycleObserver carries the dispatch-owned process lifecycle
	// effect through the Runtime-to-Workers operation boundary. It is never
	// serialized or retained by the process-scoped Workers service.
	ProcessLifecycleObserver platformprocess.ProcessLifecycleObserver `json:"-"`
	// ProgressPublisher carries the Runtime-selected observation sink for this
	// attempt. It is an execution capability, not retained Workers state.
	// ExecutionLogger carries the Runtime-selected structured log sink for this
	// attempt. It is intentionally detached from process-scoped Workers state.
	ExecutionLogger     logging.Logger      `json:"-"`
	ProgressPublisher   ProgressPublisher   `json:"-"`
	ScriptEventRecorder ScriptEventRecorder `json:"-"`
	// InferenceEventRecorder receives the canonical provider request/response
	// facts for this detached attempt. It is a request-scoped capability and is
	// never retained by the process-scoped Workers service.
	InferenceEventRecorder InferenceEventRecorder `json:"-"`
}

// ModelRuntimeInput is the request-owned Models projection consumed by the
// private inference runner for one managed-model attempt.
type ModelRuntimeInput struct {
	Scope     modelinference.RuntimeScopeRef
	Worker    modelinference.LocalWorker
	Resources []modelinference.LocalResource
}

func (input *ModelRuntimeInput) Clone() *ModelRuntimeInput {
	if input == nil {
		return nil
	}
	clone := *input
	clone.Worker.Resources = append([]modelinference.LocalResource(nil), input.Worker.Resources...)
	clone.Resources = append([]modelinference.LocalResource(nil), input.Resources...)
	return &clone
}

type WorkInput struct {
	// Kind distinguishes a Work input from a capacity/resource input without
	// exposing the runtime representation that supplied it.
	Kind         string
	State        string
	InputNames   []string
	WorkID       string
	Name         string
	WorkTypeID   string
	RequestID    string
	Content      []work.WorkContentPart
	Tags         map[string]string
	Relations    []work.Relation
	Lineage      WorkLineage
	AttemptFacts AttemptFacts
}

type WorkLineage struct {
	ParentWorkID string
	TraceID      string
	OriginRef    string
}

type AttemptFacts struct {
	AttemptNumber int
	LastOutcome   string
	LastFailure   string
}

type AttemptContext struct {
	Number int
}

type AttemptSummary struct {
	AttemptID string
	Outcome   ExecutionOutcome
	Failure   *ExecutionFailure
	Finished  time.Time
}

// ProviderContinuationRef is retained as the Workers compatibility name for
// the Providers-owned opaque continuation value.
type ProviderContinuationRef = providers.ContinuationRef

type ExecuteResult struct {
	Correlation  ExecutionCorrelation
	Outcome      ExecutionOutcome
	Cancellation *DispatchCancellation
	Output       ProposedOutput
	// ProposedOutputPresent distinguishes a runner-owned detached proposal
	// from the compatibility text projection synthesized from legacy runner
	// content. Runtime must only materialize the former as structured content;
	// the latter may contain a serialized Work payload that its legacy parser
	// still needs to decode.
	ProposedOutputPresent   bool `json:"-"`
	StructuredResult        any
	StructuredResultPresent bool
	ArtifactVerification    *ExpectedArtifactVerification
	Failure                 *ExecutionFailure
	Diagnostics             *SafeDiagnostics
	Metrics                 ExecutionMetrics
	Continuation            *ProviderContinuationRef
}

type ExecutionFailure struct {
	Type                            WorkFailureType
	Family                          WorkFailureFamily
	Message                         string
	RetryHint                       bool
	Detail                          *FailureDetail
	ProviderFailureKind             providers.ExecuteFailureKind
	ProviderContinuationFailureKind providers.ContinuationFailureKind
	ProviderContinuationOutcome     providers.ContinuationOutcome
}

type ExecutionMetrics struct {
	Duration   time.Duration
	Cost       float64
	RetryCount int
}

func (request ExecuteRequest) Validate() error {
	if err := request.Correlation.Validate(); err != nil {
		return err
	}
	if err := validateDetachedDispatch(request); err != nil {
		return err
	}
	if err := validateWorkflowContext(request); err != nil {
		return err
	}
	if err := validateExecutionTarget(request.Target); err != nil {
		return err
	}
	return validateResumeContinuation(request.Input.Resume)
}

func validateDetachedDispatch(request ExecuteRequest) error {
	if dispatchID := strings.TrimSpace(request.Input.Dispatch.DispatchID); dispatchID != "" &&
		dispatchID != strings.TrimSpace(request.Correlation.DispatchID) {
		return fmt.Errorf("%w: dispatch identity conflicts with detached dispatch", ErrInvalidExecuteRequest)
	}
	if request.Input.Dispatch.Execution.RequestID != "" &&
		strings.TrimSpace(request.Correlation.RequestID) != "" &&
		strings.TrimSpace(request.Input.Dispatch.Execution.RequestID) != strings.TrimSpace(request.Correlation.RequestID) {
		return fmt.Errorf("%w: request identity conflicts with detached dispatch", ErrInvalidExecuteRequest)
	}
	if request.Input.Dispatch.Execution.TraceID != "" &&
		strings.TrimSpace(request.Correlation.TraceID) != "" &&
		strings.TrimSpace(request.Input.Dispatch.Execution.TraceID) != strings.TrimSpace(request.Correlation.TraceID) {
		return fmt.Errorf("%w: trace identity conflicts with detached dispatch", ErrInvalidExecuteRequest)
	}
	return nil
}

func validateWorkflowContext(request ExecuteRequest) error {
	context := request.Input.WorkflowContext
	if context == nil || strings.TrimSpace(context.SessionID) == "" ||
		strings.TrimSpace(context.SessionID) == strings.TrimSpace(request.Correlation.FactorySessionID) {
		return nil
	}
	return fmt.Errorf("%w: workflow context session identity conflicts with correlation", ErrInvalidExecuteRequest)
}

func validateExecutionTarget(target ExecutionTarget) error {
	if strings.TrimSpace(target.RunnerID) == "" &&
		strings.TrimSpace(target.Provider.ID) == "" &&
		strings.TrimSpace(target.Provider.Alias) == "" &&
		strings.TrimSpace(target.Model.Name) == "" {
		return fmt.Errorf("%w: runner, provider, or model target is required", ErrInvalidExecuteRequest)
	}
	if target.Timeout < 0 {
		return fmt.Errorf("%w: timeout must not be negative", ErrInvalidExecuteRequest)
	}
	return nil
}

func validateResumeContinuation(resume *ProviderContinuationRef) error {
	if resume == nil {
		return nil
	}
	if err := resume.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecuteRequest, err)
	}
	return nil
}

// Validate checks the complete identity required to attribute one detached
// attempt to the Factory Session and Runtime that admitted it.
func (correlation ExecutionCorrelation) Validate() error {
	required := []struct {
		name  string
		value string
	}{
		{name: "factory session id", value: correlation.FactorySessionID},
		{name: "runtime id", value: correlation.RuntimeID},
		{name: "generation id", value: correlation.GenerationID},
		{name: "dispatch id", value: correlation.DispatchID},
		{name: "attempt id", value: correlation.AttemptID},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidExecuteRequest, item.name)
		}
	}
	return nil
}

func (request ExecuteRequest) Clone() ExecuteRequest {
	clone := request
	clone.Target = request.Target.Clone()
	clone.Input = request.Input.Clone()
	return clone
}

func (target ExecutionTarget) Clone() ExecutionTarget {
	clone := target
	if target.Capabilities != nil {
		capabilities := *target.Capabilities
		clone.Capabilities = &capabilities
	}
	clone.Args = append([]string(nil), target.Args...)
	clone.Tools.RequiredOptionalCapabilities = append(
		[]RunnerOptionalCapability(nil),
		target.Tools.RequiredOptionalCapabilities...,
	)
	clone.Environment.Vars = cloneStringMap(target.Environment.Vars)
	clone.Environment.ProcessEnvironment = append(
		[]string(nil),
		target.Environment.ProcessEnvironment...,
	)
	clone.Prompt.Redaction = target.Prompt.Redaction.Clone()
	return clone
}

func (input ExecutionInput) Clone() ExecutionInput {
	clone := input
	clone.Dispatch = work.CloneWorkDispatch(input.Dispatch)
	if args := work.CloneInvocationArguments(&input.Invocation); args != nil {
		clone.Invocation = *args
	}
	clone.ModelBindings = CloneResolvedModelOperationBindings(input.ModelBindings)
	clone.ModelRuntime = input.ModelRuntime.Clone()
	if len(input.Work) > 0 {
		clone.Work = make([]WorkInput, len(input.Work))
		for i, item := range input.Work {
			clone.Work[i] = item.Clone()
		}
	}
	if len(input.PreviousAttempts) > 0 {
		clone.PreviousAttempts = make([]AttemptSummary, len(input.PreviousAttempts))
		for i, summary := range input.PreviousAttempts {
			clone.PreviousAttempts[i] = summary.Clone()
		}
	}
	if input.Resume != nil {
		clone.Resume = cloneContinuation(input.Resume)
	}
	clone.WorkflowContext = input.WorkflowContext.Clone()
	clone.MockWorkers = input.MockWorkers.Clone()
	if input.InvocationSkipPermissionsOverride != nil {
		value := *input.InvocationSkipPermissionsOverride
		clone.InvocationSkipPermissionsOverride = &value
	}
	return clone
}

func (input WorkInput) Clone() WorkInput {
	clone := input
	clone.InputNames = append([]string(nil), input.InputNames...)
	clone.Content = work.CloneWorkContentParts(input.Content)
	clone.Tags = cloneStringMap(input.Tags)
	clone.Relations = append([]work.Relation(nil), input.Relations...)
	return clone
}

func (summary AttemptSummary) Clone() AttemptSummary {
	clone := summary
	if summary.Failure != nil {
		failure := summary.Failure.Clone()
		clone.Failure = &failure
	}
	return clone
}

func (failure ExecutionFailure) Clone() ExecutionFailure {
	clone := failure
	clone.Detail = CloneFailureDetail(failure.Detail)
	return clone
}

func (result ExecuteResult) Clone() ExecuteResult {
	clone := result
	clone.Cancellation = result.Cancellation.Clone()
	clone.Output = result.Output.Clone()
	clone.StructuredResult = jsonvalue.Clone(result.StructuredResult)
	clone.StructuredResultPresent = jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent)
	clone.ArtifactVerification = result.ArtifactVerification.Clone()
	if result.Failure != nil {
		failure := result.Failure.Clone()
		clone.Failure = &failure
	}
	clone.Diagnostics = cloneSafeDiagnostics(result.Diagnostics)
	if result.Continuation != nil {
		continuation := *result.Continuation
		clone.Continuation = &continuation
	}
	return clone
}

func cloneSafeDiagnostics(diagnostics *SafeDiagnostics) *SafeDiagnostics {
	if diagnostics == nil {
		return nil
	}
	clone := &SafeDiagnostics{
		RenderedPrompt: cloneSafeRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneSafeProviderDiagnostic(diagnostics.Provider),
		AgentRun:       cloneSafeAgentRunDiagnostic(diagnostics.AgentRun),
		Invocation:     CloneInvocationDiagnostic(diagnostics.Invocation),
		Metadata:       cloneStringMap(diagnostics.Metadata),
	}
	if diagnostics.Command != nil {
		clone.Command = &SafeCommandDiagnostic{
			Command:    diagnostics.Command.Command,
			Args:       append([]string(nil), diagnostics.Command.Args...),
			Stdout:     diagnostics.Command.Stdout,
			Stderr:     diagnostics.Command.Stderr,
			ExitCode:   diagnostics.Command.ExitCode,
			TimedOut:   diagnostics.Command.TimedOut,
			Duration:   diagnostics.Command.Duration,
			WorkingDir: diagnostics.Command.WorkingDir,
		}
	}
	if diagnostics.Panic != nil {
		clone.Panic = &PanicDiagnostic{
			Message: diagnostics.Panic.Message,
			Stack:   diagnostics.Panic.Stack,
		}
	}
	return clone
}

func cloneSafeRenderedPromptDiagnostic(
	diagnostic *SafeRenderedPromptDiagnostic,
) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func cloneSafeProviderDiagnostic(diagnostic *SafeProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func cloneSafeAgentRunDiagnostic(diagnostic *SafeAgentRunDiagnostic) *SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	clone := &SafeAgentRunDiagnostic{
		ExecutionBehavior: diagnostic.ExecutionBehavior,
		FailureClass:      diagnostic.FailureClass,
		RecoveryAction:    diagnostic.RecoveryAction,
		ToolPolicy:        diagnostic.ToolPolicy,
		ToolCallCount:     diagnostic.ToolCallCount,
	}
	clone.ToolDiagnostics = append([]AgentRunToolDiagnostic(nil), diagnostic.ToolDiagnostics...)
	clone.Transcript = append([]AgentRunTranscriptEntry(nil), diagnostic.Transcript...)
	return clone
}

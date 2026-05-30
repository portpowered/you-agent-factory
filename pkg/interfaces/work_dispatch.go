package interfaces

import (
	"time"
)

// WorkDispatch is the canonical dispatch-owned runtime payload.
//
// Dispatcher construction owns these fields. Worker execution must derive any
// prompt, environment, path, model, provider, or session state into the
// dedicated boundary request types below instead of mutating WorkDispatch.
type WorkDispatch struct {
	DispatchID               string              `json:"dispatch_id"`
	TransitionID             string              `json:"transition_id"`
	WorkerType               string              `json:"worker_type,omitempty"`
	WorkstationName          string              `json:"workstation_name,omitempty"`
	ProjectID                string              `json:"project_id,omitempty"`
	CurrentChainingTraceID   string              `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string            `json:"previous_chaining_trace_ids,omitempty"`
	Execution                ExecutionMetadata   `json:"execution,omitempty"`
	InputTokens              []any               `json:"input_tokens"`
	InputBindings            map[string][]string `json:"input_bindings,omitempty"`
}

// ExecutionMetadata carries replay matching and logical tick context from the
// dispatch runtime.
type ExecutionMetadata struct {
	DispatchCreatedTick int      `json:"dispatch_created_tick,omitempty"`
	CurrentTick         int      `json:"current_tick,omitempty"`
	RequestID           string   `json:"request_id,omitempty"`
	TraceID             string   `json:"trace_id,omitempty"`
	WorkIDs             []string `json:"work_ids,omitempty"`
	ReplayKey           string   `json:"replay_key,omitempty"`
}

// WorkstationExecutionRequest is the worker-owned request assembled after
// workstation rendering. It combines the canonical dispatch identity with the
// resolved prompt, ordered inputs, runtime context, and worker selection needed
// by inner executors.
type WorkstationExecutionRequest struct {
	Dispatch              WorkDispatch                    `json:"dispatch"`
	WorkerType            string                          `json:"worker_type,omitempty"`
	WorkstationType       string                          `json:"workstation_type,omitempty"`
	RunnerID              string                          `json:"runner_id,omitempty"`
	RunnerSelectionSource RunnerSelectionSource           `json:"runner_selection_source,omitempty"`
	ProjectID             string                          `json:"project_id,omitempty"`
	InputTokens           []any                           `json:"input_tokens,omitempty"`
	ModelOperation        string                          `json:"model_operation,omitempty"`
	ModelBindings         []ResolvedModelOperationBinding `json:"model_bindings,omitempty"`
	SystemPrompt          string                          `json:"system_prompt,omitempty"`
	UserMessage           string                          `json:"user_message,omitempty"`
	OutputSchema          string                          `json:"output_schema,omitempty"`
	EnvVars               map[string]string               `json:"env_vars,omitempty"`
	Worktree              string                          `json:"worktree,omitempty"`
	WorkingDirectory      string                          `json:"working_directory,omitempty"`
}

// ProviderInferenceRequest is the provider-owned request shape derived from one
// rendered workstation execution request plus runtime worker configuration.
type ProviderInferenceRequest struct {
	Dispatch                     WorkDispatch                    `json:"dispatch"`
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
	Worktree                     string                          `json:"worktree,omitempty"`
	WorkingDirectory             string                          `json:"working_directory,omitempty"`
	Model                        string                          `json:"model,omitempty"`
	ModelProvider                string                          `json:"model_provider,omitempty"`
	ModelLocality                string                          `json:"model_locality,omitempty"`
	SessionID                    string                          `json:"session_id,omitempty"`
	OpenCodeAgent                string                          `json:"open_code_agent,omitempty"`
}

// SubprocessExecutionRequest is the command-boundary request derived from
// workstation or provider execution. It carries only subprocess-owned command
// fields plus the dispatch correlation and input context needed at that seam.
type SubprocessExecutionRequest struct {
	Command                  string              `json:"command"`
	Args                     []string            `json:"args,omitempty"`
	Stdin                    []byte              `json:"stdin,omitempty"`
	Env                      []string            `json:"env,omitempty"`
	WorkDir                  string              `json:"work_dir,omitempty"`
	DispatchID               string              `json:"dispatch_id,omitempty"`
	TransitionID             string              `json:"transition_id,omitempty"`
	WorkerType               string              `json:"worker_type,omitempty"`
	WorkstationName          string              `json:"workstation_name,omitempty"`
	ProjectID                string              `json:"project_id,omitempty"`
	CurrentChainingTraceID   string              `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string            `json:"previous_chaining_trace_ids,omitempty"`
	Execution                ExecutionMetadata   `json:"execution,omitempty"`
	InputTokens              []any               `json:"input_tokens,omitempty"`
	InputBindings            map[string][]string `json:"input_bindings,omitempty"`
}

// CloneExecutionMetadata returns a detached copy of canonical dispatch
// execution metadata.
func CloneExecutionMetadata(metadata ExecutionMetadata) ExecutionMetadata {
	clone := metadata
	clone.WorkIDs = cloneStringSlice(metadata.WorkIDs)
	return clone
}

// CloneWorkDispatch returns a detached copy of the canonical dispatch-owned
// contract.
func CloneWorkDispatch(dispatch WorkDispatch) WorkDispatch {
	clone := dispatch
	clone.PreviousChainingTraceIDs = cloneStringSlice(dispatch.PreviousChainingTraceIDs)
	clone.Execution = CloneExecutionMetadata(dispatch.Execution)
	clone.InputTokens = cloneAnySlice(dispatch.InputTokens)
	clone.InputBindings = cloneStringSliceMap(dispatch.InputBindings)
	return clone
}

// CloneWorkstationExecutionRequest returns a detached copy of the rendered
// workstation boundary request.
func CloneWorkstationExecutionRequest(request WorkstationExecutionRequest) WorkstationExecutionRequest {
	clone := request
	clone.Dispatch = CloneWorkDispatch(request.Dispatch)
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.ModelBindings = CloneResolvedModelOperationBindings(request.ModelBindings)
	clone.EnvVars = cloneStringMap(request.EnvVars)
	return clone
}

// CloneProviderInferenceRequest returns a detached copy of the provider-owned
// inference request.
func CloneProviderInferenceRequest(request ProviderInferenceRequest) ProviderInferenceRequest {
	clone := request
	clone.Dispatch = CloneWorkDispatch(request.Dispatch)
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.ModelBindings = CloneResolvedModelOperationBindings(request.ModelBindings)
	clone.RequiredOptionalCapabilities = cloneRunnerOptionalCapabilities(request.RequiredOptionalCapabilities)
	clone.EnvVars = cloneStringMap(request.EnvVars)
	return clone
}

// CloneSubprocessExecutionRequest returns a detached copy of the subprocess
// boundary request.
func CloneSubprocessExecutionRequest(request SubprocessExecutionRequest) SubprocessExecutionRequest {
	clone := request
	clone.Args = cloneStringSlice(request.Args)
	clone.Stdin = cloneByteSlice(request.Stdin)
	clone.Env = cloneStringSlice(request.Env)
	clone.PreviousChainingTraceIDs = cloneStringSlice(request.PreviousChainingTraceIDs)
	clone.Execution = CloneExecutionMetadata(request.Execution)
	clone.InputTokens = cloneAnySlice(request.InputTokens)
	clone.InputBindings = cloneStringSliceMap(request.InputBindings)
	return clone
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(values))
	for key, items := range values {
		clone[key] = cloneStringSlice(items)
	}
	return clone
}

func cloneAnySlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	clone := make([]any, len(values))
	copy(clone, values)
	return clone
}

func cloneByteSlice(values []byte) []byte {
	if len(values) == 0 {
		return nil
	}
	clone := make([]byte, len(values))
	copy(clone, values)
	return clone
}

func cloneRunnerOptionalCapabilities(values []RunnerOptionalCapability) []RunnerOptionalCapability {
	if len(values) == 0 {
		return nil
	}
	clone := make([]RunnerOptionalCapability, len(values))
	copy(clone, values)
	return clone
}

// CloneResolvedModelOperationBindings returns a detached copy of one resolved
// model-operation binding slice.
func CloneResolvedModelOperationBindings(values []ResolvedModelOperationBinding) []ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]ResolvedModelOperationBinding, len(values))
	for i, value := range values {
		cloned[i] = ResolvedModelOperationBinding{
			Slot:    value.Slot,
			Source:  value.Source,
			Content: cloneWorkContentParts(value.Content),
		}
	}
	return cloned
}

// DataType distinguishes resource tokens from work tokens.
type DataType string

const (
	DataTypeResource DataType = "resource"
	DataTypeWork     DataType = "work"
)

// TokenColor carries the domain data attached to a token.
type TokenColor struct {
	Name                     string            `json:"name"`
	RequestID                string            `json:"request_id"`
	WorkID                   string            `json:"work_id"`
	WorkTypeID               string            `json:"work_type_id"`
	DataType                 DataType          `json:"data_type"`
	ChainingTraceDepth       int               `json:"chaining_trace_depth,omitempty"`
	CurrentChainingTraceID   string            `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previous_chaining_trace_ids,omitempty"`
	TraceID                  string            `json:"trace_id"`
	ParentID                 string            `json:"parent_id"`
	Tags                     map[string]string `json:"tags"`
	Relations                []Relation        `json:"relations"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  []byte            `json:"payload"`
}

// Token is a colored token: a work item or resource with data flowing through
// the net.
type Token struct {
	ID        string       `json:"id"`
	PlaceID   string       `json:"place_id"`
	Color     TokenColor   `json:"color"`
	CreatedAt time.Time    `json:"created_at"`
	EnteredAt time.Time    `json:"entered_at"`
	History   TokenHistory `json:"history"`
}

// TokenHistory tracks a token's journey through the net.
type TokenHistory struct {
	TotalVisits         map[string]int  `json:"total_visits"`
	ConsecutiveFailures map[string]int  `json:"consecutive_failures"`
	PlaceVisits         map[string]int  `json:"place_visits"`
	TotalDuration       time.Duration   `json:"total_duration"`
	LastError           string          `json:"last_error"`
	FailureLog          []FailureRecord `json:"failure_log"`
}

// FailureRecord captures a single failure event for a token.
type FailureRecord struct {
	TransitionID string    `json:"transition_id"`
	Timestamp    time.Time `json:"timestamp"`
	Error        string    `json:"error"`
	Attempt      int       `json:"attempt"`
}

// ClearGuardBlockingFields resets visit and failure-count guard counters while
// preserving customer-visible failure history during operator recovery moves.
func ClearGuardBlockingFields(history *TokenHistory) {
	if history == nil {
		return
	}
	history.TotalVisits = make(map[string]int)
	history.ConsecutiveFailures = make(map[string]int)
	history.PlaceVisits = make(map[string]int)
}

// WorkStateChangeSource identifies who initiated a WORK_STATE_CHANGE event.
type WorkStateChangeSource string

const (
	WorkStateChangeSourceAPI              WorkStateChangeSource = "api"
	WorkStateChangeSourceCLI              WorkStateChangeSource = "cli"
	WorkStateChangeSourceCascadingFailure WorkStateChangeSource = "cascading-failure"
)

// WorkStateChangeRecord captures one operator or cascade marking relocation for
// canonical event history emission.
type WorkStateChangeRecord struct {
	WorkID        string
	WorkTypeID    string
	WorkTypeName  string
	FromState     string
	ToState       string
	FromPlaceID   string
	ToPlaceID     string
	Source        WorkStateChangeSource
	RequestID     string
	TriggerWorkID string
	Reason        string
}

// OperatorMoveResult is the engine outcome of a successful manual work relocation.
type OperatorMoveResult struct {
	WorkID      string
	WorkTypeID  string
	FromState   string
	ToState     string
	FromPlaceID string
	ToPlaceID   string
	TokenID     string
}

// Relation defines a typed relationship between work items.
type Relation struct {
	Type          RelationType `json:"type"`
	TargetWorkID  string       `json:"target_work_id"`
	RequiredState string       `json:"required_state,omitempty"`
}

// RelationType classifies the relationship between two work items.
type RelationType string

const (
	RelationDependsOn   RelationType = "DEPENDS_ON"
	RelationParentChild RelationType = "PARENT_CHILD"
	RelationSpawnedBy   RelationType = "SPAWNED_BY"
)

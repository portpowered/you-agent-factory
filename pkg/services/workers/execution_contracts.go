package workers

import (
	"encoding/json"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// WorkerState is a point-in-time snapshot of the dispatcher's state.
type WorkerState struct {
	// ID is a unique identifier for this snapshot.
	ID string
	// WorkDispatchIDs lists the IDs of currently in-flight dispatches.
	WorkDispatchIDs []string
	// StartedAt is when the dispatcher was created.
	StartedAt time.Time
}

// InferenceResponse is returned by a provider after model inference.
type InferenceResponse struct {
	Content         string                   `json:"content"`
	ProviderSession *ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics     *WorkDiagnostics         `json:"diagnostics,omitempty"`
}

// InferenceEventKind identifies which provider-boundary fact was observed.
// Factory owns the corresponding canonical event vocabulary and envelope.
type InferenceEventKind string

const (
	InferenceEventKindRequest  InferenceEventKind = "REQUEST"
	InferenceEventKindResponse InferenceEventKind = "RESPONSE"
)

// InferenceEvent carries provider-owned attempt facts to Factory history
// without coupling provider execution to a transport or Factory envelope.
type InferenceEvent struct {
	ID         string
	Kind       InferenceEventKind
	EventTime  time.Time
	Tick       int
	DispatchID string
	RequestID  string
	TraceIDs   []string
	WorkIDs    []string
	Request    *InferenceRequestEventPayload
	Response   *InferenceResponseEventPayload
}

// InferenceRequestEventPayload records the concrete provider request boundary.
type InferenceRequestEventPayload struct {
	Attempt            int    `json:"attempt"`
	InferenceRequestID string `json:"inferenceRequestId"`
	Prompt             string `json:"prompt"`
	WorkingDirectory   string `json:"workingDirectory"`
	Worktree           string `json:"worktree"`
}

// InferenceOutcome is the stable provider-attempt result category.
type InferenceOutcome string

const (
	InferenceOutcomeSucceeded InferenceOutcome = "SUCCEEDED"
	InferenceOutcomeFailed    InferenceOutcome = "FAILED"
)

// InferenceResponseEventPayload is the worker-execution-owned response
// contract consumed by Factory event reducers. Diagnostics retain their public
// event JSON until the diagnostics owner projects them onto execution details.
type InferenceResponseEventPayload struct {
	Attempt            int                             `json:"attempt"`
	Diagnostics        json.RawMessage                 `json:"diagnostics,omitempty"`
	DurationMillis     int64                           `json:"durationMillis"`
	ExitCode           *int                            `json:"exitCode,omitempty"`
	FailureDetail      *InferenceResponseFailureDetail `json:"failureDetail,omitempty"`
	InferenceRequestID string                          `json:"inferenceRequestId"`
	Outcome            InferenceOutcome                `json:"outcome"`
	ProviderSession    *ProviderSessionMetadata        `json:"providerSession,omitempty"`
	Response           *string                         `json:"response,omitempty"`
}

// InferenceResponseFailureDetail carries the safe provider failure reported by
// an inference response event.
type InferenceResponseFailureDetail struct {
	Reason  WorkFailureType `json:"reason"`
	Message string          `json:"message"`
}

// ModelEventKind identifies which model-execution boundary fact was observed.
// Factory owns the corresponding canonical event vocabulary and envelope.
type ModelEventKind string

const (
	ModelEventKindRequest  ModelEventKind = "REQUEST"
	ModelEventKindResponse ModelEventKind = "RESPONSE"
)

// ModelEvent carries model-worker execution facts to Factory history without
// coupling worker composition to a transport or Factory event envelope.
type ModelEvent struct {
	ID         string
	Kind       ModelEventKind
	EventTime  time.Time
	Tick       int
	DispatchID string
	RequestID  string
	TraceIDs   []string
	WorkIDs    []string
	Request    *ModelRequestEventPayload
	Response   *ModelResponseEventPayload
}

// ModelResourceSummary records the concrete resource facts used by one model
// execution without exposing the Factory configuration or transport model.
type ModelResourceSummary struct {
	Backend    *string `json:"backend,omitempty"`
	Capacity   int     `json:"capacity"`
	LoadPolicy *string `json:"loadPolicy,omitempty"`
	Model      *string `json:"model,omitempty"`
	Name       string  `json:"name"`
	Provider   *string `json:"provider,omitempty"`
	Type       string  `json:"type"`
}

// ModelRequestEventPayload records the resolved model invocation boundary.
type ModelRequestEventPayload struct {
	Attempt          int                              `json:"attempt"`
	Bindings         *[]ResolvedModelOperationBinding `json:"bindings,omitempty"`
	Model            string                           `json:"model"`
	ModelRequestID   string                           `json:"modelRequestId"`
	Operation        string                           `json:"operation"`
	ProviderLocality string                           `json:"providerLocality"`
	Resources        *[]ModelResourceSummary          `json:"resources,omitempty"`
	Worker           string                           `json:"worker"`
	WorkingDirectory *string                          `json:"workingDirectory,omitempty"`
	Worktree         *string                          `json:"worktree,omitempty"`
}

// ModelResponseEventPayload records one model invocation result. Diagnostics
// retain their public camel-case event encoding as detached JSON.
type ModelResponseEventPayload struct {
	Attempt            int                              `json:"attempt"`
	Bindings           *[]ResolvedModelOperationBinding `json:"bindings,omitempty"`
	Diagnostics        json.RawMessage                  `json:"diagnostics,omitempty"`
	DurationMillis     int64                            `json:"durationMillis"`
	FailureDetail      *FailureDetail                   `json:"failureDetail,omitempty"`
	LoadDurationMillis *int64                           `json:"loadDurationMillis,omitempty"`
	LoadRequested      *bool                            `json:"loadRequested,omitempty"`
	LoadReused         *bool                            `json:"loadReused,omitempty"`
	Model              string                           `json:"model"`
	ModelRequestID     string                           `json:"modelRequestId"`
	Operation          string                           `json:"operation"`
	Outcome            InferenceOutcome                 `json:"outcome"`
	OutputContent      *[]work.WorkContentPart          `json:"outputContent,omitempty"`
	OutputPreview      *string                          `json:"outputPreview,omitempty"`
	ProviderSession    *ProviderSessionMetadata         `json:"providerSession,omitempty"`
	ProviderLocality   string                           `json:"providerLocality"`
	ResourceAcquired   *bool                            `json:"resourceAcquired,omitempty"`
	ResourceWaitMillis *int64                           `json:"resourceWaitMillis,omitempty"`
	Resources          *[]ModelResourceSummary          `json:"resources,omitempty"`
	Worker             string                           `json:"worker"`
}

// AgentRunResponseEvent carries worker-owned agent-loop completion facts to a
// Factory event recorder without coupling worker execution to a transport or
// Factory event envelope.
type AgentRunResponseEvent struct {
	ID         string
	DispatchID string
	EventTime  time.Time
	Payload    AgentRunResponseEventPayload
}

// AgentRunResponseEventPayload is the stable agent-run completion payload.
// Diagnostics retain their public camel-case event shape as detached JSON.
type AgentRunResponseEventPayload struct {
	AgentRunID     string          `json:"agentRunId"`
	Diagnostics    json.RawMessage `json:"diagnostics,omitempty"`
	DurationMillis int64           `json:"durationMillis"`
	Outcome        string          `json:"outcome"`
}

// ScriptEventKind identifies which worker-owned script boundary fact was
// observed. Factory owns the corresponding canonical event vocabulary.
type ScriptEventKind string

const (
	ScriptEventKindRequest  ScriptEventKind = "REQUEST"
	ScriptEventKindResponse ScriptEventKind = "RESPONSE"
)

// ScriptExecutionOutcome is the stable result category for a script attempt.
type ScriptExecutionOutcome string

const (
	ScriptExecutionOutcomeSucceeded      ScriptExecutionOutcome = "SUCCEEDED"
	ScriptExecutionOutcomeFailedExitCode ScriptExecutionOutcome = "FAILED_EXIT_CODE"
	ScriptExecutionOutcomeCanceled       ScriptExecutionOutcome = "CANCELED"
	ScriptExecutionOutcomeTimedOut       ScriptExecutionOutcome = "TIMED_OUT"
	ScriptExecutionOutcomeProcessError   ScriptExecutionOutcome = "PROCESS_ERROR"
)

// ScriptFailureType classifies failures where no normal process exit outcome
// is available.
type ScriptFailureType string

const (
	ScriptFailureTypeCanceled     ScriptFailureType = "CANCELED"
	ScriptFailureTypeTimeout      ScriptFailureType = "TIMEOUT"
	ScriptFailureTypeProcessError ScriptFailureType = "PROCESS_ERROR"
)

// ScriptEvent carries worker-owned script execution facts to Factory history
// without coupling the worker executor to a transport or Factory envelope.
type ScriptEvent struct {
	ID         string
	Kind       ScriptEventKind
	EventTime  time.Time
	Tick       int
	DispatchID string
	RequestID  string
	TraceIDs   []string
	WorkIDs    []string
	Request    *ScriptRequestEventPayload
	Response   *ScriptResponseEventPayload
}

// ScriptRequestEventPayload records the concrete command boundary while
// intentionally excluding environment values and stdin.
type ScriptRequestEventPayload struct {
	Args            []string `json:"args"`
	Attempt         int      `json:"attempt"`
	Command         string   `json:"command"`
	DispatchID      string   `json:"dispatchId"`
	ScriptRequestID string   `json:"scriptRequestId"`
	TransitionID    string   `json:"transitionId"`
}

// ScriptResponseEventPayload records one script attempt outcome.
type ScriptResponseEventPayload struct {
	Attempt         int                    `json:"attempt"`
	DispatchID      string                 `json:"dispatchId"`
	DurationMillis  int64                  `json:"durationMillis"`
	ExitCode        *int                   `json:"exitCode,omitempty"`
	FailureType     *ScriptFailureType     `json:"failureType,omitempty"`
	Outcome         ScriptExecutionOutcome `json:"outcome"`
	ScriptRequestID string                 `json:"scriptRequestId"`
	Stderr          string                 `json:"stderr"`
	Stdout          string                 `json:"stdout"`
	TransitionID    string                 `json:"transitionId"`
}

// DispatchResponseEventPayload is the worker-execution-owned completion
// contract consumed by Factory event reducers. FactoryEvent context remains
// authoritative for dispatch identity and ordering.
type DispatchResponseEventPayload struct {
	CompletionID                *string                      `json:"completionId,omitempty"`
	CurrentChainingTraceID      *string                      `json:"currentChainingTraceId,omitempty"`
	DurationMillis              *int64                       `json:"durationMillis,omitempty"`
	Error                       *string                      `json:"error,omitempty"`
	FailureDetail               *FailureDetail               `json:"failureDetail,omitempty"`
	Feedback                    *string                      `json:"feedback,omitempty"`
	Metadata                    map[string]string            `json:"metadata,omitempty"`
	Metrics                     *WorkMetricsEventPayload     `json:"metrics,omitempty"`
	Outcome                     WorkOutcome                  `json:"outcome"`
	Output                      *string                      `json:"output,omitempty"`
	OutputResources             *[]DispatchResourceEventRef  `json:"outputResources,omitempty"`
	OutputWork                  *[]work.WorkRequestEventWork `json:"outputWork,omitempty"`
	PreviousChainingTraceIDs    *[]string                    `json:"previousChainingTraceIds,omitempty"`
	ProviderFailure             *WorkFailureMetadata         `json:"providerFailure,omitempty"`
	SelectedClassificationLabel *string                      `json:"selectedClassificationLabel,omitempty"`
	TransitionID                string                       `json:"transitionId"`
}

// DispatchResourceEventRef preserves the public resource facts emitted with a
// completed dispatch without coupling worker execution to a transport model.
type DispatchResourceEventRef struct {
	Capacity int    `json:"capacity"`
	Name     string `json:"name"`
}

// WorkMetricsEventPayload preserves the millisecond-based public event shape
// until replay converts it to the duration-based execution result contract.
type WorkMetricsEventPayload struct {
	Cost           *float64 `json:"cost,omitempty"`
	DurationMillis *int64   `json:"durationMillis,omitempty"`
	RetryCount     *int     `json:"retryCount,omitempty"`
}

// WorkstationResult describes the business result of one workstation execution
// carried by Factory event payloads and world-state projections.
type WorkstationResult struct {
	Outcome                     string               `json:"outcome"`
	Output                      string               `json:"output,omitempty"`
	Error                       string               `json:"error,omitempty"`
	Feedback                    string               `json:"feedback,omitempty"`
	SelectedClassificationLabel string               `json:"selected_classification_label,omitempty"`
	FailureDetail               *FailureDetail       `json:"failureDetail,omitempty"`
	FailureMetadata             *WorkFailureMetadata `json:"failure_metadata,omitempty"`
}

func CloneWorkstationResult(result WorkstationResult) WorkstationResult {
	clone := result
	clone.FailureDetail = CloneFailureDetail(result.FailureDetail)
	clone.FailureMetadata = CloneWorkFailureMetadata(result.FailureMetadata)
	return clone
}

// WorkResult is returned by a worker after processing.
// The Outcome determines which arc set is used to route the resulting tokens.
type WorkResult struct {
	DispatchID                  string                   `json:"dispatch_id"`
	TransitionID                string                   `json:"transition_id"`
	Outcome                     WorkOutcome              `json:"outcome"`
	Output                      string                   `json:"output,omitempty"`
	RecordedOutputWork          []work.FactoryWorkItem   `json:"recorded_output_work,omitempty"`
	Error                       string                   `json:"error,omitempty"`
	Feedback                    string                   `json:"feedback,omitempty"`
	SelectedClassificationLabel string                   `json:"selected_classification_label,omitempty"`
	FailureMetadata             *WorkFailureMetadata     `json:"failure_metadata,omitempty"`
	ProviderSession             *ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics                 *WorkDiagnostics         `json:"diagnostics,omitempty"`
	Metrics                     WorkMetrics              `json:"metrics"`
}

// ProviderSessionMetadata carries a stable provider rollout/session identity.
type ProviderSessionMetadata = providersessions.Metadata

// CanonicalProviderSessionProvider maps provider-session identities onto the
// stable backend-facing names used for loading, events, and persisted
// diagnostics. Cursor keeps the CLI command name `agent` but stores `cursor`
// as the provider-session contract.
func CanonicalProviderSessionProvider(provider string) string {
	return providersessions.CanonicalProvider(provider)
}

// WorkOutcome distinguishes the result routing behavior for worker output.
type WorkOutcome string

const (
	// OutcomeAccepted means the transition succeeded. Use output arcs.
	OutcomeAccepted WorkOutcome = "ACCEPTED"
	// OutcomeContinue means the worker made partial progress. Use continue arcs.
	OutcomeContinue WorkOutcome = "CONTINUE"
	// OutcomeRejected means the business result was negative. Use rejection arcs.
	OutcomeRejected WorkOutcome = "REJECTED"
	// OutcomeFailed means execution crashed, timed out, or hit a system error.
	OutcomeFailed WorkOutcome = "FAILED"
)

// WorkMetrics captures performance data from a worker execution.
type WorkMetrics struct {
	Duration   time.Duration `json:"duration"`
	Cost       float64       `json:"cost"`
	RetryCount int           `json:"retry_count"`
}

// WorkDiagnostics carries nested provider and script diagnostics.
type WorkDiagnostics struct {
	RenderedPrompt *RenderedPromptDiagnostic `json:"rendered_prompt,omitempty"`
	Provider       *ProviderDiagnostic       `json:"provider,omitempty"`
	Invocation     *InvocationDiagnostic     `json:"invocation,omitempty"`
	Command        *CommandDiagnostic        `json:"command,omitempty"`
	Panic          *PanicDiagnostic          `json:"panic,omitempty"`
	Metadata       map[string]string         `json:"metadata,omitempty"`
}

// RenderedPromptDiagnostic describes prompt material rendered for a model worker.
type RenderedPromptDiagnostic struct {
	SystemPromptHash string            `json:"system_prompt_hash,omitempty"`
	UserMessageHash  string            `json:"user_message_hash,omitempty"`
	Variables        map[string]string `json:"variables,omitempty"`
}

// ProviderDiagnostic records provider request and response metadata.
type ProviderDiagnostic struct {
	Provider         string            `json:"provider,omitempty"`
	Model            string            `json:"model,omitempty"`
	RequestMetadata  map[string]string `json:"request_metadata,omitempty"`
	ResponseMetadata map[string]string `json:"response_metadata,omitempty"`
}

// Provider response metadata keys are shared across provider normalization and
// runtime metrics so core factory packages do not depend on provider adapters.
const (
	ProviderResponseMetadataDurationMS    = "duration_ms"
	ProviderResponseMetadataDurationAPIMS = "duration_api_ms"
	ProviderResponseMetadataInputTokens   = "input_tokens"
	ProviderResponseMetadataOutputTokens  = "output_tokens"
)

// InvocationDiagnostic records replay-safe invocation metadata derived from
// canonical normalized arguments without exposing raw values.
type InvocationDiagnostic struct {
	SignatureHash string                          `json:"signature_hash,omitempty"`
	Parameters    []InvocationParameterDiagnostic `json:"parameters,omitempty"`
}

// InvocationParameterDiagnostic records one normalized invocation parameter in
// a replay-safe form.
type InvocationParameterDiagnostic struct {
	Name        string   `json:"name,omitempty"`
	SourceKinds []string `json:"source_kinds,omitempty"`
	ValueCount  int      `json:"value_count,omitempty"`
	Redacted    bool     `json:"redacted,omitempty"`
}

// CommandDiagnostic records script and provider command execution details.
type CommandDiagnostic struct {
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Stdin      string            `json:"stdin,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Stdout     string            `json:"stdout,omitempty"`
	Stderr     string            `json:"stderr,omitempty"`
	ExitCode   int               `json:"exit_code,omitempty"`
	TimedOut   bool              `json:"timed_out,omitempty"`
	Duration   time.Duration     `json:"duration,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
}

// PanicDiagnostic records panic details captured at worker boundaries.
type PanicDiagnostic struct {
	Message string `json:"message,omitempty"`
	Stack   string `json:"stack,omitempty"`
}

// WorkFailureFamily captures the runtime behavior category for a normalized
// work failure.
type WorkFailureFamily string

const (
	WorkFailureFamilyTerminal  WorkFailureFamily = "terminal"
	WorkFailureFamilyRetryable WorkFailureFamily = "retryable"
	WorkFailureFamilyThrottle  WorkFailureFamily = "throttle"
)

// WorkFailureType is the stable customer-facing normalized failure type for
// scoped runtime work execution paths.
type WorkFailureType string

const (
	WorkFailureTypeAuthFailure         WorkFailureType = "auth_failure"
	WorkFailureTypePermanentBadRequest WorkFailureType = "permanent_bad_request"
	WorkFailureTypeThrottled           WorkFailureType = "throttled"
	WorkFailureTypeInternalServerError WorkFailureType = "internal_server_error"
	WorkFailureTypeTimeout             WorkFailureType = "timeout"
	WorkFailureTypeUnknown             WorkFailureType = "unknown"
	WorkFailureTypeMisconfigured       WorkFailureType = "misconfigured"
	WorkFailureTypeCommandLineTooLong  WorkFailureType = "command_line_too_long"
	WorkFailureTypeMissingExecutable   WorkFailureType = "missing_executable"
)

// FailureDetail is the canonical customer-safe explanation of a failed
// operation. Runtime projections copy this value without reclassifying or
// reparsing provider output.
type FailureDetail struct {
	Reason  WorkFailureType `json:"reason"`
	Message string          `json:"message"`
}

// WorkFailureDecision is the normalized behavior contract consumed by
// downstream retry, termination, and throttle-pause logic.
type WorkFailureDecision struct {
	Retryable             bool
	Terminal              bool
	TriggersThrottlePause bool
}

// WorkFailureMetadata carries the normalized failure contract
// across runtime boundaries after the original error has been rendered.
type WorkFailureMetadata struct {
	Family WorkFailureFamily `json:"family,omitempty"`
	Type   WorkFailureType   `json:"type,omitempty"`
}

// FailureDecisionFromMetadata resolves durable failure metadata into the
// retry/throttle/terminal policy consumed outside the provider implementation.
func FailureDecisionFromMetadata(metadata *WorkFailureMetadata) WorkFailureDecision {
	if metadata == nil {
		return WorkFailureDecision{}
	}
	switch metadata.Type {
	case WorkFailureTypeThrottled:
		return WorkFailureDecision{Retryable: true, TriggersThrottlePause: true}
	case WorkFailureTypeInternalServerError, WorkFailureTypeTimeout:
		return WorkFailureDecision{Retryable: true}
	case WorkFailureTypeAuthFailure,
		WorkFailureTypePermanentBadRequest,
		WorkFailureTypeUnknown,
		WorkFailureTypeMisconfigured,
		WorkFailureTypeMissingExecutable,
		WorkFailureTypeCommandLineTooLong:
		return WorkFailureDecision{Terminal: true}
	}
	switch metadata.Family {
	case WorkFailureFamilyRetryable:
		return WorkFailureDecision{Retryable: true}
	case WorkFailureFamilyThrottle:
		return WorkFailureDecision{Retryable: true, TriggersThrottlePause: true}
	default:
		return WorkFailureDecision{Terminal: true}
	}
}

func CloneProviderSessionMetadata(session *ProviderSessionMetadata) *ProviderSessionMetadata {
	return providersessions.CloneMetadata(session)
}

func CloneWorkFailureMetadata(failure *WorkFailureMetadata) *WorkFailureMetadata {
	if failure == nil {
		return nil
	}
	clone := *failure
	return &clone
}

func CloneFailureDetail(detail *FailureDetail) *FailureDetail {
	if detail == nil {
		return nil
	}
	clone := *detail
	return &clone
}

func CloneWorkDiagnostics(diagnostics *WorkDiagnostics) *WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	clone := &WorkDiagnostics{
		RenderedPrompt: cloneRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneProviderDiagnostic(diagnostics.Provider),
		Invocation:     cloneInvocationDiagnostic(diagnostics.Invocation),
		Command:        cloneCommandDiagnostic(diagnostics.Command),
		Metadata:       cloneStringMap(diagnostics.Metadata),
	}
	if diagnostics.Panic != nil {
		clone.Panic = &PanicDiagnostic{Message: diagnostics.Panic.Message, Stack: diagnostics.Panic.Stack}
	}
	return clone
}

func CloneInvocationDiagnostic(diagnostic *InvocationDiagnostic) *InvocationDiagnostic {
	return cloneInvocationDiagnostic(diagnostic)
}

func cloneRenderedPromptDiagnostic(diagnostic *RenderedPromptDiagnostic) *RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &RenderedPromptDiagnostic{SystemPromptHash: diagnostic.SystemPromptHash, UserMessageHash: diagnostic.UserMessageHash, Variables: cloneStringMap(diagnostic.Variables)}
}

func cloneProviderDiagnostic(diagnostic *ProviderDiagnostic) *ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &ProviderDiagnostic{Provider: diagnostic.Provider, Model: diagnostic.Model, RequestMetadata: cloneStringMap(diagnostic.RequestMetadata), ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata)}
}

func cloneInvocationDiagnostic(diagnostic *InvocationDiagnostic) *InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	clone := &InvocationDiagnostic{SignatureHash: diagnostic.SignatureHash}
	if len(diagnostic.Parameters) > 0 {
		clone.Parameters = make([]InvocationParameterDiagnostic, len(diagnostic.Parameters))
		for i, parameter := range diagnostic.Parameters {
			clone.Parameters[i] = InvocationParameterDiagnostic{Name: parameter.Name, SourceKinds: append([]string(nil), parameter.SourceKinds...), ValueCount: parameter.ValueCount, Redacted: parameter.Redacted}
		}
	}
	return clone
}

func cloneCommandDiagnostic(diagnostic *CommandDiagnostic) *CommandDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &CommandDiagnostic{Command: diagnostic.Command, Args: append([]string(nil), diagnostic.Args...), Stdin: diagnostic.Stdin, Env: cloneStringMap(diagnostic.Env), Stdout: diagnostic.Stdout, Stderr: diagnostic.Stderr, ExitCode: diagnostic.ExitCode, TimedOut: diagnostic.TimedOut, Duration: diagnostic.Duration, WorkingDir: diagnostic.WorkingDir}
}

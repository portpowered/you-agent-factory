package interfaces

import "time"

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

// WorkResult is returned by a worker after processing.
// The Outcome determines which arc set is used to route the resulting tokens.
type WorkResult struct {
	DispatchID         string                   `json:"dispatch_id"`
	TransitionID       string                   `json:"transition_id"`
	Outcome            WorkOutcome              `json:"outcome"`
	Output             string                   `json:"output,omitempty"`
	SpawnedWork        []TokenColor             `json:"spawned_work,omitempty"`
	RecordedOutputWork []FactoryWorkItem        `json:"recorded_output_work,omitempty"`
	Error              string                   `json:"error,omitempty"`
	Feedback           string                   `json:"feedback,omitempty"`
	ProviderFailure    *ProviderFailureMetadata `json:"provider_failure,omitempty"`
	ProviderSession    *ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics        *WorkDiagnostics         `json:"diagnostics,omitempty"`
	Metrics            WorkMetrics              `json:"metrics"`
}

// ProviderSessionMetadata carries a stable provider rollout/session identity.
type ProviderSessionMetadata struct {
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind,omitempty"`
	ID       string `json:"id,omitempty"`
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

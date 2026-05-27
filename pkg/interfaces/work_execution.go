package interfaces

import (
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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

// WorkResult is returned by a worker after processing.
// The Outcome determines which arc set is used to route the resulting tokens.
type WorkResult struct {
	DispatchID                  string                   `json:"dispatch_id"`
	TransitionID                string                   `json:"transition_id"`
	Outcome                     WorkOutcome              `json:"outcome"`
	Output                      string                   `json:"output,omitempty"`
	SpawnedWork                 []TokenColor             `json:"spawned_work,omitempty"`
	RecordedOutputWork          []FactoryWorkItem        `json:"recorded_output_work,omitempty"`
	Error                       string                   `json:"error,omitempty"`
	Feedback                    string                   `json:"feedback,omitempty"`
	SelectedClassificationLabel string                   `json:"selected_classification_label,omitempty"`
	FailureMetadata             *WorkFailureMetadata     `json:"failure_metadata,omitempty"`
	ProviderFailure             *ProviderFailureMetadata `json:"provider_failure,omitempty"`
	ProviderSession             *ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics                 *WorkDiagnostics         `json:"diagnostics,omitempty"`
	Metrics                     WorkMetrics              `json:"metrics"`
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

// WorkFailureFamily captures the runtime behavior category for a normalized
// work failure.
type WorkFailureFamily string

const (
	WorkFailureFamilyTerminal  WorkFailureFamily = "terminal"
	WorkFailureFamilyRetryable WorkFailureFamily = "retryable"
	WorkFailureFamilyThrottle  WorkFailureFamily = "throttle"
)

// ProviderErrorFamily remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderErrorFamily = WorkFailureFamily

const (
	ProviderErrorFamilyTerminal  ProviderErrorFamily = WorkFailureFamilyTerminal
	ProviderErrorFamilyRetryable ProviderErrorFamily = WorkFailureFamilyRetryable
	ProviderErrorFamilyThrottle  ProviderErrorFamily = WorkFailureFamilyThrottle
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
)

// ProviderErrorType remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderErrorType = WorkFailureType

const (
	ProviderErrorTypeAuthFailure         ProviderErrorType = WorkFailureTypeAuthFailure
	ProviderErrorTypePermanentBadRequest ProviderErrorType = WorkFailureTypePermanentBadRequest
	ProviderErrorTypeThrottled           ProviderErrorType = WorkFailureTypeThrottled
	ProviderErrorTypeInternalServerError ProviderErrorType = WorkFailureTypeInternalServerError
	ProviderErrorTypeTimeout             ProviderErrorType = WorkFailureTypeTimeout
	ProviderErrorTypeUnknown             ProviderErrorType = WorkFailureTypeUnknown
	ProviderErrorTypeMisconfigured       ProviderErrorType = WorkFailureTypeMisconfigured
)

// WorkFailureDecision is the normalized behavior contract consumed by
// downstream retry, termination, and throttle-pause logic.
type WorkFailureDecision struct {
	Retryable             bool
	Terminal              bool
	TriggersThrottlePause bool
}

// ProviderFailureDecision remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderFailureDecision = WorkFailureDecision

// WorkFailureMetadata carries the normalized failure contract
// across runtime boundaries after the original error has been rendered.
type WorkFailureMetadata struct {
	Family WorkFailureFamily `json:"family"`
	Type   WorkFailureType   `json:"type"`
}

// ProviderFailureMetadata remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderFailureMetadata = WorkFailureMetadata

// CanonicalWorkFailureMetadata returns the generalized failure metadata from
// the runtime result, falling back to the legacy provider-named field while
// older callers are still being migrated.
func CanonicalWorkFailureMetadata(failure *WorkFailureMetadata, providerFailure *ProviderFailureMetadata) *WorkFailureMetadata {
	if failure != nil {
		return failure
	}
	return providerFailure
}

// ReplayArtifact is the versioned, self-contained recording used to replay a
// factory run without requiring the original customer files or live side
// effects.
type ReplayArtifact struct {
	SchemaVersion string                    `json:"schemaVersion"`
	RecordedAt    time.Time                 `json:"recordedAt"`
	Events        []factoryapi.FactoryEvent `json:"events"`

	// The fields below are hydrated from Events for the current replay
	// implementation. They are intentionally excluded from artifact storage.
	Factory     factoryapi.Factory       `json:"-"`
	Diagnostics ReplayDiagnostics        `json:"-"`
	WallClock   *ReplayWallClockMetadata `json:"-"`
}

// ReplayDiagnostics stores artifact-level notes and optional nested execution
// details.
type ReplayDiagnostics struct {
	Notes   []string                       `json:"notes,omitempty"`
	Workers map[string]SafeWorkDiagnostics `json:"workers,omitempty"`
}

// ReplayWallClockMetadata retains wall-clock timing for investigation only.
// Replay behavior is driven by logical ticks, not these timestamps.
type ReplayWallClockMetadata struct {
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

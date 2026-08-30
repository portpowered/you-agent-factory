package factorycontracts

import (
	"fmt"
	"math"
	"strings"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	FactoryWebhookEventTypeWorkStateChange     = "WORK_STATE_CHANGE"
	FactoryWebhookEventTypeDispatchResponse    = "DISPATCH_RESPONSE"
	FactoryWebhookEventTypeDispatchReconciled  = "DISPATCH_RECONCILED"
	FactoryWebhookEventTypeDispatchInterrupted = "DISPATCH_INTERRUPTED"

	FactoryWebhookDispatchStatusFailed      = "FAILED"
	FactoryWebhookDispatchStatusInterrupted = "INTERRUPTED"

	DefaultFactoryWebhookRequestTimeout    = 10 * time.Second
	DefaultFactoryWebhookMaxAttempts       = 5
	DefaultFactoryWebhookInitialBackoff    = time.Second
	DefaultFactoryWebhookBackoffMultiplier = 2.0
	DefaultFactoryWebhookMaxBackoff        = 30 * time.Second
)

// FactoryWebhookConfig declares one outbound subscription without carrying
// resolved secret material. Delivery policy is resolved only at runtime.
type FactoryWebhookConfig struct {
	Name             string                              `json:"name" yaml:"name"`
	Enabled          bool                                `json:"enabled" yaml:"enabled"`
	URL              string                              `json:"url" yaml:"url"`
	SigningSecretRef string                              `json:"signingSecretRef" yaml:"signingSecretRef"`
	Filter           FactoryWebhookFilterConfig          `json:"filter" yaml:"filter"`
	DeliveryPolicy   *FactoryWebhookDeliveryPolicyConfig `json:"deliveryPolicy,omitempty" yaml:"deliveryPolicy,omitempty"`
}

// FactoryWebhookFilterConfig selects canonical Factory Event types and, for
// dispatch event types, optional canonical dispatch statuses.
type FactoryWebhookFilterConfig struct {
	EventTypes       []string `json:"eventTypes" yaml:"eventTypes"`
	DispatchStatuses []string `json:"dispatchStatuses,omitempty" yaml:"dispatchStatuses,omitempty"`
}

// FactoryWebhookDeliveryPolicyConfig keeps optional authored values distinct
// from their effective defaults so explicit invalid zero values are rejected.
type FactoryWebhookDeliveryPolicyConfig struct {
	RequestTimeout    *string  `json:"requestTimeout,omitempty" yaml:"requestTimeout,omitempty"`
	MaxAttempts       *int     `json:"maxAttempts,omitempty" yaml:"maxAttempts,omitempty"`
	InitialBackoff    *string  `json:"initialBackoff,omitempty" yaml:"initialBackoff,omitempty"`
	BackoffMultiplier *float64 `json:"backoffMultiplier,omitempty" yaml:"backoffMultiplier,omitempty"`
	MaxBackoff        *string  `json:"maxBackoff,omitempty" yaml:"maxBackoff,omitempty"`
}

// FactoryWebhookEffectiveDeliveryPolicy contains parsed, bounded values used
// by the delivery runtime.
type FactoryWebhookEffectiveDeliveryPolicy struct {
	RequestTimeout    time.Duration
	MaxAttempts       int
	InitialBackoff    time.Duration
	BackoffMultiplier float64
	MaxBackoff        time.Duration
}

// ResolveFactoryWebhookDeliveryPolicy applies the documented defaults and
// parses authored Go duration values for a webhook delivery policy.
func ResolveFactoryWebhookDeliveryPolicy(config *FactoryWebhookDeliveryPolicyConfig) (FactoryWebhookEffectiveDeliveryPolicy, error) {
	effective := FactoryWebhookEffectiveDeliveryPolicy{RequestTimeout: DefaultFactoryWebhookRequestTimeout, MaxAttempts: DefaultFactoryWebhookMaxAttempts, InitialBackoff: DefaultFactoryWebhookInitialBackoff, BackoffMultiplier: DefaultFactoryWebhookBackoffMultiplier, MaxBackoff: DefaultFactoryWebhookMaxBackoff}
	if config == nil {
		return effective, nil
	}
	var err error
	if effective.RequestTimeout, err = resolveFactoryWebhookDuration("requestTimeout", config.RequestTimeout, effective.RequestTimeout); err != nil {
		return FactoryWebhookEffectiveDeliveryPolicy{}, err
	}
	if config.MaxAttempts != nil {
		if *config.MaxAttempts <= 0 {
			return FactoryWebhookEffectiveDeliveryPolicy{}, fmt.Errorf("maxAttempts must be positive")
		}
		effective.MaxAttempts = *config.MaxAttempts
	}
	if effective.InitialBackoff, err = resolveFactoryWebhookDuration("initialBackoff", config.InitialBackoff, effective.InitialBackoff); err != nil {
		return FactoryWebhookEffectiveDeliveryPolicy{}, err
	}
	if config.BackoffMultiplier != nil {
		if math.IsNaN(*config.BackoffMultiplier) || math.IsInf(*config.BackoffMultiplier, 0) || *config.BackoffMultiplier < 1 {
			return FactoryWebhookEffectiveDeliveryPolicy{}, fmt.Errorf("backoffMultiplier must be at least 1")
		}
		effective.BackoffMultiplier = *config.BackoffMultiplier
	}
	if effective.MaxBackoff, err = resolveFactoryWebhookDuration("maxBackoff", config.MaxBackoff, effective.MaxBackoff); err != nil {
		return FactoryWebhookEffectiveDeliveryPolicy{}, err
	}
	if effective.MaxBackoff < effective.InitialBackoff {
		return FactoryWebhookEffectiveDeliveryPolicy{}, fmt.Errorf("maxBackoff must not be less than initialBackoff")
	}
	return effective, nil
}

func resolveFactoryWebhookDuration(field string, value *string, fallback time.Duration) (time.Duration, error) {
	if value == nil {
		return fallback, nil
	}
	duration, err := time.ParseDuration(strings.TrimSpace(*value))
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", field)
	}
	return duration, nil
}

// RequestValidationError reports a stable client-side validation failure.
type RequestValidationError struct {
	Message string
}

func (e *RequestValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// FactoryState represents the current lifecycle state of a Factory.
type FactoryState string

const (
	FactoryStateIdle      FactoryState = "IDLE"
	FactoryStateRunning   FactoryState = "RUNNING"
	FactoryStatePaused    FactoryState = "PAUSED"
	FactoryStateCompleted FactoryState = "COMPLETED"
	FactoryStateFailed    FactoryState = "FAILED"
)

// RuntimeMode determines whether the runtime exits on idle completion or stays
// available for future submissions until its context is canceled.
type RuntimeMode string

const (
	RuntimeModeBatch   RuntimeMode = "BATCH"
	RuntimeModeService RuntimeMode = "SERVICE"
)

// TerminationClassification describes the outcome of a finite runtime drain.
// It is carried with the tick result so the engine can preserve the final
// submission-drain race protection before converting an incomplete drain into
// a runtime error.
type TerminationClassification string

const (
	TerminationClassificationComplete   TerminationClassification = "COMPLETE"
	TerminationClassificationIncomplete TerminationClassification = "INCOMPLETE"
)

// TerminationResult is the authoritative finite-runtime termination decision.
// NonTerminalWorkCount is the number of distinct customer Work items that were
// still non-terminal when the runtime became quiescent.
type TerminationResult struct {
	Classification       TerminationClassification `json:"classification"`
	NonTerminalWorkCount int                       `json:"non_terminal_work_count,omitempty"`
}

// InvocationTerminalStatus is the Factory Session-owned terminal outcome for
// one invocation. Transport adapters map it to their generated contract.
type InvocationTerminalStatus = string

const (
	InvocationTerminalStatusCanceled  InvocationTerminalStatus = "CANCELED"
	InvocationTerminalStatusCompleted InvocationTerminalStatus = "COMPLETED"
	InvocationTerminalStatusFailed    InvocationTerminalStatus = "FAILED"
	InvocationTerminalStatusTimedOut  InvocationTerminalStatus = "TIMED_OUT"
)

// InvocationErrorCode is the stable Factory Session-owned failure code emitted
// with a non-completed invocation result.
type InvocationErrorCode string

const (
	InvocationErrorCodeCanceled       InvocationErrorCode = "INVOCATION_CANCELED"
	InvocationErrorCodeRuntimeFailure InvocationErrorCode = "INVOCATION_RUNTIME_FAILURE"
	InvocationErrorCodeTimedOut       InvocationErrorCode = "INVOCATION_TIMED_OUT"
)

// FactoryInvocationResult carries the transport-independent outcome of one
// Factory Session invocation after input resolution and result selection.
type FactoryInvocationResult struct {
	RequestID       string
	TraceID         string
	Status          InvocationTerminalStatus
	PrimaryResult   []work.WorkContentPart
	ErrorCode       string
	Message         string
	SessionID       string
	WorkID          string
	WorkName        string
	WorkState       string
	ApprovalID      string
	DispatchID      string
	WorkstationID   string
	WorkstationName string
	Decisions       []string
}

// CanonicalEventTime normalizes runtime event boundary timestamps to UTC while
// preserving zero values so optional/fallback handling remains explicit.
func CanonicalEventTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.UTC()
}

// RuntimeWorkstationLookup resolves runtime workstation definitions by authored name.
type RuntimeWorkstationLookup interface {
	Workstation(name string) (*FactoryWorkstationConfig, bool)
}

// RuntimeDefinitionLookup resolves runtime worker and workstation definitions by authored name.
type RuntimeDefinitionLookup interface {
	RuntimeWorkstationLookup
	Worker(name string) (*workerconfig.Config, bool)
}

// RuntimeFactoryConfigLookup resolves the effective runtime factory config when
// a consumer needs optional access to factory-level settings.
type RuntimeFactoryConfigLookup interface {
	FactoryConfig() *FactoryConfig
}

// PromptSource identifies the fixed authored file used to refresh one prompt
// at dispatch time. It is runtime metadata, not Factory configuration.
type PromptSource struct {
	Path       string
	IsTemplate bool
}

// RuntimePromptProvenance carries the value-free authored prompt fields that
// were present before invocation interpolation. Runtime snapshots use this
// metadata when an inline Factory definition has no prompt source path to
// identify the authored placeholders at dispatch time.
type RuntimePromptProvenance struct {
	Name           string
	Body           string
	PromptTemplate string
}

// RuntimePromptProvenanceLookup exposes authored prompt fields separately from
// the effective runtime Factory Definition. The fields contain authored
// placeholders, never resolved invocation values.
type RuntimePromptProvenanceLookup interface {
	WorkerPromptProvenance(name string) (RuntimePromptProvenance, bool)
	WorkstationPromptProvenance(name string) (RuntimePromptProvenance, bool)
}

// RuntimePromptSourceLookup exposes authored prompt identity without adding
// source paths to the customer-facing runtime Factory Definition.
type RuntimePromptSourceLookup interface {
	WorkerPromptSource(name string) (PromptSource, bool)
	WorkstationPromptSource(name string) (PromptSource, bool)
}

// RuntimeConfigLookup exposes the canonical public runtime-facing lookup
// contract for consumers that need runtime definitions plus path-aware
// execution lookups.
type RuntimeConfigLookup interface {
	RuntimeDefinitionLookup
	RuntimeFactoryConfigLookup
	FactoryDir() string
	RuntimeBaseDir() string
}

func firstNonNilLookup[T comparable](lookups ...T) T {
	var zero T
	for _, lookup := range lookups {
		if lookup != zero {
			return lookup
		}
	}
	return zero
}

// FirstRuntimeDefinitionLookup returns the first non-nil runtime definition
// lookup from the provided candidates.
func FirstRuntimeDefinitionLookup(lookups ...RuntimeDefinitionLookup) RuntimeDefinitionLookup {
	return firstNonNilLookup(lookups...)
}

// FirstRuntimeFactoryConfigLookup returns the first non-nil runtime factory
// config lookup from the provided candidates.
func FirstRuntimeFactoryConfigLookup(lookups ...RuntimeFactoryConfigLookup) RuntimeFactoryConfigLookup {
	return firstNonNilLookup(lookups...)
}

// FirstRuntimeWorkstationLookup returns the first non-nil runtime workstation
// lookup from the provided candidates.
func FirstRuntimeWorkstationLookup(lookups ...RuntimeWorkstationLookup) RuntimeWorkstationLookup {
	return firstNonNilLookup(lookups...)
}

// MutationType describes the kind of marking mutation.
type MutationType string

const (
	MutationMove    MutationType = "MOVE"
	MutationCreate  MutationType = "CREATE"
	MutationConsume MutationType = "CONSUME"
)

// MarkingMutation is a declarative description of a single token movement.
type MarkingMutation struct {
	Type           MutationType              `json:"type"`
	TokenID        string                    `json:"token_id"`
	FromPlace      string                    `json:"from_place"`
	ToPlace        string                    `json:"to_place"`
	Reason         string                    `json:"reason"`
	NewToken       *workerexecution.Token    `json:"-"`
	FailureRecords []workerexecution.Failure `json:"-"`
}

// TokenMutationRecord stores the raw token mutation emitted while applying a
// worker result.
type TokenMutationRecord struct {
	DispatchID   string                      `json:"dispatch_id"`
	TransitionID string                      `json:"transition_id"`
	Outcome      workerexecution.WorkOutcome `json:"outcome"`
	Type         MutationType                `json:"type"`
	TokenID      string                      `json:"token_id"`
	FromPlace    string                      `json:"from_place"`
	ToPlace      string                      `json:"to_place"`
	Reason       string                      `json:"reason"`
	Token        *workerexecution.Token      `json:"token,omitempty"`
	// Terminal is an execution-owner fact about the token's destination (or
	// source for CONSUME). It is populated by the runtime that owns the
	// topology, rather than inferred from authored state names at persistence
	// time. Older records leave it false and remain lossless until a current
	// runtime supplies enough facts to compact them safely.
	Terminal bool `json:"terminal,omitempty"`
	// TransitionReachable reports whether a live transition can still use the
	// terminal place. A consumed terminal token is unreachable by definition.
	TransitionReachable bool `json:"transition_reachable,omitempty"`
}

// DispatchEntry tracks an in-flight dispatch awaiting a worker result.
type DispatchEntry struct {
	DispatchID              string                                `json:"dispatch_id"`
	TransitionID            string                                `json:"transition_id"`
	WorkstationName         string                                `json:"workstation_name,omitempty"`
	ExpectedArtifactContext *work.ExpectedArtifactTemplateContext `json:"expected_artifact_context,omitempty"`
	StartTime               time.Time                             `json:"start_time"`
	ConsumedTokens          []workerexecution.Token               `json:"consumed_tokens"`
	HeldMutations           []MarkingMutation                     `json:"held_mutations"`
}

// CompletedDispatch records a dispatch that has finished, with timing data.
type CompletedDispatch struct {
	DispatchID                  string                                        `json:"dispatch_id"`
	TransitionID                string                                        `json:"transition_id"`
	WorkstationName             string                                        `json:"workstation_name,omitempty"`
	ExpectedArtifactContext     *work.ExpectedArtifactTemplateContext         `json:"expected_artifact_context,omitempty"`
	Outcome                     workerexecution.WorkOutcome                   `json:"outcome"`
	Cancellation                *workerexecution.DispatchCancellation         `json:"cancellation,omitempty"`
	SelectedClassificationLabel string                                        `json:"selected_classification_label,omitempty"`
	Reason                      string                                        `json:"reason,omitempty"`
	ArtifactVerification        *workerexecution.ExpectedArtifactVerification `json:"artifact_verification,omitempty"`
	FailureMetadata             *workerexecution.WorkFailureMetadata          `json:"failure_metadata,omitempty"`
	FailureDetail               *workerexecution.FailureDetail                `json:"failure_detail,omitempty"`
	ProviderSession             *providers.SessionMetadata                    `json:"provider_session,omitempty"`
	StartTime                   time.Time                                     `json:"start_time"`
	EndTime                     time.Time                                     `json:"end_time"`
	Duration                    time.Duration                                 `json:"duration"`
	ConsumedTokens              []workerexecution.Token                       `json:"consumed_tokens,omitempty"`
	OutputMutations             []TokenMutationRecord                         `json:"output_mutations,omitempty"`
	// IgnoredResult is an internal handoff marker. It lets Runtime retire the
	// dispatch while recording a redacted diagnostic instead of a normal
	// DISPATCH_RESPONSE. It is intentionally absent from runtime snapshots and
	// replay artifacts because the canonical event is the source of truth.
	IgnoredResult *DispatchResultIgnoredEventPayload `json:"-"`
	IgnoredWorkID string                             `json:"-"`
}

// ActiveThrottlePause records an active provider/model dispatch pause window.
type ActiveThrottlePause struct {
	LaneID      string    `json:"lane_id"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	PausedAt    time.Time `json:"paused_at,omitempty"`
	PausedUntil time.Time `json:"paused_until"`
}

// EnabledTransition represents a transition that is ready to fire.
type EnabledTransition struct {
	TransitionID string                             `json:"transition_id"`
	WorkerType   string                             `json:"worker_type"`
	Bindings     map[string][]workerexecution.Token `json:"bindings"`
	ArcModes     map[string]ArcMode                 `json:"arc_modes"`
}

// ArcMode describes how an enabled transition uses an input arc.
type ArcMode int

const (
	ArcModeConsume ArcMode = iota
	ArcModeObserve
)

// FiringDecision represents a scheduler's decision to fire a transition.
type FiringDecision struct {
	TransitionID  string              `json:"transition_id"`
	InputTokens   []string            `json:"input_tokens,omitempty"`
	ConsumeTokens []string            `json:"consume_tokens"`
	WorkerType    string              `json:"worker_type"`
	InputBindings map[string][]string `json:"input_bindings,omitempty"`
}

// TickResult is the output of a single subsystem execution.
type TickResult struct {
	Mutations              []MarkingMutation               `json:"mutations,omitempty"`
	GeneratedBatches       []work.GeneratedSubmissionBatch `json:"generated_batches,omitempty"`
	Dispatches             []DispatchRecord                `json:"dispatches,omitempty"`
	Histories              []workerexecution.History       `json:"histories,omitempty"`
	CompletedDispatches    []CompletedDispatch             `json:"completed_dispatches,omitempty"`
	ActiveThrottlePauses   []ActiveThrottlePause           `json:"active_throttle_pauses,omitempty"`
	ThrottlePausesObserved bool                            `json:"throttle_pauses_observed,omitempty"`
	ShouldTerminate        bool                            `json:"should_terminate,omitempty"`
	Termination            *TerminationResult              `json:"termination,omitempty"`
}

// DispatchRecord pairs a WorkDispatch with the marking mutations consumed to fire it.
type DispatchRecord struct {
	Dispatch  work.WorkDispatch `json:"dispatch"`
	Mutations []MarkingMutation `json:"mutations"`
}

// RuntimeStatus describes whether the runtime is actively processing work,
// intentionally idle but still available, or terminally finished.
type RuntimeStatus string

const (
	RuntimeStatusActive   RuntimeStatus = "ACTIVE"
	RuntimeStatusIdle     RuntimeStatus = "IDLE"
	RuntimeStatusFinished RuntimeStatus = "FINISHED"
)

// EngineStateSnapshot is a unified point-in-time snapshot of the full engine
// state: runtime state, factory lifecycle, session metrics, and uptime.
type EngineStateSnapshot[TMarking any, TTopology any] struct {
	RuntimeStatus      RuntimeStatus                `json:"runtime_status"`
	StreamGenerationID string                       `json:"stream_generation_id,omitempty"`
	Marking            TMarking                     `json:"marking"`
	Dispatches         map[string]*DispatchEntry    `json:"dispatches"`
	InFlightCount      int                          `json:"in_flight_count"`
	Results            []workerexecution.WorkResult `json:"results"`
	DispatchHistory    []CompletedDispatch          `json:"dispatch_history"`
	// ActiveThrottlePauses exposes active provider/model pause windows owned by
	// dispatcher policy for tests and observability reconstruction.
	ActiveThrottlePauses []ActiveThrottlePause `json:"active_throttle_pauses,omitempty"`
	TickCount            int                   `json:"tick_count"`

	// Factory lifecycle state.
	FactoryState string `json:"factory_state"`

	// LifecycleControlStatus is the canonical pause/resume lifecycle status
	// reconstructed from SESSION_PAUSED and SESSION_RESUMED events when present.
	LifecycleControlStatus string `json:"lifecycle_control_status,omitempty"`

	// Uptime since the factory started.
	Uptime time.Duration `json:"uptime"`

	// Topology is the workflow net used to interpret marking and dispatch
	// records for service-facing observability read models.
	Topology TTopology `json:"topology,omitempty"`

	// EnabledTransitions is computed by Factory Runtime before the snapshot
	// crosses a service boundary.
	EnabledTransitions []EnabledTransition `json:"enabled_transitions,omitempty"`
}

// RuntimeStateSnapshot returns the raw runtime portion of the aggregate
// snapshot for reducers that intentionally operate on runtime records.
func (s EngineStateSnapshot[TMarking, TTopology]) RuntimeStateSnapshot() EngineStateSnapshot[TMarking, TTopology] {
	return EngineStateSnapshot[TMarking, TTopology]{
		RuntimeStatus:        s.RuntimeStatus,
		StreamGenerationID:   s.StreamGenerationID,
		Marking:              s.Marking,
		Dispatches:           s.Dispatches,
		InFlightCount:        s.InFlightCount,
		Results:              s.Results,
		DispatchHistory:      s.DispatchHistory,
		EnabledTransitions:   s.EnabledTransitions,
		ActiveThrottlePauses: s.ActiveThrottlePauses,
		TickCount:            s.TickCount,
	}
}

// FactoryDispatchRecord stores a raw WorkDispatch plus token mutations held
// while the worker is in flight.
type FactoryDispatchRecord struct {
	DispatchID     string
	CreatedTick    int
	Dispatch       work.WorkDispatch
	HeldMutations  []MarkingMutation
	ConsumedTokens []string
	// HumanApproval marks a dispatch that is durably pending operator input.
	// Such a dispatch owns its consumed tokens but must never enter a Worker,
	// Provider, Model, script, runner, or capacity execution path.
	HumanApproval bool
}

// FactoryCompletionRecord stores a worker result at the logical tick where the
// engine observed it.
type FactoryCompletionRecord struct {
	CompletionID string
	DispatchID   string
	ObservedTick int
	Result       workerexecution.WorkResult
}

// SubmissionHookContext is the input passed to engine-owned submission hooks
// once per logical tick.
type SubmissionHookContext[TSnapshot any] struct {
	Snapshot          TSnapshot
	ContinuationState map[string]string
}

// SubmissionHookResult contains all due hook output observed by the engine at
// one logical tick.
type SubmissionHookResult struct {
	GeneratedBatches  []work.GeneratedSubmissionBatch
	Results           []workerexecution.WorkResult
	MarkingMutations  []MarkingMutation
	ContinuationState map[string]string
	KeepAlive         bool
}

// FactorySessionJavaScriptCheckpointRef is a customer-visible JavaScript checkpoint
// reference without raw VM checkpoint payload bodies.
type FactorySessionJavaScriptCheckpointRef struct {
	ID                 string                           `json:"id"`
	Label              string                           `json:"label"`
	Summary            string                           `json:"summary"`
	Timestamp          time.Time                        `json:"timestamp,omitempty"`
	ArtifactRef        *JavaScriptCheckpointArtifactRef `json:"artifactRef,omitempty"`
	ResumabilityStatus string                           `json:"resumabilityStatus,omitempty"`
	Warnings           []FactorySessionDispatchWarning  `json:"warnings,omitempty"`
}

// FactorySessionJavaScriptRuntimeState carries JavaScript orchestrator runtime
// projection fields for one factory session.
type FactorySessionJavaScriptRuntimeState struct {
	Phase               string                                  `json:"phase"`
	Phases              []string                                `json:"phases"`
	ArgsDigest          string                                  `json:"argsDigest"`
	Checkpoints         []FactorySessionJavaScriptCheckpointRef `json:"checkpoints"`
	ScriptStatus        string                                  `json:"scriptStatus"`
	QueuedDispatches    int                                     `json:"queuedDispatches"`
	RunningDispatches   int                                     `json:"runningDispatches"`
	CompletedDispatches int                                     `json:"completedDispatches"`
	Dispatches          []FactorySessionDispatchState           `json:"dispatches,omitempty"`
	Artifacts           []FactorySessionArtifactState           `json:"artifacts,omitempty"`
	PrimaryResult       []work.WorkContentPart                  `json:"primaryResult,omitempty"`
	ResultStatus        string                                  `json:"resultStatus,omitempty"`
}

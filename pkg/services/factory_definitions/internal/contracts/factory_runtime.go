package factorycontracts

import (
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	RequestID     string
	TraceID       string
	Status        InvocationTerminalStatus
	PrimaryResult []work.WorkContentPart
	ErrorCode     string
	Message       string
	SessionID     string
	WorkID        string
	WorkName      string
	WorkState     string
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
}

// DispatchEntry tracks an in-flight dispatch awaiting a worker result.
type DispatchEntry struct {
	DispatchID      string                  `json:"dispatch_id"`
	TransitionID    string                  `json:"transition_id"`
	WorkstationName string                  `json:"workstation_name,omitempty"`
	StartTime       time.Time               `json:"start_time"`
	ConsumedTokens  []workerexecution.Token `json:"consumed_tokens"`
	HeldMutations   []MarkingMutation       `json:"held_mutations"`
}

// CompletedDispatch records a dispatch that has finished, with timing data.
type CompletedDispatch struct {
	DispatchID                  string                                   `json:"dispatch_id"`
	TransitionID                string                                   `json:"transition_id"`
	WorkstationName             string                                   `json:"workstation_name,omitempty"`
	Outcome                     workerexecution.WorkOutcome              `json:"outcome"`
	SelectedClassificationLabel string                                   `json:"selected_classification_label,omitempty"`
	Reason                      string                                   `json:"reason,omitempty"`
	FailureMetadata             *workerexecution.WorkFailureMetadata     `json:"failure_metadata,omitempty"`
	ProviderSession             *workerexecution.ProviderSessionMetadata `json:"provider_session,omitempty"`
	StartTime                   time.Time                                `json:"start_time"`
	EndTime                     time.Time                                `json:"end_time"`
	Duration                    time.Duration                            `json:"duration"`
	ConsumedTokens              []workerexecution.Token                  `json:"consumed_tokens,omitempty"`
	OutputMutations             []TokenMutationRecord                    `json:"output_mutations,omitempty"`
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

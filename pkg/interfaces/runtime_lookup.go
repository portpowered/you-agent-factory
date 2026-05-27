package interfaces

import (
	"time"
)

// RuntimeWorkstationLookup resolves runtime workstation definitions by authored name.
type RuntimeWorkstationLookup interface {
	Workstation(name string) (*FactoryWorkstationConfig, bool)
}

// RuntimeDefinitionLookup resolves runtime worker and workstation definitions by authored name.
type RuntimeDefinitionLookup interface {
	RuntimeWorkstationLookup
	Worker(name string) (*WorkerConfig, bool)
}

// RuntimeConfigLookup exposes the canonical public runtime-facing lookup
// contract for consumers that need runtime definitions plus path-aware
// execution lookups.
type RuntimeConfigLookup interface {
	RuntimeDefinitionLookup
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
	Type           MutationType    `json:"type"`
	TokenID        string          `json:"token_id"`
	FromPlace      string          `json:"from_place"`
	ToPlace        string          `json:"to_place"`
	Reason         string          `json:"reason"`
	NewToken       *Token          `json:"-"`
	FailureRecords []FailureRecord `json:"-"`
}

// TokenMutationRecord stores the raw token mutation emitted while applying a
// worker result.
type TokenMutationRecord struct {
	DispatchID   string       `json:"dispatch_id"`
	TransitionID string       `json:"transition_id"`
	Outcome      WorkOutcome  `json:"outcome"`
	Type         MutationType `json:"type"`
	TokenID      string       `json:"token_id"`
	FromPlace    string       `json:"from_place"`
	ToPlace      string       `json:"to_place"`
	Reason       string       `json:"reason"`
	Token        *Token       `json:"token,omitempty"`
}

// DispatchEntry tracks an in-flight dispatch awaiting a worker result.
type DispatchEntry struct {
	DispatchID      string            `json:"dispatch_id"`
	TransitionID    string            `json:"transition_id"`
	WorkstationName string            `json:"workstation_name,omitempty"`
	StartTime       time.Time         `json:"start_time"`
	ConsumedTokens  []Token           `json:"consumed_tokens"`
	HeldMutations   []MarkingMutation `json:"held_mutations"`
}

// CompletedDispatch records a dispatch that has finished, with timing data.
type CompletedDispatch struct {
	DispatchID                  string                   `json:"dispatch_id"`
	TransitionID                string                   `json:"transition_id"`
	WorkstationName             string                   `json:"workstation_name,omitempty"`
	Outcome                     WorkOutcome              `json:"outcome"`
	SelectedClassificationLabel string                   `json:"selected_classification_label,omitempty"`
	Reason                      string                   `json:"reason,omitempty"`
	FailureMetadata             *WorkFailureMetadata     `json:"failure_metadata,omitempty"`
	ProviderFailure             *ProviderFailureMetadata `json:"provider_failure,omitempty"`
	ProviderSession             *ProviderSessionMetadata `json:"provider_session,omitempty"`
	StartTime                   time.Time                `json:"start_time"`
	EndTime                     time.Time                `json:"end_time"`
	Duration                    time.Duration            `json:"duration"`
	ConsumedTokens              []Token                  `json:"consumed_tokens,omitempty"`
	OutputMutations             []TokenMutationRecord    `json:"output_mutations,omitempty"`
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
	TransitionID string             `json:"transition_id"`
	WorkerType   string             `json:"worker_type"`
	Bindings     map[string][]Token `json:"bindings"`
	ArcModes     map[string]ArcMode `json:"arc_modes"`
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
	Mutations              []MarkingMutation          `json:"mutations,omitempty"`
	GeneratedBatches       []GeneratedSubmissionBatch `json:"generated_batches,omitempty"`
	Dispatches             []DispatchRecord           `json:"dispatches,omitempty"`
	Histories              []TokenHistory             `json:"histories,omitempty"`
	CompletedDispatches    []CompletedDispatch        `json:"completed_dispatches,omitempty"`
	ActiveThrottlePauses   []ActiveThrottlePause      `json:"active_throttle_pauses,omitempty"`
	ThrottlePausesObserved bool                       `json:"throttle_pauses_observed,omitempty"`
	ShouldTerminate        bool                       `json:"should_terminate,omitempty"`
}

// DispatchRecord pairs a WorkDispatch with the marking mutations consumed to fire it.
type DispatchRecord struct {
	Dispatch  WorkDispatch      `json:"dispatch"`
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
	RuntimeStatus   RuntimeStatus             `json:"runtime_status"`
	Marking         TMarking                  `json:"marking"`
	Dispatches      map[string]*DispatchEntry `json:"dispatches"`
	InFlightCount   int                       `json:"in_flight_count"`
	Results         []WorkResult              `json:"results"`
	DispatchHistory []CompletedDispatch       `json:"dispatch_history"`
	// ActiveThrottlePauses exposes active provider/model pause windows owned by
	// dispatcher policy for tests and observability reconstruction.
	ActiveThrottlePauses []ActiveThrottlePause `json:"active_throttle_pauses,omitempty"`
	TickCount            int                   `json:"tick_count"`

	// Factory lifecycle state.
	FactoryState string `json:"factory_state"`

	// Uptime since the factory started.
	Uptime time.Duration `json:"uptime"`

	// Topology is the workflow net used to interpret marking and dispatch
	// records for service-facing observability read models.
	Topology TTopology `json:"topology,omitempty"`
}

// RuntimeStateSnapshot returns the raw runtime portion of the aggregate
// snapshot for reducers that intentionally operate on runtime records.
func (s EngineStateSnapshot[TMarking, TTopology]) RuntimeStateSnapshot() EngineStateSnapshot[TMarking, TTopology] {
	var topology TTopology
	return EngineStateSnapshot[TMarking, TTopology]{
		RuntimeStatus:        s.RuntimeStatus,
		Marking:              s.Marking,
		Dispatches:           s.Dispatches,
		InFlightCount:        s.InFlightCount,
		Results:              s.Results,
		DispatchHistory:      s.DispatchHistory,
		ActiveThrottlePauses: s.ActiveThrottlePauses,
		TickCount:            s.TickCount,
		Topology:             topology,
	}
}

package interfaces

import "time"

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

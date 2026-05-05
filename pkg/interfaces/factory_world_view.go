package interfaces

import "time"

// FactoryWorldView is a presentation projection derived from
// FactoryWorldState. It intentionally does not reconstruct from runtime
// snapshots; callers must build the generic world state from canonical events
// first.
type FactoryWorldView struct {
	Topology FactoryWorldTopologyView `json:"topology"`
	Runtime  FactoryWorldRuntimeView  `json:"runtime"`
}

// FactoryWorldTopologyView contains stable graph structure for UI rendering.
type FactoryWorldTopologyView struct {
	SubmitWorkTypes      []FactoryWorldSubmitWorkType           `json:"submit_work_types,omitempty"`
	WorkstationNodeIDs   []string                               `json:"workstation_node_ids,omitempty"`
	WorkstationNodesByID map[string]FactoryWorldWorkstationNode `json:"workstation_nodes_by_id,omitempty"`
	Edges                []FactoryWorldWorkstationEdge          `json:"edges,omitempty"`
}

type FactoryWorldSubmitWorkType struct {
	WorkTypeName string `json:"work_type_name"`
}

type FactoryWorldWorkstationNode struct {
	NodeID            string                 `json:"node_id"`
	TransitionID      string                 `json:"transition_id"`
	WorkstationName   string                 `json:"workstation_name,omitempty"`
	WorkerType        string                 `json:"worker_type,omitempty"`
	WorkstationKind   string                 `json:"workstation_kind,omitempty"`
	InputPlaces       []FactoryWorldPlaceRef `json:"input_places,omitempty"`
	OutputPlaces      []FactoryWorldPlaceRef `json:"output_places,omitempty"`
	InputPlaceIDs     []string               `json:"input_place_ids,omitempty"`
	OutputPlaceIDs    []string               `json:"output_place_ids,omitempty"`
	InputWorkTypeIDs  []string               `json:"input_work_type_ids,omitempty"`
	OutputWorkTypeIDs []string               `json:"output_work_type_ids,omitempty"`
}

type FactoryWorldPlaceRef struct {
	PlaceID       string `json:"place_id"`
	TypeID        string `json:"type_id,omitempty"`
	StateValue    string `json:"state_value,omitempty"`
	Kind          string `json:"kind"`
	StateCategory string `json:"state_category,omitempty"`
}

type FactoryWorldWorkstationEdge struct {
	EdgeID        string `json:"edge_id"`
	FromNodeID    string `json:"from_node_id"`
	ToNodeID      string `json:"to_node_id"`
	ViaPlaceID    string `json:"via_place_id"`
	WorkTypeID    string `json:"work_type_id,omitempty"`
	StateValue    string `json:"state_value,omitempty"`
	StateCategory string `json:"state_category,omitempty"`
	OutcomeKind   string `json:"outcome_kind,omitempty"`
}

type FactoryWorldRuntimeView struct {
	InFlightDispatchCount            int                                                `json:"in_flight_dispatch_count"`
	ActiveDispatchIDs                []string                                           `json:"active_dispatch_ids,omitempty"`
	ActiveExecutionsByDispatchID     map[string]FactoryWorldActiveExecution             `json:"active_executions_by_dispatch_id,omitempty"`
	ActiveWorkstationNodeIDs         []string                                           `json:"active_workstation_node_ids,omitempty"`
	InferenceAttemptsByDispatchID    map[string]map[string]FactoryWorldInferenceAttempt `json:"inference_attempts_by_dispatch_id,omitempty"`
	WorkstationActivityByNodeID      map[string]FactoryWorldActivity                    `json:"workstation_activity_by_node_id,omitempty"`
	PlaceTokenCounts                 map[string]int                                     `json:"place_token_counts,omitempty"`
	CurrentWorkItemsByPlaceID        map[string][]FactoryWorldWorkItemRef               `json:"current_work_items_by_place_id,omitempty"`
	PlaceOccupancyWorkItemsByPlaceID map[string][]FactoryWorldWorkItemRef               `json:"place_occupancy_work_items_by_place_id,omitempty"`
	ActiveThrottlePauses             []FactoryWorldThrottlePause                        `json:"active_throttle_pauses,omitempty"`
	Session                          FactoryWorldSessionRuntime                         `json:"session"`
}

type FactoryWorldThrottlePause struct {
	LaneID                   string    `json:"lane_id"`
	Provider                 string    `json:"provider"`
	Model                    string    `json:"model"`
	PausedAt                 time.Time `json:"paused_at,omitempty"`
	PausedUntil              time.Time `json:"paused_until"`
	RecoverAt                time.Time `json:"recover_at"`
	AffectedTransitionIDs    []string  `json:"affected_transition_ids,omitempty"`
	AffectedWorkstationNames []string  `json:"affected_workstation_names,omitempty"`
	AffectedWorkerTypes      []string  `json:"affected_worker_types,omitempty"`
	AffectedWorkTypeIDs      []string  `json:"affected_work_type_ids,omitempty"`
}

type FactoryWorldActiveExecution struct {
	DispatchID               string                    `json:"dispatch_id"`
	WorkstationNodeID        string                    `json:"workstation_node_id"`
	TransitionID             string                    `json:"transition_id"`
	WorkstationName          string                    `json:"workstation_name,omitempty"`
	StartedAt                time.Time                 `json:"started_at"`
	WorkTypeIDs              []string                  `json:"work_type_ids,omitempty"`
	WorkItems                []FactoryWorldWorkItemRef `json:"work_items,omitempty"`
	CurrentChainingTraceID   string                    `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string                  `json:"previous_chaining_trace_ids,omitempty"`
	TraceIDs                 []string                  `json:"trace_ids,omitempty"`
	ConsumedInputs           []WorkstationInput        `json:"consumed_inputs,omitempty"`
}

type FactoryWorldActivity struct {
	WorkstationNodeID string                    `json:"workstation_node_id"`
	ActiveDispatchIDs []string                  `json:"active_dispatch_ids,omitempty"`
	ActiveWorkItems   []FactoryWorldWorkItemRef `json:"active_work_items,omitempty"`
	TraceIDs          []string                  `json:"trace_ids,omitempty"`
}

type FactoryWorldWorkItemRef struct {
	WorkID                   string   `json:"work_id"`
	WorkTypeID               string   `json:"work_type_id,omitempty"`
	DisplayName              string   `json:"display_name,omitempty"`
	ChainingTraceDepth       int      `json:"chaining_trace_depth,omitempty"`
	CurrentChainingTraceID   string   `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string `json:"previous_chaining_trace_ids,omitempty"`
	TraceID                  string   `json:"trace_id,omitempty"`
}

type FactoryWorldSessionRuntime struct {
	HasData              bool                                `json:"has_data"`
	DispatchedCount      int                                 `json:"dispatched_count"`
	CompletedCount       int                                 `json:"completed_count"`
	FailedCount          int                                 `json:"failed_count"`
	DispatchHistory      []FactoryWorldDispatchCompletion    `json:"dispatch_history,omitempty"`
	ProviderSessions     []FactoryWorldProviderSessionRecord `json:"provider_sessions,omitempty"`
	DispatchedByWorkType map[string]int                      `json:"dispatched_by_work_type,omitempty"`
	CompletedByWorkType  map[string]int                      `json:"completed_by_work_type,omitempty"`
	FailedByWorkType     map[string]int                      `json:"failed_by_work_type,omitempty"`
}

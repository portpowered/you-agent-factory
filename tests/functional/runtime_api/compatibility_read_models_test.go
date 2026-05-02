package runtime_api_test

import "time"

type DashboardResponse struct {
	FactoryState  string            `json:"factory_state"`
	Resources     *[]ResourceUsage  `json:"resources,omitempty"`
	Runtime       DashboardRuntime  `json:"runtime"`
	RuntimeStatus string            `json:"runtime_status"`
	TickCount     int               `json:"tick_count"`
	Topology      DashboardTopology `json:"topology"`
	UptimeSeconds int64             `json:"uptime_seconds"`
}

type DashboardRuntime struct {
	ActiveDispatchIds             *[]string                                `json:"active_dispatch_ids,omitempty"`
	ActiveExecutionsByDispatchId  *map[string]DashboardActiveExecution     `json:"active_executions_by_dispatch_id,omitempty"`
	ActiveThrottlePauses          *[]DashboardThrottlePause                `json:"active_throttle_pauses,omitempty"`
	ActiveWorkstationNodeIds      *[]string                                `json:"active_workstation_node_ids,omitempty"`
	CurrentWorkItemsByPlaceId     *map[string][]DashboardWorkItemRef       `json:"current_work_items_by_place_id,omitempty"`
	InFlightDispatchCount         int                                      `json:"in_flight_dispatch_count"`
	InferenceAttemptsByDispatchId *map[string]map[string]InferenceAttempt  `json:"inference_attempts_by_dispatch_id,omitempty"`
	PlaceTokenCounts              *IntegerMap                              `json:"place_token_counts,omitempty"`
	Session                       DashboardSessionRuntime                  `json:"session"`
	WorkstationActivityByNodeId   *map[string]DashboardWorkstationActivity `json:"workstation_activity_by_node_id,omitempty"`
}

type InferenceAttempt struct {
	Attempt            int    `json:"attempt"`
	DispatchId         string `json:"dispatch_id"`
	DurationMillis     int64  `json:"duration_millis,omitempty"`
	ErrorClass         string `json:"error_class,omitempty"`
	ExitCode           *int   `json:"exit_code,omitempty"`
	InferenceRequestId string `json:"inference_request_id"`
	Outcome            string `json:"outcome,omitempty"`
	Prompt             string `json:"prompt"`
	RequestTime        string `json:"request_time"`
	Response           string `json:"response,omitempty"`
	ResponseTime       string `json:"response_time,omitempty"`
	TransitionId       string `json:"transition_id"`
	WorkingDirectory   string `json:"working_directory,omitempty"`
	Worktree           string `json:"worktree,omitempty"`
}

type DashboardTopology struct {
	Edges                *[]DashboardWorkstationEdge          `json:"edges,omitempty"`
	WorkstationNodeIds   *[]string                            `json:"workstation_node_ids,omitempty"`
	WorkstationNodesById *map[string]DashboardWorkstationNode `json:"workstation_nodes_by_id,omitempty"`
}

type DashboardActiveExecution struct {
	ConsumedTokens    *[]TraceTokenView       `json:"consumed_tokens,omitempty"`
	DispatchId        string                  `json:"dispatch_id"`
	OutputMutations   *[]TraceMutationView    `json:"output_mutations,omitempty"`
	StartedAt         time.Time               `json:"started_at"`
	TraceIds          *[]string               `json:"trace_ids,omitempty"`
	TransitionId      string                  `json:"transition_id"`
	WorkItems         *[]DashboardWorkItemRef `json:"work_items,omitempty"`
	WorkTypeIds       *[]string               `json:"work_type_ids,omitempty"`
	WorkstationName   *string                 `json:"workstation_name,omitempty"`
	WorkstationNodeId string                  `json:"workstation_node_id"`
}

type DashboardWorkItemRef struct {
	DisplayName *string `json:"display_name,omitempty"`
	TraceId     *string `json:"trace_id,omitempty"`
	WorkId      string  `json:"work_id"`
	WorkTypeId  *string `json:"work_type_id,omitempty"`
}

type DashboardWorkstationActivity struct {
	ActiveDispatchIds *[]string               `json:"active_dispatch_ids,omitempty"`
	ActiveWorkItems   *[]DashboardWorkItemRef `json:"active_work_items,omitempty"`
	TraceIds          *[]string               `json:"trace_ids,omitempty"`
	WorkstationNodeId string                  `json:"workstation_node_id"`
}

type DashboardWorkstationEdge struct {
	EdgeId        string  `json:"edge_id"`
	FromNodeId    string  `json:"from_node_id"`
	OutcomeKind   *string `json:"outcome_kind,omitempty"`
	StateCategory *string `json:"state_category,omitempty"`
	StateValue    *string `json:"state_value,omitempty"`
	ToNodeId      string  `json:"to_node_id"`
	ViaPlaceId    string  `json:"via_place_id"`
	WorkTypeId    *string `json:"work_type_id,omitempty"`
}

type DashboardWorkstationNode struct {
	InputPlaceIds     *[]string            `json:"input_place_ids,omitempty"`
	InputPlaces       *[]DashboardPlaceRef `json:"input_places,omitempty"`
	InputWorkTypeIds  *[]string            `json:"input_work_type_ids,omitempty"`
	NodeId            string               `json:"node_id"`
	OutputPlaceIds    *[]string            `json:"output_place_ids,omitempty"`
	OutputPlaces      *[]DashboardPlaceRef `json:"output_places,omitempty"`
	OutputWorkTypeIds *[]string            `json:"output_work_type_ids,omitempty"`
	TransitionId      string               `json:"transition_id"`
	WorkerType        *string              `json:"worker_type,omitempty"`
	WorkstationKind   *string              `json:"workstation_kind,omitempty"`
	WorkstationName   *string              `json:"workstation_name,omitempty"`
}

type DashboardPlaceRef struct {
	Kind          string  `json:"kind"`
	PlaceId       string  `json:"place_id"`
	StateCategory *string `json:"state_category,omitempty"`
	StateValue    *string `json:"state_value,omitempty"`
	TypeId        *string `json:"type_id,omitempty"`
}

type DashboardThrottlePause struct {
	AffectedTransitionIds    *[]string  `json:"affected_transition_ids,omitempty"`
	AffectedWorkTypeIds      *[]string  `json:"affected_work_type_ids,omitempty"`
	AffectedWorkerTypes      *[]string  `json:"affected_worker_types,omitempty"`
	AffectedWorkstationNames *[]string  `json:"affected_workstation_names,omitempty"`
	LaneId                   string     `json:"lane_id"`
	Model                    string     `json:"model"`
	PausedAt                 *time.Time `json:"paused_at,omitempty"`
	PausedUntil              time.Time  `json:"paused_until"`
	Provider                 string     `json:"provider"`
	RecoverAt                time.Time  `json:"recover_at"`
}

type DashboardSessionRuntime struct {
	CompletedByWorkType  *IntegerMap               `json:"completed_by_work_type,omitempty"`
	CompletedCount       int                       `json:"completed_count"`
	CompletedWorkLabels  *[]string                 `json:"completed_work_labels,omitempty"`
	DispatchHistory      *[]DashboardDispatchView  `json:"dispatch_history,omitempty"`
	DispatchedByWorkType *IntegerMap               `json:"dispatched_by_work_type,omitempty"`
	DispatchedCount      int                       `json:"dispatched_count"`
	FailedByWorkType     *IntegerMap               `json:"failed_by_work_type,omitempty"`
	FailedCount          int                       `json:"failed_count"`
	FailedWorkLabels     *[]string                 `json:"failed_work_labels,omitempty"`
	HasData              bool                      `json:"has_data"`
	ProviderSessions     *[]ProviderSessionAttempt `json:"provider_sessions,omitempty"`
}

type DashboardDispatchView struct {
	ConsumedTokens  *[]TraceTokenView        `json:"consumed_tokens,omitempty"`
	DispatchId      string                   `json:"dispatch_id"`
	DurationMillis  int64                    `json:"duration_millis"`
	EndTime         string                   `json:"end_time"`
	Outcome         string                   `json:"outcome"`
	OutputMutations *[]TraceMutationView     `json:"output_mutations,omitempty"`
	ProviderSession *ProviderSessionMetadata `json:"provider_session,omitempty"`
	StartedAt       string                   `json:"started_at"`
	TraceIds        *[]string                `json:"trace_ids,omitempty"`
	TransitionId    string                   `json:"transition_id"`
	WorkItems       *[]DashboardWorkItemRef  `json:"work_items,omitempty"`
	WorkTypeIds     *[]string                `json:"work_type_ids,omitempty"`
	WorkstationName *string                  `json:"workstation_name,omitempty"`
}

type ProviderSessionAttempt struct {
	ConsumedTokens  *[]TraceTokenView        `json:"consumed_tokens,omitempty"`
	DispatchId      string                   `json:"dispatch_id"`
	Outcome         string                   `json:"outcome"`
	OutputMutations *[]TraceMutationView     `json:"output_mutations,omitempty"`
	ProviderSession *ProviderSessionMetadata `json:"provider_session,omitempty"`
	TransitionId    string                   `json:"transition_id"`
	WorkItems       *[]DashboardWorkItemRef  `json:"work_items,omitempty"`
	WorkstationName *string                  `json:"workstation_name,omitempty"`
}

type ProviderSessionMetadata struct {
	Id       *string `json:"id,omitempty"`
	Kind     *string `json:"kind,omitempty"`
	Provider *string `json:"provider,omitempty"`
}

type TraceTokenView struct {
	CreatedAt  string     `json:"created_at"`
	EnteredAt  string     `json:"entered_at"`
	Name       *string    `json:"name,omitempty"`
	PlaceId    string     `json:"place_id"`
	Tags       *StringMap `json:"tags,omitempty"`
	TokenId    string     `json:"token_id"`
	TraceId    *string    `json:"trace_id,omitempty"`
	WorkId     string     `json:"work_id"`
	WorkTypeId string     `json:"work_type_id"`
}

type TraceMutationView struct {
	FromPlace      *string         `json:"from_place,omitempty"`
	Reason         *string         `json:"reason,omitempty"`
	ResultingToken *TraceTokenView `json:"resulting_token,omitempty"`
	ToPlace        *string         `json:"to_place,omitempty"`
	TokenId        string          `json:"token_id"`
	Type           string          `json:"type"`
}

type StateResponse struct {
	Categories    StateCategories  `json:"categories"`
	FactoryState  string           `json:"factory_state"`
	Resources     *[]ResourceUsage `json:"resources,omitempty"`
	RuntimeStatus string           `json:"runtime_status"`
	TotalTokens   int              `json:"total_tokens"`
}

type StateCategories struct {
	Failed     int `json:"failed"`
	Initial    int `json:"initial"`
	Processing int `json:"processing"`
	Terminal   int `json:"terminal"`
}

type ResourceUsage struct {
	Available int    `json:"available"`
	Name      string `json:"name"`
	Total     int    `json:"total"`
}

type IntegerMap map[string]int
type StringMap map[string]string

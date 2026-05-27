package interfaces

import (
	"time"
)

// FactoryWorldState is the dashboard-agnostic reconstruction of factory state
// from canonical factory events up to one selected engine tick.
type FactoryWorldState struct {
	Tick                          int                                                `json:"tick"`
	EventTime                     time.Time                                          `json:"event_time,omitempty"`
	Topology                      InitialStructurePayload                            `json:"topology"`
	PayloadLineage                WorkPayloadLineageProjection                       `json:"payload_lineage,omitempty"`
	WorkRequestsByID              map[string]WorkRequestPayload                      `json:"work_requests_by_id,omitempty"`
	RelationsByWorkID             map[string][]FactoryRelation                       `json:"relations_by_work_id,omitempty"`
	WorkItemsByID                 map[string]FactoryWorkItem                         `json:"work_items_by_id,omitempty"`
	ActiveWorkItemsByID           map[string]FactoryWorkItem                         `json:"active_work_items_by_id,omitempty"`
	TerminalWorkByID              map[string]FactoryTerminalWork                     `json:"terminal_work_by_id,omitempty"`
	FailedWorkItemsByID           map[string]FactoryWorkItem                         `json:"failed_work_items_by_id,omitempty"`
	PlaceOccupancyByID            map[string]FactoryPlaceOccupancy                   `json:"place_occupancy_by_id,omitempty"`
	ActiveDispatches              map[string]FactoryWorldDispatch                    `json:"active_dispatches,omitempty"`
	CompletedDispatches           []FactoryWorldDispatchCompletion                   `json:"completed_dispatches,omitempty"`
	FailedDispatches              []FactoryWorldDispatchCompletion                   `json:"failed_dispatches,omitempty"`
	FailureDetailsByWorkID        map[string]FactoryWorldFailureDetail               `json:"failure_details_by_work_id,omitempty"`
	InferenceAttemptsByDispatchID map[string]map[string]FactoryWorldInferenceAttempt `json:"inference_attempts_by_dispatch_id,omitempty"`
	ScriptRequestsByDispatchID    map[string]map[string]FactoryWorldScriptRequest    `json:"script_requests_by_dispatch_id,omitempty"`
	ScriptResponsesByDispatchID   map[string]map[string]FactoryWorldScriptResponse   `json:"script_responses_by_dispatch_id,omitempty"`
	TracesByID                    map[string]FactoryWorldTrace                       `json:"traces_by_id,omitempty"`
	ProviderSessions              []FactoryWorldProviderSessionRecord                `json:"provider_sessions,omitempty"`
	FactoryState                  string                                             `json:"factory_state,omitempty"`
	FactoryStateReason            string                                             `json:"factory_state_reason,omitempty"`
	FactoryStatePrevious          string                                             `json:"factory_state_previous,omitempty"`
}

// FactoryPlaceOccupancy describes work and resource tokens reconstructed at a
// place for the selected tick.
type FactoryPlaceOccupancy struct {
	PlaceID          string   `json:"place_id"`
	WorkItemIDs      []string `json:"work_item_ids,omitempty"`
	ResourceTokenIDs []string `json:"resource_token_ids,omitempty"`
	TokenCount       int      `json:"token_count"`
}

// FactoryWorldDispatch describes a workstation request that has not yet
// received its matching response at the selected tick.
type FactoryWorldDispatch struct {
	DispatchID               string                `json:"dispatch_id"`
	TransitionID             string                `json:"transition_id"`
	Workstation              FactoryWorkstationRef `json:"workstation"`
	RunnerID                 string                `json:"runner_id,omitempty"`
	RunnerSelectionSource    RunnerSelectionSource `json:"runner_selection_source,omitempty"`
	Provider                 string                `json:"provider,omitempty"`
	Model                    string                `json:"model,omitempty"`
	StartedTick              int                   `json:"started_tick"`
	StartedAt                time.Time             `json:"started_at,omitempty"`
	Inputs                   []WorkstationInput    `json:"inputs,omitempty"`
	Resources                []FactoryResourceUnit `json:"resources,omitempty"`
	WorkItemIDs              []string              `json:"work_item_ids,omitempty"`
	CurrentChainingTraceID   string                `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string              `json:"previous_chaining_trace_ids,omitempty"`
	TraceIDs                 []string              `json:"trace_ids,omitempty"`
}

// FactoryWorldDispatchCompletion describes a finished dispatch reconstructed
// from a workstation response event.
type FactoryWorldDispatchCompletion struct {
	DispatchID               string                   `json:"dispatch_id"`
	TransitionID             string                   `json:"transition_id"`
	Workstation              FactoryWorkstationRef    `json:"workstation"`
	RunnerID                 string                   `json:"runner_id,omitempty"`
	RunnerSelectionSource    RunnerSelectionSource    `json:"runner_selection_source,omitempty"`
	StartedTick              int                      `json:"started_tick,omitempty"`
	CompletedTick            int                      `json:"completed_tick"`
	StartedAt                time.Time                `json:"started_at,omitempty"`
	CompletedAt              time.Time                `json:"completed_at,omitempty"`
	DurationMillis           int64                    `json:"duration_millis"`
	Result                   WorkstationResult        `json:"result"`
	WorkItemIDs              []string                 `json:"work_item_ids,omitempty"`
	ConsumedInputs           []WorkstationInput       `json:"consumed_inputs,omitempty"`
	InputWorkItems           []FactoryWorkItem        `json:"input_work_items,omitempty"`
	OutputWorkItems          []FactoryWorkItem        `json:"output_work_items,omitempty"`
	CurrentChainingTraceID   string                   `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string                 `json:"previous_chaining_trace_ids,omitempty"`
	TraceIDs                 []string                 `json:"trace_ids,omitempty"`
	ProviderSession          *ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics              *SafeWorkDiagnostics     `json:"diagnostics,omitempty"`
	TerminalWork             *FactoryTerminalWork     `json:"terminal_work,omitempty"`
}

// FactoryWorldTrace groups work and dispatch activity by trace identifier.
type FactoryWorldTrace struct {
	TraceID       string   `json:"trace_id"`
	WorkItemIDs   []string `json:"work_item_ids,omitempty"`
	DispatchIDs   []string `json:"dispatch_ids,omitempty"`
	TerminalWork  []string `json:"terminal_work,omitempty"`
	FailedWorkIDs []string `json:"failed_work_ids,omitempty"`
}

// FactoryWorldProviderSessionRecord records one provider session attached to a
// workstation response in the canonical event-first world state.
type FactoryWorldProviderSessionRecord struct {
	DispatchID               string                  `json:"dispatch_id"`
	TransitionID             string                  `json:"transition_id"`
	WorkstationName          string                  `json:"workstation_name,omitempty"`
	RunnerID                 string                  `json:"runner_id,omitempty"`
	RunnerSelectionSource    RunnerSelectionSource   `json:"runner_selection_source,omitempty"`
	Outcome                  string                  `json:"outcome"`
	ProviderSession          ProviderSessionMetadata `json:"provider_session"`
	Diagnostics              *SafeWorkDiagnostics    `json:"diagnostics,omitempty"`
	WorkItemIDs              []string                `json:"work_item_ids,omitempty"`
	ConsumedInputs           []WorkstationInput      `json:"consumed_inputs,omitempty"`
	CurrentChainingTraceID   string                  `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string                `json:"previous_chaining_trace_ids,omitempty"`
	TraceIDs                 []string                `json:"trace_ids,omitempty"`
	FailureReason            string                  `json:"failure_reason,omitempty"`
	FailureMessage           string                  `json:"failure_message,omitempty"`
}

// FactoryWorldInferenceAttempt records one provider-boundary inference attempt
// reconstructed from canonical inference request and response events.
type FactoryWorldInferenceAttempt struct {
	DispatchID         string                   `json:"dispatch_id"`
	TransitionID       string                   `json:"transition_id"`
	InferenceRequestID string                   `json:"inference_request_id"`
	Attempt            int                      `json:"attempt"`
	WorkingDirectory   string                   `json:"working_directory,omitempty"`
	Worktree           string                   `json:"worktree,omitempty"`
	Prompt             string                   `json:"prompt"`
	RequestTime        time.Time                `json:"request_time,omitempty"`
	Outcome            string                   `json:"outcome,omitempty"`
	Response           string                   `json:"response,omitempty"`
	DurationMillis     int64                    `json:"duration_millis,omitempty"`
	ExitCode           *int                     `json:"exit_code,omitempty"`
	ErrorClass         string                   `json:"error_class,omitempty"`
	ProviderSession    *ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics        *SafeWorkDiagnostics     `json:"diagnostics,omitempty"`
	ResponseTime       time.Time                `json:"response_time,omitempty"`
}

// FactoryWorldScriptRequest records one script-boundary request reconstructed
// from canonical script request events.
type FactoryWorldScriptRequest struct {
	DispatchID      string    `json:"dispatch_id"`
	TransitionID    string    `json:"transition_id"`
	ScriptRequestID string    `json:"script_request_id"`
	Attempt         int       `json:"attempt"`
	Command         string    `json:"command"`
	Args            []string  `json:"args,omitempty"`
	RequestTime     time.Time `json:"request_time,omitempty"`
}

// FactoryWorldScriptResponse records one script-boundary response
// reconstructed from canonical script response events.
type FactoryWorldScriptResponse struct {
	DispatchID      string    `json:"dispatch_id"`
	TransitionID    string    `json:"transition_id"`
	ScriptRequestID string    `json:"script_request_id"`
	Attempt         int       `json:"attempt"`
	Outcome         string    `json:"outcome,omitempty"`
	Stdout          string    `json:"stdout,omitempty"`
	Stderr          string    `json:"stderr,omitempty"`
	DurationMillis  int64     `json:"duration_millis,omitempty"`
	ExitCode        *int      `json:"exit_code,omitempty"`
	FailureType     string    `json:"failure_type,omitempty"`
	ResponseTime    time.Time `json:"response_time,omitempty"`
}

// FactoryWorldFailureDetail associates failed terminal work with the dispatch
// completion that produced the failure.
type FactoryWorldFailureDetail struct {
	DispatchID      string          `json:"dispatch_id"`
	TransitionID    string          `json:"transition_id"`
	WorkstationName string          `json:"workstation_name,omitempty"`
	WorkItem        FactoryWorkItem `json:"work_item"`
	FailureReason   string          `json:"failure_reason,omitempty"`
	FailureMessage  string          `json:"failure_message,omitempty"`
}

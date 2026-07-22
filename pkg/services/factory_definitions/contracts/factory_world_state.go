package factorycontracts

import (
	"encoding/json"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// FactoryWorldState is the dashboard-agnostic reconstruction of factory state
// from canonical factory events up to one selected engine tick.
type FactoryWorldState struct {
	Tick                          int                                                `json:"tick"`
	EventTime                     time.Time                                          `json:"event_time,omitempty"`
	Factory                       *FactorySnapshot                                   `json:"factory,omitempty"`
	Topology                      InitialStructurePayload                            `json:"topology"`
	PayloadLineage                work.WorkPayloadLineageProjection                  `json:"payload_lineage,omitempty"`
	WorkRequestsByID              map[string]WorkRequestPayload                      `json:"work_requests_by_id,omitempty"`
	RelationsByWorkID             map[string][]work.FactoryRelation                  `json:"relations_by_work_id,omitempty"`
	WorkItemsByID                 map[string]work.FactoryWorkItem                    `json:"work_items_by_id,omitempty"`
	ActiveWorkItemsByID           map[string]work.FactoryWorkItem                    `json:"active_work_items_by_id,omitempty"`
	TerminalWorkByID              map[string]FactoryTerminalWork                     `json:"terminal_work_by_id,omitempty"`
	FailedWorkItemsByID           map[string]work.FactoryWorkItem                    `json:"failed_work_items_by_id,omitempty"`
	PlaceOccupancyByID            map[string]FactoryPlaceOccupancy                   `json:"place_occupancy_by_id,omitempty"`
	ActiveDispatches              map[string]FactoryWorldDispatch                    `json:"active_dispatches,omitempty"`
	CompletedDispatches           []FactoryWorldDispatchCompletion                   `json:"completed_dispatches,omitempty"`
	FailedDispatches              []FactoryWorldDispatchCompletion                   `json:"failed_dispatches,omitempty"`
	FailureDetailsByWorkID        map[string]FactoryWorldFailureDetail               `json:"failure_details_by_work_id,omitempty"`
	InferenceAttemptsByDispatchID map[string]map[string]FactoryWorldInferenceAttempt `json:"inference_attempts_by_dispatch_id,omitempty"`
	ScriptRequestsByDispatchID    map[string]map[string]FactoryWorldScriptRequest    `json:"script_requests_by_dispatch_id,omitempty"`
	ScriptResponsesByDispatchID   map[string]map[string]FactoryWorldScriptResponse   `json:"script_responses_by_dispatch_id,omitempty"`
	AgentRunResponsesByDispatchID map[string]map[string]FactoryWorldAgentRunResponse `json:"agent_run_responses_by_dispatch_id,omitempty"`
	TracesByID                    map[string]FactoryWorldTrace                       `json:"traces_by_id,omitempty"`
	ProviderSessions              []FactoryWorldProviderSessionRecord                `json:"provider_sessions,omitempty"`
	WorkStateChangesByWorkID      map[string][]FactoryWorldWorkStateChangeRecord     `json:"work_state_changes_by_work_id,omitempty"`
	FactoryState                  string                                             `json:"factory_state,omitempty"`
	FactoryStateReason            string                                             `json:"factory_state_reason,omitempty"`
	FactoryStatePrevious          string                                             `json:"factory_state_previous,omitempty"`
	JavaScriptCheckpoints         []FactorySessionJavaScriptCheckpointRef            `json:"javascript_checkpoints,omitempty"`
	JavaScriptRuntime             *FactorySessionJavaScriptRuntimeState              `json:"javascript_runtime,omitempty"`
	Artifacts                     []FactorySessionArtifactState                      `json:"artifacts,omitempty"`
	SessionBracket                *FactoryWorldSessionBracketState                   `json:"session_bracket,omitempty"`
}

// InvocationWorldState exposes only the Work-owned projection required for
// invocation return-policy evaluation.
func (s FactoryWorldState) InvocationWorldState() work.InvocationWorldState {
	requests := make(map[string]work.InvocationWorkRequest, len(s.WorkRequestsByID))
	for id, request := range s.WorkRequestsByID {
		requests[id] = work.InvocationWorkRequest{WorkItems: request.WorkItems, TraceID: request.TraceID}
	}
	terminal := make(map[string]work.InvocationTerminalWork, len(s.TerminalWorkByID))
	for id, item := range s.TerminalWorkByID {
		terminal[id] = work.InvocationTerminalWork{WorkItem: item.WorkItem, Status: item.Status}
	}
	changes := make(map[string][]work.InvocationWorkStateChange, len(s.WorkStateChangesByWorkID))
	for id, records := range s.WorkStateChangesByWorkID {
		mapped := make([]work.InvocationWorkStateChange, len(records))
		for index, record := range records {
			mapped[index] = work.InvocationWorkStateChange{
				WorkID: record.WorkID, WorkTypeName: record.WorkTypeName,
				ToState: record.ToState, ToPlaceID: record.ToPlaceID, RequestID: record.RequestID,
			}
		}
		changes[id] = mapped
	}
	var runtime *work.InvocationJavaScriptRuntime
	if s.JavaScriptRuntime != nil {
		dispatches := make([]work.InvocationDispatchState, len(s.JavaScriptRuntime.Dispatches))
		for index, dispatch := range s.JavaScriptRuntime.Dispatches {
			dispatches[index] = work.InvocationDispatchState{
				ID: dispatch.ID, Status: dispatch.Status,
				RelatedWorkIDs: append([]string(nil), dispatch.RelatedWorkIDs...),
			}
		}
		runtime = &work.InvocationJavaScriptRuntime{Dispatches: dispatches}
	}
	var bracket *work.InvocationSessionBracket
	if s.SessionBracket != nil {
		failureReason := ""
		if s.SessionBracket.FailureDetail != nil {
			failureReason = string(s.SessionBracket.FailureDetail.Reason)
		}
		bracket = &work.InvocationSessionBracket{
			SessionID: s.SessionBracket.SessionID, FinalStatus: s.SessionBracket.FinalStatus,
			LifecycleControlStatus: s.SessionBracket.LifecycleControlStatus,
			FailureReason:          failureReason,
		}
	}
	return work.InvocationWorldState{
		PayloadLineage: s.PayloadLineage, WorkRequestsByID: requests,
		WorkItemsByID: s.WorkItemsByID, FailedWorkItemsByID: s.FailedWorkItemsByID,
		TerminalWorkByID: terminal, WorkStateChangesByWorkID: changes,
		FactoryState: s.FactoryState, JavaScriptRuntime: runtime, SessionBracket: bracket,
	}
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
	DispatchID               string                                `json:"dispatch_id"`
	TransitionID             string                                `json:"transition_id"`
	Workstation              FactoryWorkstationRef                 `json:"workstation"`
	RunnerID                 string                                `json:"runner_id,omitempty"`
	RunnerSelectionSource    workerexecution.RunnerSelectionSource `json:"runner_selection_source,omitempty"`
	Provider                 string                                `json:"provider,omitempty"`
	Model                    string                                `json:"model,omitempty"`
	StartedTick              int                                   `json:"started_tick"`
	StartedAt                time.Time                             `json:"started_at,omitempty"`
	Inputs                   []WorkstationInput                    `json:"inputs,omitempty"`
	Resources                []FactoryResourceUnit                 `json:"resources,omitempty"`
	WorkItemIDs              []string                              `json:"work_item_ids,omitempty"`
	CurrentChainingTraceID   string                                `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string                              `json:"previous_chaining_trace_ids,omitempty"`
	TraceIDs                 []string                              `json:"trace_ids,omitempty"`
}

// FactoryWorldDispatchCompletion describes a finished dispatch reconstructed
// from a workstation response event.
type FactoryWorldDispatchCompletion struct {
	DispatchID               string                                   `json:"dispatch_id"`
	TransitionID             string                                   `json:"transition_id"`
	Workstation              FactoryWorkstationRef                    `json:"workstation"`
	RunnerID                 string                                   `json:"runner_id,omitempty"`
	RunnerSelectionSource    workerexecution.RunnerSelectionSource    `json:"runner_selection_source,omitempty"`
	StartedTick              int                                      `json:"started_tick,omitempty"`
	CompletedTick            int                                      `json:"completed_tick"`
	StartedAt                time.Time                                `json:"started_at,omitempty"`
	CompletedAt              time.Time                                `json:"completed_at,omitempty"`
	DurationMillis           int64                                    `json:"duration_millis"`
	Result                   WorkstationResult                        `json:"result"`
	WorkItemIDs              []string                                 `json:"work_item_ids,omitempty"`
	ConsumedInputs           []WorkstationInput                       `json:"consumed_inputs,omitempty"`
	InputWorkItems           []work.FactoryWorkItem                   `json:"input_work_items,omitempty"`
	OutputWorkItems          []work.FactoryWorkItem                   `json:"output_work_items,omitempty"`
	CurrentChainingTraceID   string                                   `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string                                 `json:"previous_chaining_trace_ids,omitempty"`
	TraceIDs                 []string                                 `json:"trace_ids,omitempty"`
	ProviderSession          *workerexecution.ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics              *workerdiagnostics.SafeWorkDiagnostics   `json:"diagnostics,omitempty"`
	TerminalWork             *FactoryTerminalWork                     `json:"terminal_work,omitempty"`
}

// FactoryWorldTrace groups work and dispatch activity by trace identifier.
type FactoryWorldTrace struct {
	TraceID       string   `json:"trace_id"`
	WorkItemIDs   []string `json:"work_item_ids,omitempty"`
	DispatchIDs   []string `json:"dispatch_ids,omitempty"`
	TerminalWork  []string `json:"terminal_work,omitempty"`
	FailedWorkIDs []string `json:"failed_work_ids,omitempty"`
}

// FactoryWorldWorkStateChangeRecord records one canonical WORK_STATE_CHANGE
// affecting a work item at the selected projection tick.
type FactoryWorldWorkStateChangeRecord struct {
	WorkID       string                     `json:"work_id"`
	WorkTypeName string                     `json:"work_type_name,omitempty"`
	FromState    string                     `json:"from_state"`
	ToState      string                     `json:"to_state"`
	FromPlaceID  string                     `json:"from_place_id"`
	ToPlaceID    string                     `json:"to_place_id"`
	Source       work.WorkStateChangeSource `json:"source"`
	RequestID    string                     `json:"request_id,omitempty"`
	Tick         int                        `json:"tick"`
	Sequence     int                        `json:"sequence"`
	EventTime    time.Time                  `json:"event_time,omitempty"`
}

// FactoryWorldProviderSessionRecord records one provider session attached to a
// workstation response in the canonical event-first world state.
type FactoryWorldProviderSessionRecord struct {
	DispatchID               string                                  `json:"dispatch_id"`
	TransitionID             string                                  `json:"transition_id"`
	WorkstationName          string                                  `json:"workstation_name,omitempty"`
	RunnerID                 string                                  `json:"runner_id,omitempty"`
	RunnerSelectionSource    workerexecution.RunnerSelectionSource   `json:"runner_selection_source,omitempty"`
	Outcome                  string                                  `json:"outcome"`
	ProviderSession          workerexecution.ProviderSessionMetadata `json:"provider_session"`
	Diagnostics              *workerdiagnostics.SafeWorkDiagnostics  `json:"diagnostics,omitempty"`
	WorkItemIDs              []string                                `json:"work_item_ids,omitempty"`
	WorkItems                []FactoryWorldWorkItemRef               `json:"work_items,omitempty"`
	ConsumedInputs           []WorkstationInput                      `json:"consumed_inputs,omitempty"`
	CurrentChainingTraceID   string                                  `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string                                `json:"previous_chaining_trace_ids,omitempty"`
	TraceIDs                 []string                                `json:"trace_ids,omitempty"`
	FailureDetail            *workerexecution.FailureDetail          `json:"failureDetail,omitempty"`
}

// FactoryWorldInferenceAttempt records one provider-boundary inference attempt
// reconstructed from canonical inference request and response events.
type FactoryWorldInferenceAttempt struct {
	DispatchID         string                                   `json:"dispatch_id"`
	TransitionID       string                                   `json:"transition_id"`
	InferenceRequestID string                                   `json:"inference_request_id"`
	Attempt            int                                      `json:"attempt"`
	WorkingDirectory   string                                   `json:"working_directory,omitempty"`
	Worktree           string                                   `json:"worktree,omitempty"`
	Prompt             string                                   `json:"prompt"`
	RequestTime        time.Time                                `json:"request_time,omitempty"`
	Outcome            string                                   `json:"outcome,omitempty"`
	Response           string                                   `json:"response,omitempty"`
	DurationMillis     int64                                    `json:"duration_millis,omitempty"`
	ExitCode           *int                                     `json:"exit_code,omitempty"`
	FailureDetail      *workerexecution.FailureDetail           `json:"failureDetail,omitempty"`
	ProviderSession    *workerexecution.ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics        *workerdiagnostics.SafeWorkDiagnostics   `json:"diagnostics,omitempty"`
	ResponseTime       time.Time                                `json:"response_time,omitempty"`
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

// FactoryWorldAgentRunResponse records one agent-run boundary response
// reconstructed from canonical agent-run response events.
type FactoryWorldAgentRunResponse struct {
	DispatchID     string                                 `json:"dispatch_id"`
	AgentRunID     string                                 `json:"agent_run_id"`
	Outcome        string                                 `json:"outcome,omitempty"`
	DurationMillis int64                                  `json:"duration_millis,omitempty"`
	Diagnostics    *workerdiagnostics.SafeWorkDiagnostics `json:"diagnostics,omitempty"`
	ResponseTime   time.Time                              `json:"response_time,omitempty"`
}

// FactoryWorldFailureDetail associates failed terminal work with the dispatch
// completion that produced the failure.
type FactoryWorldFailureDetail struct {
	DispatchID      string                         `json:"dispatch_id"`
	TransitionID    string                         `json:"transition_id"`
	WorkstationName string                         `json:"workstation_name,omitempty"`
	WorkItem        work.FactoryWorkItem           `json:"work_item"`
	FailureDetail   *workerexecution.FailureDetail `json:"failureDetail,omitempty"`
}

const (
	JavaScriptCheckpointArtifactVisibility = "INTERNAL_CHECKPOINT"
	JavaScriptCheckpointArtifactKind       = "CHECKPOINT"
)

// JavaScriptCheckpointRecord is an orchestrator-owned checkpoint bundle kept out
// of public session, world, and event projections.
type JavaScriptCheckpointRecord struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	Summary     string          `json:"summary"`
	Timestamp   time.Time       `json:"timestamp,omitempty"`
	ArtifactID  string          `json:"artifactId"`
	ContentHash string          `json:"contentHash"`
	SizeBytes   int64           `json:"sizeBytes"`
	RawBody     json.RawMessage `json:"rawBody"`
	StoragePath string          `json:"storagePath"`
}

// JavaScriptCheckpointArtifactRef is customer-visible artifact metadata for one
// orchestrator-owned checkpoint bundle.
type JavaScriptCheckpointArtifactRef struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Visibility  string `json:"visibility"`
	ContentHash string `json:"contentHash,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
}

// FactoryWorldSessionBracketState reconstructs one durable factory session
// execution bracket from SESSION_STARTED, SESSION_RESULT_UPDATED, and
// SESSION_COMPLETED canonical events.
type FactoryWorldSessionBracketState struct {
	SessionID              string                                     `json:"session_id,omitempty"`
	OrchestratorKind       string                                     `json:"orchestrator_kind,omitempty"`
	OrchestratorDialect    string                                     `json:"orchestrator_dialect,omitempty"`
	FactoryID              string                                     `json:"factory_id,omitempty"`
	SourceRef              string                                     `json:"source_ref,omitempty"`
	SourceHash             string                                     `json:"source_hash,omitempty"`
	PolicyHash             string                                     `json:"policy_hash,omitempty"`
	ArgsDigest             string                                     `json:"args_digest,omitempty"`
	StartedAt              time.Time                                  `json:"started_at,omitempty"`
	LifecycleControlStatus string                                     `json:"lifecycle_control_status,omitempty"`
	PausedAt               time.Time                                  `json:"paused_at,omitempty"`
	ResumedAt              time.Time                                  `json:"resumed_at,omitempty"`
	ResultStatus           string                                     `json:"result_status,omitempty"`
	ResultSummary          []work.WorkContentPart                     `json:"result_summary,omitempty"`
	ArtifactIDs            []string                                   `json:"artifact_ids,omitempty"`
	Terminal               bool                                       `json:"terminal"`
	FinalStatus            string                                     `json:"final_status,omitempty"`
	CompletedAt            time.Time                                  `json:"completed_at,omitempty"`
	DurationMillis         int64                                      `json:"duration_millis,omitempty"`
	DispatchCounts         *FactoryWorldJavaScriptChildDispatchCounts `json:"dispatch_counts,omitempty"`
	FailureDetail          *workerexecution.FailureDetail             `json:"failureDetail,omitempty"`
}

// FactoryWorldSessionBracketProjection is the customer-visible session bracket
// projection derived from reconstructed world state.
type FactoryWorldSessionBracketProjection struct {
	SessionID              string                         `json:"session_id,omitempty"`
	OrchestratorKind       string                         `json:"orchestrator_kind,omitempty"`
	OrchestratorDialect    string                         `json:"orchestrator_dialect,omitempty"`
	FactoryID              string                         `json:"factory_id,omitempty"`
	SourceRef              string                         `json:"source_ref,omitempty"`
	StartedAt              time.Time                      `json:"started_at,omitempty"`
	LifecycleControlStatus string                         `json:"lifecycle_control_status,omitempty"`
	PausedAt               time.Time                      `json:"paused_at,omitempty"`
	ResumedAt              time.Time                      `json:"resumed_at,omitempty"`
	ResultStatus           string                         `json:"result_status,omitempty"`
	ResultSummary          []work.WorkContentPart         `json:"result_summary,omitempty"`
	ArtifactIDs            []string                       `json:"artifact_ids,omitempty"`
	Terminal               bool                           `json:"terminal"`
	FinalStatus            string                         `json:"final_status,omitempty"`
	CompletedAt            time.Time                      `json:"completed_at,omitempty"`
	DurationMillis         int64                          `json:"duration_millis,omitempty"`
	FailureDetail          *workerexecution.FailureDetail `json:"failureDetail,omitempty"`
}

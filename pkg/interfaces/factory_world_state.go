package interfaces

import (
	"encoding/json"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// FactoryWorldState is the dashboard-agnostic reconstruction of factory state
// from canonical factory events up to one selected engine tick.
type FactoryWorldState struct {
	Tick                          int                                                `json:"tick"`
	EventTime                     time.Time                                          `json:"event_time,omitempty"`
	Factory                       *factoryapi.Factory                                `json:"factory,omitempty"`
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
	WorkStateChangesByWorkID      map[string][]FactoryWorldWorkStateChangeRecord     `json:"work_state_changes_by_work_id,omitempty"`
	FactoryState                  string                                             `json:"factory_state,omitempty"`
	FactoryStateReason            string                                             `json:"factory_state_reason,omitempty"`
	FactoryStatePrevious          string                                             `json:"factory_state_previous,omitempty"`
	JavaScriptCheckpoints         []FactorySessionJavaScriptCheckpointRef            `json:"javascript_checkpoints,omitempty"`
	JavaScriptRuntime             *FactorySessionJavaScriptRuntimeState              `json:"javascript_runtime,omitempty"`
	Artifacts                     []FactorySessionArtifactState                      `json:"artifacts,omitempty"`
	SessionBracket                *FactoryWorldSessionBracketState                   `json:"session_bracket,omitempty"`
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
	ModelProvider            string                `json:"model_provider,omitempty"`
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
	ModelProvider            string                   `json:"model_provider,omitempty"`
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

// FactoryWorldWorkStateChangeRecord records one canonical WORK_STATE_CHANGE
// affecting a work item at the selected projection tick.
type FactoryWorldWorkStateChangeRecord struct {
	WorkID       string                `json:"work_id"`
	WorkTypeName string                `json:"work_type_name,omitempty"`
	FromState    string                `json:"from_state"`
	ToState      string                `json:"to_state"`
	FromPlaceID  string                `json:"from_place_id"`
	ToPlaceID    string                `json:"to_place_id"`
	Source       WorkStateChangeSource `json:"source"`
	RequestID    string                `json:"request_id,omitempty"`
	Tick         int                   `json:"tick"`
	Sequence     int                   `json:"sequence"`
	EventTime    time.Time             `json:"event_time,omitempty"`
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
	WorkItemIDs              []string                  `json:"work_item_ids,omitempty"`
	WorkItems                []FactoryWorldWorkItemRef `json:"work_items,omitempty"`
	ConsumedInputs           []WorkstationInput        `json:"consumed_inputs,omitempty"`
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
	SessionID           string                              `json:"session_id,omitempty"`
	OrchestratorKind    string                              `json:"orchestrator_kind,omitempty"`
	OrchestratorDialect string                              `json:"orchestrator_dialect,omitempty"`
	FactoryID           string                              `json:"factory_id,omitempty"`
	SourceRef           string                              `json:"source_ref,omitempty"`
	SourceHash          string                              `json:"source_hash,omitempty"`
	PolicyHash          string                              `json:"policy_hash,omitempty"`
	ArgsDigest          string                              `json:"args_digest,omitempty"`
	StartedAt           time.Time                           `json:"started_at,omitempty"`
	ResultStatus        string                              `json:"result_status,omitempty"`
	ResultSummary       []WorkContentPart                   `json:"result_summary,omitempty"`
	ArtifactIDs         []string                            `json:"artifact_ids,omitempty"`
	Terminal            bool                                `json:"terminal"`
	FinalStatus         string                              `json:"final_status,omitempty"`
	CompletedAt         time.Time                           `json:"completed_at,omitempty"`
	DurationMillis      int64                               `json:"duration_millis,omitempty"`
	DispatchCounts      *FactoryWorldJavaScriptChildDispatchCounts `json:"dispatch_counts,omitempty"`
	FailureReason       string                              `json:"failure_reason,omitempty"`
	FailureMessage      string                              `json:"failure_message,omitempty"`
	FailureErrorClass   string                              `json:"failure_error_class,omitempty"`
}

// FactoryWorldSessionBracketProjection is the customer-visible session bracket
// projection derived from reconstructed world state.
type FactoryWorldSessionBracketProjection struct {
	SessionID           string            `json:"session_id,omitempty"`
	OrchestratorKind    string            `json:"orchestrator_kind,omitempty"`
	OrchestratorDialect string            `json:"orchestrator_dialect,omitempty"`
	FactoryID           string            `json:"factory_id,omitempty"`
	SourceRef           string            `json:"source_ref,omitempty"`
	StartedAt           time.Time         `json:"started_at,omitempty"`
	ResultStatus        string            `json:"result_status,omitempty"`
	ResultSummary       []WorkContentPart `json:"result_summary,omitempty"`
	ArtifactIDs         []string          `json:"artifact_ids,omitempty"`
	Terminal            bool              `json:"terminal"`
	FinalStatus         string            `json:"final_status,omitempty"`
	CompletedAt         time.Time         `json:"completed_at,omitempty"`
	DurationMillis      int64             `json:"duration_millis,omitempty"`
	FailureReason       string            `json:"failure_reason,omitempty"`
	FailureMessage      string            `json:"failure_message,omitempty"`
}
// CloneToken returns a detached copy of the canonical runtime token shape.
func CloneToken(token Token) Token {
	return Token{
		ID:        token.ID,
		PlaceID:   token.PlaceID,
		Color:     CloneTokenColor(token.Color),
		CreatedAt: token.CreatedAt,
		EnteredAt: token.EnteredAt,
		History:   CloneTokenHistory(token.History),
	}
}

// CloneTokens returns detached copies of canonical runtime tokens.
func CloneTokens(tokens []Token) []Token {
	if tokens == nil {
		return nil
	}
	clones := make([]Token, len(tokens))
	for i := range tokens {
		clones[i] = CloneToken(tokens[i])
	}
	return clones
}

// CloneTokenColor returns a detached copy of the canonical runtime token color.
func CloneTokenColor(color TokenColor) TokenColor {
	return TokenColor{
		Name:                     color.Name,
		RequestID:                color.RequestID,
		WorkID:                   color.WorkID,
		WorkTypeID:               color.WorkTypeID,
		DataType:                 color.DataType,
		ChainingTraceDepth:       color.ChainingTraceDepth,
		CurrentChainingTraceID:   color.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(color.PreviousChainingTraceIDs),
		TraceID:                  color.TraceID,
		ParentID:                 color.ParentID,
		Tags:                     cloneStringMap(color.Tags),
		Relations:                cloneRelations(color.Relations),
		Content:                  CloneWorkContentParts(color.Content),
		Payload:                  cloneBytes(color.Payload),
	}
}

// CloneWorkContentParts returns a detached copy of canonical work content parts.
func CloneWorkContentParts(parts []WorkContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	clone := make([]WorkContentPart, len(parts))
	copy(clone, parts)
	return clone
}

// CloneTokenHistory returns a detached copy of canonical runtime token history.
func CloneTokenHistory(history TokenHistory) TokenHistory {
	return TokenHistory{
		TotalVisits:         cloneStringIntMap(history.TotalVisits),
		ConsecutiveFailures: cloneStringIntMap(history.ConsecutiveFailures),
		PlaceVisits:         cloneStringIntMap(history.PlaceVisits),
		TotalDuration:       history.TotalDuration,
		LastError:           history.LastError,
		FailureLog:          cloneFailureRecords(history.FailureLog),
	}
}

// CloneProviderSessionMetadata returns a detached copy of canonical provider
// session metadata.
func CloneProviderSessionMetadata(session *ProviderSessionMetadata) *ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}

// CloneWorkFailureMetadata returns a detached copy of canonical work failure
// metadata.
func CloneWorkFailureMetadata(failure *WorkFailureMetadata) *WorkFailureMetadata {
	if failure == nil {
		return nil
	}
	clone := *failure
	return &clone
}

// CloneSafeWorkDiagnostics returns a detached copy of the canonical safe
// diagnostics boundary.
func CloneSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	return &SafeWorkDiagnostics{
		RenderedPrompt: cloneSafeRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneSafeProviderDiagnostic(diagnostics.Provider),
	}
}

// CloneFactoryWorldDispatchCompletion returns a detached copy of one canonical
// selected-tick dispatch completion record.
func CloneFactoryWorldDispatchCompletion(completion FactoryWorldDispatchCompletion) FactoryWorldDispatchCompletion {
	clone := completion
	clone.Result.FailureMetadata = CloneWorkFailureMetadata(completion.Result.FailureMetadata)
	clone.WorkItemIDs = cloneStringSlice(completion.WorkItemIDs)
	clone.ConsumedInputs = cloneWorkstationInputs(completion.ConsumedInputs)
	clone.InputWorkItems = cloneFactoryWorkItems(completion.InputWorkItems)
	clone.OutputWorkItems = cloneFactoryWorkItems(completion.OutputWorkItems)
	clone.PreviousChainingTraceIDs = cloneStringSlice(completion.PreviousChainingTraceIDs)
	clone.TraceIDs = cloneStringSlice(completion.TraceIDs)
	clone.ProviderSession = CloneProviderSessionMetadata(completion.ProviderSession)
	clone.Diagnostics = CloneSafeWorkDiagnostics(completion.Diagnostics)
	clone.TerminalWork = cloneFactoryTerminalWork(completion.TerminalWork)
	return clone
}

// CloneFactoryWorldProviderSessionRecord returns a detached copy of one
// canonical selected-tick provider-session record.
func CloneFactoryWorldProviderSessionRecord(record FactoryWorldProviderSessionRecord) FactoryWorldProviderSessionRecord {
	clone := record
	clone.ProviderSession = *CloneProviderSessionMetadata(&record.ProviderSession)
	clone.Diagnostics = CloneSafeWorkDiagnostics(record.Diagnostics)
	clone.WorkItemIDs = cloneStringSlice(record.WorkItemIDs)
	clone.WorkItems = cloneFactoryWorldWorkItemRefs(record.WorkItems)
	clone.ConsumedInputs = cloneWorkstationInputs(record.ConsumedInputs)
	clone.PreviousChainingTraceIDs = cloneStringSlice(record.PreviousChainingTraceIDs)
	clone.TraceIDs = cloneStringSlice(record.TraceIDs)
	return clone
}

// CloneFactoryWorldInferenceAttemptsByDispatchID returns a detached copy of
// selected-tick inference attempts keyed by dispatch and request ID.
func CloneFactoryWorldInferenceAttemptsByDispatchID(
	attemptsByDispatchID map[string]map[string]FactoryWorldInferenceAttempt,
) map[string]map[string]FactoryWorldInferenceAttempt {
	if len(attemptsByDispatchID) == 0 {
		return nil
	}
	clone := make(map[string]map[string]FactoryWorldInferenceAttempt, len(attemptsByDispatchID))
	for dispatchID, attempts := range attemptsByDispatchID {
		if len(attempts) == 0 {
			continue
		}
		clone[dispatchID] = make(map[string]FactoryWorldInferenceAttempt, len(attempts))
		for requestID, attempt := range attempts {
			clone[dispatchID][requestID] = cloneFactoryWorldInferenceAttempt(attempt)
		}
	}
	if len(clone) == 0 {
		return nil
	}
	return clone
}

// CloneWorkstationInputs returns a detached copy of canonical workstation
// inputs for selected-tick runtime projections.
func CloneWorkstationInputs(inputs []WorkstationInput) []WorkstationInput {
	return cloneWorkstationInputs(inputs)
}

func cloneSafeRenderedPromptDiagnostic(diagnostic *SafeRenderedPromptDiagnostic) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func cloneSafeProviderDiagnostic(diagnostic *SafeProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func cloneFactoryWorldInferenceAttempt(attempt FactoryWorldInferenceAttempt) FactoryWorldInferenceAttempt {
	clone := attempt
	clone.ExitCode = cloneIntPtr(attempt.ExitCode)
	clone.ProviderSession = CloneProviderSessionMetadata(attempt.ProviderSession)
	clone.Diagnostics = CloneSafeWorkDiagnostics(attempt.Diagnostics)
	return clone
}

func cloneFactoryTerminalWork(terminalWork *FactoryTerminalWork) *FactoryTerminalWork {
	if terminalWork == nil {
		return nil
	}
	clone := *terminalWork
	clone.WorkItem.PreviousChainingTraceIDs = cloneStringSlice(terminalWork.WorkItem.PreviousChainingTraceIDs)
	clone.WorkItem.Tags = cloneStringMap(terminalWork.WorkItem.Tags)
	return &clone
}

func cloneFactoryWorkItems(items []FactoryWorkItem) []FactoryWorkItem {
	if len(items) == 0 {
		return nil
	}
	clone := make([]FactoryWorkItem, len(items))
	for i, item := range items {
		clone[i] = item
		clone[i].PreviousChainingTraceIDs = cloneStringSlice(item.PreviousChainingTraceIDs)
		clone[i].Tags = cloneStringMap(item.Tags)
	}
	return clone
}

func cloneWorkstationInputs(inputs []WorkstationInput) []WorkstationInput {
	if len(inputs) == 0 {
		return nil
	}
	clone := make([]WorkstationInput, len(inputs))
	for i, input := range inputs {
		clone[i] = input
		if input.WorkItem != nil {
			item := *input.WorkItem
			item.PreviousChainingTraceIDs = cloneStringSlice(item.PreviousChainingTraceIDs)
			item.Tags = cloneStringMap(item.Tags)
			clone[i].WorkItem = &item
		}
		if input.Resource != nil {
			resource := *input.Resource
			clone[i].Resource = &resource
		}
	}
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	clone := make([]string, len(values))
	copy(clone, values)
	return clone
}

func cloneRelations(relations []Relation) []Relation {
	if relations == nil {
		return nil
	}
	clone := make([]Relation, len(relations))
	copy(clone, relations)
	return clone
}

func cloneBytes(values []byte) []byte {
	if values == nil {
		return nil
	}
	clone := make([]byte, len(values))
	copy(clone, values)
	return clone
}

func cloneFailureRecords(records []FailureRecord) []FailureRecord {
	if records == nil {
		return nil
	}
	clone := make([]FailureRecord, len(records))
	copy(clone, records)
	return clone
}

func cloneStringIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneFactoryWorldWorkItemRef(ref FactoryWorldWorkItemRef) FactoryWorldWorkItemRef {
	clone := ref
	clone.PreviousChainingTraceIDs = cloneStringSlice(ref.PreviousChainingTraceIDs)
	clone.LineageParentWorkIDs = cloneStringSlice(ref.LineageParentWorkIDs)
	clone.Content = CloneWorkContentParts(ref.Content)
	return clone
}

func cloneFactoryWorldWorkItemRefs(refs []FactoryWorldWorkItemRef) []FactoryWorldWorkItemRef {
	if len(refs) == 0 {
		return nil
	}
	clones := make([]FactoryWorldWorkItemRef, len(refs))
	for i := range refs {
		clones[i] = cloneFactoryWorldWorkItemRef(refs[i])
	}
	return clones
}

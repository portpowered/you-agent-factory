package factorycontracts

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/work"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// FactoryEventSchemaVersion identifies the serialized canonical event envelope.
// The value is owned by Factory even though OpenAPI publishes the same wire value.
type FactoryEventSchemaVersion string

const FactoryEventSchemaVersionV1 FactoryEventSchemaVersion = "agent-factory.event.v1"

// FactoryEventType is the canonical Factory event vocabulary. Transport
// adapters map these stable values to generated OpenAPI enums at the edge.
type FactoryEventType string

const (
	FactoryEventTypeAgentRunResponse              FactoryEventType = "AGENT_RUN_RESPONSE"
	FactoryEventTypeArtifactCreated               FactoryEventType = "ARTIFACT_CREATED"
	FactoryEventTypeDispatchInterrupted           FactoryEventType = "DISPATCH_INTERRUPTED"
	FactoryEventTypeDispatchQueued                FactoryEventType = "DISPATCH_QUEUED"
	FactoryEventTypeDispatchReconciled            FactoryEventType = "DISPATCH_RECONCILED"
	FactoryEventTypeDispatchRequest               FactoryEventType = "DISPATCH_REQUEST"
	FactoryEventTypeDispatchResponse              FactoryEventType = "DISPATCH_RESPONSE"
	FactoryEventTypeFactoryChange                 FactoryEventType = "FACTORY_CHANGE"
	FactoryEventTypeFactoryStateResponse          FactoryEventType = "FACTORY_STATE_RESPONSE"
	FactoryEventTypeInferenceRequest              FactoryEventType = "INFERENCE_REQUEST"
	FactoryEventTypeInferenceResponse             FactoryEventType = "INFERENCE_RESPONSE"
	FactoryEventTypeInitialStructureRequest       FactoryEventType = "INITIAL_STRUCTURE_REQUEST"
	FactoryEventTypeJavaScriptCheckpointRef       FactoryEventType = "JAVASCRIPT_CHECKPOINT_REF"
	FactoryEventTypeJavaScriptPhaseChange         FactoryEventType = "JAVASCRIPT_PHASE_CHANGE"
	FactoryEventTypeModelRequest                  FactoryEventType = "MODEL_REQUEST"
	FactoryEventTypeModelResponse                 FactoryEventType = "MODEL_RESPONSE"
	FactoryEventTypeOrchestratorCheckpointWritten FactoryEventType = "ORCHESTRATOR_CHECKPOINT_WRITTEN"
	FactoryEventTypeOrchestratorPhaseChanged      FactoryEventType = "ORCHESTRATOR_PHASE_CHANGED"
	FactoryEventTypeRelationshipChangeRequest     FactoryEventType = "RELATIONSHIP_CHANGE_REQUEST"
	FactoryEventTypeRunRequest                    FactoryEventType = "RUN_REQUEST"
	FactoryEventTypeRunResponse                   FactoryEventType = "RUN_RESPONSE"
	FactoryEventTypeScriptRequest                 FactoryEventType = "SCRIPT_REQUEST"
	FactoryEventTypeScriptResponse                FactoryEventType = "SCRIPT_RESPONSE"
	FactoryEventTypeSessionCompleted              FactoryEventType = "SESSION_COMPLETED"
	FactoryEventTypeSessionLifecycleControl       FactoryEventType = "SESSION_LIFECYCLE_CONTROL"
	FactoryEventTypeSessionPaused                 FactoryEventType = "SESSION_PAUSED"
	FactoryEventTypeSessionResultUpdated          FactoryEventType = "SESSION_RESULT_UPDATED"
	FactoryEventTypeSessionResumed                FactoryEventType = "SESSION_RESUMED"
	FactoryEventTypeSessionStarted                FactoryEventType = "SESSION_STARTED"
	FactoryEventTypeWorkRequest                   FactoryEventType = "WORK_REQUEST"
	FactoryEventTypeWorkStateChange               FactoryEventType = "WORK_STATE_CHANGE"
)

// FactoryEventReconnectCursor identifies the last acknowledged event for stream
// reconnect. Clients may supply AfterEventID or AfterSequence; when both are
// set, AfterEventID wins.
type FactoryEventReconnectCursor struct {
	AfterEventID  string
	AfterSequence *int
}

// FactorySessionLogicalResolveHint carries persisted logical-session identity
// used to remap an unknown factorySessionID to the current live session.
type FactorySessionLogicalResolveHint struct {
	BackendScopeID      string
	LogicalSessionKeyID string
}

// FactorySessionSyncPreflightOptions carries reconnect and logical identity hints
// for session sync preflight resolution.
type FactorySessionSyncPreflightOptions struct {
	Reconnect           *FactoryEventReconnectCursor
	BackendScopeID      *string
	LogicalSessionKeyID *string
}

// FactoryEventReconnectScope configures how reconnect cursors are interpreted.
type FactoryEventReconnectScope struct {
	// SessionID enables sessionSequence-based after_sequence matching for
	// session-scoped event streams.
	SessionID string
}

// FactoryEventStream carries replayed history and then live canonical events.
type FactoryEventStream struct {
	BackendScopeID      string
	LogicalSessionKeyID string
	FactorySessionID    string
	StreamGenerationID  string
	History             []FactoryEvent
	Events              <-chan FactoryEvent
}

// FactoryEvent is the canonical, transport-independent event envelope. Payload
// interpretation belongs to the Factory reducer that handles Type; transports
// may decode the detached JSON at their boundary when a generated union is
// required.
type FactoryEvent struct {
	Context       FactoryEventContext       `json:"context"`
	Id            string                    `json:"id"`
	Payload       json.RawMessage           `json:"payload"`
	SchemaVersion FactoryEventSchemaVersion `json:"schemaVersion"`
	Type          FactoryEventType          `json:"type"`
}

// FactoryEventContext carries canonical ordering and correlation metadata.
type FactoryEventContext struct {
	CheckpointID             *string   `json:"checkpointId,omitempty"`
	CurrentChainingTraceID   *string   `json:"currentChainingTraceId,omitempty"`
	DispatchID               *string   `json:"dispatchId,omitempty"`
	EventTime                time.Time `json:"eventTime"`
	OrchestratorDialect      *string   `json:"orchestratorDialect,omitempty"`
	OrchestratorKind         *string   `json:"orchestratorKind,omitempty"`
	PhaseID                  *string   `json:"phaseId,omitempty"`
	PhaseName                *string   `json:"phaseName,omitempty"`
	PreviousChainingTraceIDs *[]string `json:"previousChainingTraceIds,omitempty"`
	RequestID                *string   `json:"requestId,omitempty"`
	Sequence                 int       `json:"sequence"`
	SessionID                *string   `json:"sessionId,omitempty"`
	SessionSequence          *int      `json:"sessionSequence,omitempty"`
	Source                   *string   `json:"source,omitempty"`
	Tick                     int       `json:"tick"`
	TraceIDs                 *[]string `json:"traceIds,omitempty"`
	WorkIDs                  *[]string `json:"workIds,omitempty"`
}

// NewFactoryEvent converts an event-compatible value into the detached domain
// envelope. It is intended for temporary producer and transport adapters while
// event payload contracts migrate to their domain owners.
func NewFactoryEvent(value any) (FactoryEvent, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return FactoryEvent{}, fmt.Errorf("encode factory event: %w", err)
	}
	var event FactoryEvent
	if err := json.Unmarshal(encoded, &event); err != nil {
		return FactoryEvent{}, fmt.Errorf("decode factory event envelope: %w", err)
	}
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	return event, nil
}

// Decode writes the domain envelope into an event-compatible destination.
func (e FactoryEvent) Decode(destination any) error {
	encoded, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode factory event envelope: %w", err)
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return fmt.Errorf("decode factory event: %w", err)
	}
	return nil
}

// DecodePayload decodes the detached payload into its domain or boundary
// representation. Callers select the payload contract from Type.
func (e FactoryEvent) DecodePayload(destination any) error {
	if err := json.Unmarshal(e.Payload, destination); err != nil {
		return fmt.Errorf("decode %s factory event payload: %w", e.Type, err)
	}
	return nil
}

// InitialStructurePayload describes the topology available before work moves.
type InitialStructurePayload struct {
	Name             string                          `json:"name,omitempty"`
	Version          *FactoryVersion                 `json:"version,omitempty"`
	Resources        []FactoryResource               `json:"resources,omitempty"`
	ResourceManifest *PortableResourceManifestConfig `json:"resource_manifest,omitempty"`
	Constraints      []FactoryConstraint             `json:"constraints,omitempty"`
	Layout           *FactoryLayoutConfig            `json:"layout,omitempty"`
	Workers          []FactoryWorker                 `json:"workers,omitempty"`
	WorkTypes        []FactoryWorkType               `json:"work_types,omitempty"`
	Workstations     []FactoryWorkstation            `json:"workstations,omitempty"`
	Places           []FactoryPlace                  `json:"places,omitempty"`
	Relations        []work.FactoryRelation          `json:"relations,omitempty"`
}

// WorkInputPayload describes a work item submitted to the factory.
type WorkInputPayload struct {
	TokenID   string                 `json:"token_id"`
	WorkItem  work.FactoryWorkItem   `json:"work_item"`
	Relations []work.FactoryRelation `json:"relations,omitempty"`
}

// WorkRequestPayload describes a canonical work request batch submission.
type WorkRequestPayload struct {
	RequestID     string                 `json:"request_id"`
	Type          work.WorkRequestType   `json:"type"`
	TraceID       string                 `json:"trace_id,omitempty"`
	Source        string                 `json:"source,omitempty"`
	ParentLineage []string               `json:"parent_lineage,omitempty"`
	WorkItems     []work.FactoryWorkItem `json:"work_items,omitempty"`
}

// RelationshipChangePayload describes a relationship added by a request batch.
type RelationshipChangePayload struct {
	Relation  work.FactoryRelation `json:"relation"`
	RequestID string               `json:"request_id,omitempty"`
	TraceID   string               `json:"trace_id,omitempty"`
}

// WorkstationRequestPayload describes work and resources consumed by a dispatch.
type WorkstationRequestPayload struct {
	DispatchID            string                                `json:"dispatch_id"`
	TransitionID          string                                `json:"transition_id"`
	Workstation           FactoryWorkstationRef                 `json:"workstation"`
	RunnerID              string                                `json:"runner_id,omitempty"`
	RunnerSelectionSource workerexecution.RunnerSelectionSource `json:"runner_selection_source,omitempty"`
	Inputs                []WorkstationInput                    `json:"inputs,omitempty"`
	Resources             []FactoryResourceUnit                 `json:"resources,omitempty"`
}

// WorkstationResponsePayload describes the result and outputs of a dispatch.
type WorkstationResponsePayload struct {
	DispatchID      string                                   `json:"dispatch_id"`
	TransitionID    string                                   `json:"transition_id"`
	Workstation     FactoryWorkstationRef                    `json:"workstation"`
	Result          WorkstationResult                        `json:"result"`
	DurationMillis  int64                                    `json:"duration_millis"`
	Outputs         []WorkstationOutput                      `json:"outputs,omitempty"`
	OutputWork      []work.FactoryWorkItem                   `json:"output_work,omitempty"`
	OutputResources []FactoryResourceUnit                    `json:"output_resources,omitempty"`
	TraceData       *FactoryTraceData                        `json:"trace_data,omitempty"`
	ProviderSession *workerexecution.ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics     *workerdiagnostics.SafeWorkDiagnostics   `json:"diagnostics,omitempty"`
	TerminalWork    *FactoryTerminalWork                     `json:"terminal_work,omitempty"`
}

// DispatchConsumedWorkRef identifies one work item consumed by a dispatch.
// Work identity remains authoritative in FactoryEventContext; WorkID is kept
// for compatibility with recordings that predate context-owned work IDs.
type DispatchConsumedWorkRef struct {
	WorkID string `json:"workId,omitempty"`
}

// DispatchRequestEventMetadata carries non-identity replay metadata retained
// on a dispatch request event.
type DispatchRequestEventMetadata struct {
	ReplayKey *string `json:"replayKey,omitempty"`
}

// DispatchResourceRef identifies a resource consumed by a dispatch. Replay
// reconstruction needs only the canonical resource name from the public
// resource snapshot.
type DispatchResourceRef struct {
	Name string `json:"name"`
}

// DispatchRequestEventPayload describes a dispatch beginning execution.
// Correlation identity belongs to FactoryEventContext; the deprecated chaining
// fields remain readable for compatibility with historical recordings.
type DispatchRequestEventPayload struct {
	CurrentChainingTraceID   *string                       `json:"currentChainingTraceId,omitempty"`
	Inputs                   []DispatchConsumedWorkRef     `json:"inputs"`
	Metadata                 *DispatchRequestEventMetadata `json:"metadata,omitempty"`
	PreviousChainingTraceIDs *[]string                     `json:"previousChainingTraceIds,omitempty"`
	Resources                *[]DispatchResourceRef        `json:"resources,omitempty"`
	TransitionID             string                        `json:"transitionId"`
}

// WorkStateChangeEventPayload describes a canonical Petri marking position
// change. Correlation and ordering remain authoritative in FactoryEventContext.
type WorkStateChangeEventPayload struct {
	FromPlaceID   string                     `json:"fromPlaceId"`
	FromState     string                     `json:"fromState"`
	Reason        *string                    `json:"reason,omitempty"`
	Source        work.WorkStateChangeSource `json:"source"`
	ToPlaceID     string                     `json:"toPlaceId"`
	ToState       string                     `json:"toState"`
	TriggerWorkID *string                    `json:"triggerWorkId,omitempty"`
	WorkID        string                     `json:"workId"`
	WorkTypeName  string                     `json:"workTypeName"`
}

// RunRequestEventPayload describes the Factory snapshot and optional replay
// metadata captured when a Factory run starts.
type RunRequestEventPayload struct {
	Diagnostics *ReplayDiagnostics `json:"diagnostics,omitempty"`
	Factory     *FactorySnapshot   `json:"factory"`
	RecordedAt  time.Time          `json:"recordedAt"`
	WallClock   *RunEventWallClock `json:"wallClock,omitempty"`
}

// RunResponseEventPayload describes the terminal state and optional replay
// metadata emitted when a Factory run finishes.
type RunResponseEventPayload struct {
	Diagnostics *ReplayDiagnostics `json:"diagnostics,omitempty"`
	Reason      *string            `json:"reason,omitempty"`
	State       *FactoryState      `json:"state,omitempty"`
	WallClock   *RunEventWallClock `json:"wallClock,omitempty"`
}

// RunEventWallClock records the observable wall-clock bounds of a Factory run.
type RunEventWallClock struct {
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
}

// FactoryStateChangePayload describes a lifecycle state change.
type FactoryStateChangePayload struct {
	PreviousState string `json:"previous_state,omitempty"`
	State         string `json:"state"`
	Reason        string `json:"reason,omitempty"`
}

// FactoryResource describes a bounded resource type.
type FactoryResource struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Capacity int    `json:"capacity"`
}

// FactoryResourceUnit identifies a concrete resource token.
type FactoryResourceUnit struct {
	ResourceID string `json:"resource_id"`
	TokenID    string `json:"token_id"`
	PlaceID    string `json:"place_id,omitempty"`
}

// FactoryConstraint describes a named runtime constraint or limit.
type FactoryConstraint struct {
	ID     string            `json:"id"`
	Type   string            `json:"type"`
	Scope  string            `json:"scope,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

// FactoryWorker describes an executable worker type.
type FactoryWorker struct {
	ID            string            `json:"id"`
	Name          string            `json:"name,omitempty"`
	Provider      string            `json:"provider,omitempty"`
	ModelProvider string            `json:"model_provider,omitempty"`
	Model         string            `json:"model,omitempty"`
	Config        map[string]string `json:"config,omitempty"`
}

// FactoryWorkType describes a work type and its possible states.
type FactoryWorkType struct {
	ID     string                   `json:"id"`
	Name   string                   `json:"name,omitempty"`
	States []FactoryStateDefinition `json:"states,omitempty"`
}

// FactoryStateDefinition describes a named state in a work type lifecycle.
type FactoryStateDefinition struct {
	Value    string `json:"value"`
	Category string `json:"category"`
}

// FactoryWorkstation describes a transition that can execute work.
type FactoryWorkstation struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	WorkerID          string            `json:"worker_id,omitempty"`
	Kind              string            `json:"kind,omitempty"`
	Config            map[string]string `json:"config,omitempty"`
	InputPlaceIDs     []string          `json:"input_place_ids,omitempty"`
	OutputPlaceIDs    []string          `json:"output_place_ids,omitempty"`
	ContinuePlaceIDs  []string          `json:"continue_place_ids,omitempty"`
	RejectionPlaceIDs []string          `json:"rejection_place_ids,omitempty"`
	FailurePlaceIDs   []string          `json:"failure_place_ids,omitempty"`
}

// FactoryWorkstationRef identifies a workstation in a runtime event.
type FactoryWorkstationRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// FactoryPlace describes a topology place for work or resource tokens.
type FactoryPlace struct {
	ID       string `json:"id"`
	TypeID   string `json:"type_id"`
	State    string `json:"state"`
	Category string `json:"category,omitempty"`
}

// WorkstationInput describes an input token consumed by a dispatch.
type WorkstationInput struct {
	TokenID  string                `json:"token_id"`
	PlaceID  string                `json:"place_id"`
	WorkItem *work.FactoryWorkItem `json:"work_item,omitempty"`
	Resource *FactoryResourceUnit  `json:"resource,omitempty"`
}

// WorkstationOutput describes a token produced or moved by a dispatch.
type WorkstationOutput struct {
	Type      string                `json:"type"`
	TokenID   string                `json:"token_id"`
	FromPlace string                `json:"from_place,omitempty"`
	ToPlace   string                `json:"to_place,omitempty"`
	WorkItem  *work.FactoryWorkItem `json:"work_item,omitempty"`
	Resource  *FactoryResourceUnit  `json:"resource,omitempty"`
}

// WorkstationResult describes the business result of a workstation execution.
type WorkstationResult struct {
	Outcome                     string                               `json:"outcome"`
	Output                      string                               `json:"output,omitempty"`
	Error                       string                               `json:"error,omitempty"`
	Feedback                    string                               `json:"feedback,omitempty"`
	SelectedClassificationLabel string                               `json:"selected_classification_label,omitempty"`
	FailureDetail               *workerexecution.FailureDetail       `json:"failureDetail,omitempty"`
	FailureMetadata             *workerexecution.WorkFailureMetadata `json:"failure_metadata,omitempty"`
}

// FactoryTraceData carries trace identifiers attached to a runtime event.
type FactoryTraceData struct {
	TraceID string   `json:"trace_id,omitempty"`
	WorkIDs []string `json:"work_ids,omitempty"`
}

// FactoryTerminalWork describes work that reached a terminal outcome.
type FactoryTerminalWork struct {
	WorkItem work.FactoryWorkItem `json:"work_item"`
	Status   string               `json:"status"`
}

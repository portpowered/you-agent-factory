package factorycontracts

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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
	History             []factoryapi.FactoryEvent
	Events              <-chan factoryapi.FactoryEvent
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

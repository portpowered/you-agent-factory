package interfaces

import factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"

// FactoryEventStream carries replayed history and then live canonical events.
type FactoryEventStream struct {
	History []factoryapi.FactoryEvent
	Events  <-chan factoryapi.FactoryEvent
}

// InitialStructurePayload describes the topology available before work moves.
type InitialStructurePayload struct {
	Name         string               `json:"name,omitempty"`
	Resources    []FactoryResource    `json:"resources,omitempty"`
	Constraints  []FactoryConstraint  `json:"constraints,omitempty"`
	Workers      []FactoryWorker      `json:"workers,omitempty"`
	WorkTypes    []FactoryWorkType    `json:"work_types,omitempty"`
	Workstations []FactoryWorkstation `json:"workstations,omitempty"`
	Places       []FactoryPlace       `json:"places,omitempty"`
	Relations    []FactoryRelation    `json:"relations,omitempty"`
}

// WorkInputPayload describes a work item submitted to the factory.
type WorkInputPayload struct {
	TokenID   string            `json:"token_id"`
	WorkItem  FactoryWorkItem   `json:"work_item"`
	Relations []FactoryRelation `json:"relations,omitempty"`
}

// WorkRequestPayload describes a canonical work request batch submission.
type WorkRequestPayload struct {
	RequestID       string            `json:"request_id"`
	Type            WorkRequestType   `json:"type"`
	TraceID         string            `json:"trace_id,omitempty"`
	Source          string            `json:"source,omitempty"`
	RelationContext []WorkRelation    `json:"relation_context,omitempty"`
	ParentLineage   []string          `json:"parent_lineage,omitempty"`
	WorkItems       []FactoryWorkItem `json:"work_items,omitempty"`
}

// RelationshipChangePayload describes a relationship added by a request batch.
type RelationshipChangePayload struct {
	Relation  FactoryRelation `json:"relation"`
	RequestID string          `json:"request_id,omitempty"`
	TraceID   string          `json:"trace_id,omitempty"`
}

// WorkstationRequestPayload describes work and resources consumed by a dispatch.
type WorkstationRequestPayload struct {
	DispatchID   string                `json:"dispatch_id"`
	TransitionID string                `json:"transition_id"`
	Workstation  FactoryWorkstationRef `json:"workstation"`
	Inputs       []WorkstationInput    `json:"inputs,omitempty"`
	Resources    []FactoryResourceUnit `json:"resources,omitempty"`
}

// WorkstationResponsePayload describes the result and outputs of a dispatch.
type WorkstationResponsePayload struct {
	DispatchID      string                   `json:"dispatch_id"`
	TransitionID    string                   `json:"transition_id"`
	Workstation     FactoryWorkstationRef    `json:"workstation"`
	Result          WorkstationResult        `json:"result"`
	DurationMillis  int64                    `json:"duration_millis"`
	Outputs         []WorkstationOutput      `json:"outputs,omitempty"`
	OutputWork      []FactoryWorkItem        `json:"output_work,omitempty"`
	OutputResources []FactoryResourceUnit    `json:"output_resources,omitempty"`
	TraceData       *FactoryTraceData        `json:"trace_data,omitempty"`
	ProviderSession *ProviderSessionMetadata `json:"provider_session,omitempty"`
	Diagnostics     *SafeWorkDiagnostics     `json:"diagnostics,omitempty"`
	TerminalWork    *FactoryTerminalWork     `json:"terminal_work,omitempty"`
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
	TokenID  string               `json:"token_id"`
	PlaceID  string               `json:"place_id"`
	WorkItem *FactoryWorkItem     `json:"work_item,omitempty"`
	Resource *FactoryResourceUnit `json:"resource,omitempty"`
}

// WorkstationOutput describes a token produced or moved by a dispatch.
type WorkstationOutput struct {
	Type      string               `json:"type"`
	TokenID   string               `json:"token_id"`
	FromPlace string               `json:"from_place,omitempty"`
	ToPlace   string               `json:"to_place,omitempty"`
	WorkItem  *FactoryWorkItem     `json:"work_item,omitempty"`
	Resource  *FactoryResourceUnit `json:"resource,omitempty"`
}

// WorkstationResult describes the business result of a workstation execution.
type WorkstationResult struct {
	Outcome         string                   `json:"outcome"`
	Output          string                   `json:"output,omitempty"`
	Error           string                   `json:"error,omitempty"`
	Feedback        string                   `json:"feedback,omitempty"`
	FailureReason   string                   `json:"failure_reason,omitempty"`
	FailureMessage  string                   `json:"failure_message,omitempty"`
	ProviderFailure *ProviderFailureMetadata `json:"provider_failure,omitempty"`
}

// FactoryTraceData carries trace identifiers attached to a runtime event.
type FactoryTraceData struct {
	TraceID string   `json:"trace_id,omitempty"`
	WorkIDs []string `json:"work_ids,omitempty"`
}

// FactoryTerminalWork describes work that reached a terminal outcome.
type FactoryTerminalWork struct {
	WorkItem FactoryWorkItem `json:"work_item"`
	Status   string          `json:"status"`
}

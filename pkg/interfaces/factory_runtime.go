package interfaces

// FactoryState represents the current lifecycle state of a Factory.
type FactoryState string

const (
	FactoryStateIdle      FactoryState = "IDLE"
	FactoryStateRunning   FactoryState = "RUNNING"
	FactoryStatePaused    FactoryState = "PAUSED"
	FactoryStateCompleted FactoryState = "COMPLETED"
	FactoryStateFailed    FactoryState = "FAILED"
)

// RuntimeMode determines whether the runtime exits on idle completion or stays
// available for future submissions until its context is canceled.
type RuntimeMode string

const (
	RuntimeModeBatch   RuntimeMode = "BATCH"
	RuntimeModeService RuntimeMode = "SERVICE"
)

// SubmitRequest is the internal normalized item used to create work tokens.
type SubmitRequest struct {
	RequestID                string            `json:"requestId,omitempty"`
	WorkID                   string            `json:"workId,omitempty"`
	Name                     string            `json:"name,omitempty"`
	WorkTypeID               string            `json:"workTypeName"`
	TargetState              string            `json:"targetState,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId"`
	Payload                  []byte            `json:"payload"`
	Tags                     map[string]string `json:"tags"`
	Relations                []Relation        `json:"relations"`
	ExecutionID              string            `json:"executionId,omitempty"`
}

// WorkRequestType identifies the canonical request contract accepted by factory submit surfaces.
type WorkRequestType string

const (
	WorkRequestTypeFactoryRequestBatch WorkRequestType = "FACTORY_REQUEST_BATCH"
)

// WorkRequest is the factory-domain representation of the generated WorkRequest schema.
type WorkRequest struct {
	RequestID              string          `json:"requestId"`
	CurrentChainingTraceID string          `json:"currentChainingTraceId,omitempty"`
	Type                   WorkRequestType `json:"type"`
	Works                  []Work          `json:"works,omitempty"`
	Relations              []WorkRelation  `json:"relations,omitempty"`
}

// WorkRequestSubmitResult describes accepted request metadata.
type WorkRequestSubmitResult struct {
	RequestID string
	TraceID   string
	Accepted  bool
}

// Work is one public item inside a WorkRequest batch.
type Work struct {
	Name                     string            `json:"name"`
	WorkID                   string            `json:"workId,omitempty"`
	RequestID                string            `json:"requestId,omitempty"`
	WorkTypeID               string            `json:"workTypeName,omitempty"`
	State                    string            `json:"state,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Payload                  any               `json:"payload,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
	ExecutionID              string            `json:"-"`
	RuntimeRelations         []Relation        `json:"-"`
}

// WorkRelationType identifies a relationship between work items in a WorkRequest.
type WorkRelationType string

const (
	WorkRelationDependsOn   WorkRelationType = "DEPENDS_ON"
	WorkRelationParentChild WorkRelationType = "PARENT_CHILD"
)

// WorkRelation describes a relation between named work items in a WorkRequest.
type WorkRelation struct {
	Type           WorkRelationType `json:"type"`
	SourceWorkName string           `json:"sourceWorkName"`
	TargetWorkName string           `json:"targetWorkName"`
	RequiredState  string           `json:"requiredState,omitempty"`
}

// WorkRequestNormalizeOptions provides context inferred from a submit surface.
type WorkRequestNormalizeOptions struct {
	DefaultWorkTypeID string
	ValidWorkTypes    map[string]bool
	ValidStatesByType map[string]map[string]bool
}

// FactoryWorkItem describes a unit of work at a point in history.
type FactoryWorkItem struct {
	ID                       string            `json:"id"`
	WorkTypeID               string            `json:"workTypeId"`
	State                    string            `json:"state,omitempty"`
	DisplayName              string            `json:"displayName,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	ParentID                 string            `json:"parentId,omitempty"`
	PlaceID                  string            `json:"placeId,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
}

// FactoryRelation describes a typed relationship between work items.
type FactoryRelation struct {
	Type           string `json:"type"`
	SourceWorkID   string `json:"sourceWorkId,omitempty"`
	SourceWorkName string `json:"sourceWorkName,omitempty"`
	TargetWorkID   string `json:"targetWorkId"`
	TargetWorkName string `json:"targetWorkName,omitempty"`
	RequiredState  string `json:"requiredState,omitempty"`
	RequestID      string `json:"requestId,omitempty"`
	TraceID        string `json:"traceId,omitempty"`
}

// WorkRequestRecord stores the batch-level request observed before work token injection.
type WorkRequestRecord struct {
	RequestID       string
	Type            WorkRequestType
	TraceID         string
	Source          string
	RelationContext []WorkRelation
	ParentLineage   []string
	WorkItems       []FactoryWorkItem
	Relations       []FactoryRelation
}

// GeneratedSubmissionBatchMetadata captures request-level metadata for generated work.
type GeneratedSubmissionBatchMetadata struct {
	Source          string         `json:"source"`
	RelationContext []WorkRelation `json:"relationContext"`
	ParentLineage   []string       `json:"parentLineage"`
}

// GeneratedSubmissionBatch carries a canonical generated request with runtime submissions.
type GeneratedSubmissionBatch struct {
	Request     WorkRequest                      `json:"request"`
	Metadata    GeneratedSubmissionBatchMetadata `json:"metadata"`
	Submissions []SubmitRequest                  `json:"submissions"`
}

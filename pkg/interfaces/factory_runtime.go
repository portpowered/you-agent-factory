package interfaces

import "encoding/json"

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
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId"`
	Content                  []WorkContentPart `json:"content,omitempty"`
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
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  any               `json:"payload,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
	ExecutionID              string            `json:"-"`
	RuntimeRelations         []Relation        `json:"-"`
}

// WorkContentPart is the backend-owned canonical work content shape mirrored
// from the public API contract.
type WorkContentPart struct {
	Type        WorkContentPartType `json:"type"`
	Text        string              `json:"text,omitempty"`
	File        string              `json:"file,omitempty"`
	JSON        json.RawMessage     `json:"json,omitempty"`
	Slot        string              `json:"slot,omitempty"`
	Label       string              `json:"label,omitempty"`
	Role        string              `json:"role,omitempty"`
	ContentType string              `json:"contentType,omitempty"`
	ArtifactID  string              `json:"artifactId,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
}

// WorkContentPartType identifies one canonical content part kind.
type WorkContentPartType string

const (
	WorkContentPartTypeText   WorkContentPartType = "text"
	WorkContentPartTypeImage  WorkContentPartType = "image"
	WorkContentPartTypeAudio  WorkContentPartType = "AUDIO"
	WorkContentPartTypeJSON   WorkContentPartType = "JSON"
	WorkContentPartTypeBinary WorkContentPartType = "BINARY"
)

// Normalized returns the stable backend-owned kind for supported public aliases.
func (t WorkContentPartType) Normalized() WorkContentPartType {
	switch t {
	case "TEXT":
		return WorkContentPartTypeText
	case "IMAGE":
		return WorkContentPartTypeImage
	default:
		return t
	}
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
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
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
	RequestID     string
	Type          WorkRequestType
	TraceID       string
	Source        string
	ParentLineage []string
	WorkItems     []FactoryWorkItem
	Relations     []FactoryRelation
}

// GeneratedSubmissionBatchMetadata captures request-level metadata for generated work.
type GeneratedSubmissionBatchMetadata struct {
	Source        string   `json:"source"`
	ParentLineage []string `json:"parentLineage"`
}

// GeneratedSubmissionBatch carries a canonical generated request with runtime submissions.
type GeneratedSubmissionBatch struct {
	Request     WorkRequest                      `json:"request"`
	Metadata    GeneratedSubmissionBatchMetadata `json:"metadata"`
	Submissions []SubmitRequest                  `json:"submissions"`
}

// FactorySubmissionRecord stores the engine tick at which a submit request
// became visible to the runtime.
type FactorySubmissionRecord struct {
	SubmissionID string
	ObservedTick int
	Request      SubmitRequest
	Source       string
}

// FactoryDispatchRecord stores a raw WorkDispatch plus token mutations held
// while the worker is in flight.
type FactoryDispatchRecord struct {
	DispatchID     string
	CreatedTick    int
	Dispatch       WorkDispatch
	HeldMutations  []MarkingMutation
	ConsumedTokens []string
}

// FactoryCompletionRecord stores a worker result at the logical tick where the
// engine observed it.
type FactoryCompletionRecord struct {
	CompletionID string
	DispatchID   string
	ObservedTick int
	Result       WorkResult
}

// SubmissionHookContext is the input passed to engine-owned submission hooks
// once per logical tick.
type SubmissionHookContext[TSnapshot any] struct {
	Snapshot          TSnapshot
	ContinuationState map[string]string
}

// SubmissionHookResult contains all due hook output observed by the engine at
// one logical tick.
type SubmissionHookResult struct {
	GeneratedBatches  []GeneratedSubmissionBatch
	Results           []WorkResult
	ContinuationState map[string]string
	KeepAlive         bool
}

package work

import (
	"encoding/json"
	"errors"
)

// ErrMoveWorkRequestAlreadyApplied reports a duplicate operator move requestId.
var ErrMoveWorkRequestAlreadyApplied = errors.New("operator move request was already applied")

// SubmitRequest is the internal normalized item used to create work tokens.
type SubmitRequest struct {
	RequestID                string               `json:"requestId,omitempty"`
	WorkID                   string               `json:"workId,omitempty"`
	Name                     string               `json:"name,omitempty"`
	WorkTypeID               string               `json:"workTypeName"`
	TargetState              string               `json:"targetState,omitempty"`
	ChainingTraceDepth       int                  `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string               `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string             `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string               `json:"traceId"`
	Content                  []WorkContentPart    `json:"content,omitempty"`
	Payload                  []byte               `json:"payload"`
	Tags                     map[string]string    `json:"tags"`
	Relations                []Relation           `json:"relations"`
	ExecutionID              string               `json:"executionId,omitempty"`
	InvocationArguments      *InvocationArguments `json:"-"`
}

// WorkRequestType identifies the canonical request contract accepted by factory submit surfaces.
type WorkRequestType string

const WorkRequestTypeFactoryRequestBatch WorkRequestType = "FACTORY_REQUEST_BATCH"

// WorkRequest is the domain representation of the generated WorkRequest schema.
type WorkRequest struct {
	RequestID              string          `json:"requestId"`
	CurrentChainingTraceID string          `json:"currentChainingTraceId,omitempty"`
	Type                   WorkRequestType `json:"type"`
	Works                  []Work          `json:"works,omitempty"`
	Relations              []WorkRelation  `json:"relations,omitempty"`
}

// WorkRequestEventPayload is the Work-owned payload recorded when a request
// enters a Factory. FactoryEvent context remains authoritative for request,
// trace, and work identity; these fields preserve the public event wire shape.
type WorkRequestEventPayload struct {
	ParentLineage []string                   `json:"parentLineage,omitempty"`
	Relations     []WorkRequestEventRelation `json:"relations,omitempty"`
	Source        string                     `json:"source,omitempty"`
	Type          WorkRequestType            `json:"type"`
	Works         []WorkRequestEventWork     `json:"works,omitempty"`
}

// WorkRequestEventWork preserves the event representation of submitted work
// until Factory context fallbacks are applied by the consuming reducer.
type WorkRequestEventWork struct {
	Name                     string            `json:"name"`
	WorkID                   string            `json:"workId,omitempty"`
	RequestID                string            `json:"requestId,omitempty"`
	WorkTypeID               string            `json:"workTypeName,omitempty"`
	State                    *WorkEventState   `json:"state,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  json.RawMessage   `json:"payload,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
}

// WorkEventState is the state reference embedded in the public Work event
// shape. Replay needs the authored name; the optional durable ID and category
// remain available for compatible decoding.
type WorkEventState struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// WorkRequestEventRelation preserves both name- and ID-based relationship
// references accepted by historical Work request events.
type WorkRequestEventRelation struct {
	Type           WorkRelationType `json:"type"`
	SourceWorkName string           `json:"sourceWorkName"`
	TargetWorkID   string           `json:"targetWorkId,omitempty"`
	TargetWorkName string           `json:"targetWorkName"`
	RequiredState  string           `json:"requiredState,omitempty"`
}

// RelationshipChangeRequestEventPayload records one Work-owned relationship
// in canonical Factory history.
type RelationshipChangeRequestEventPayload struct {
	Relation WorkRequestEventRelation `json:"relation"`
}

// WorkRequestSubmittedWork identifies one accepted work item in a batch upsert.
type WorkRequestSubmittedWork struct {
	Name         string
	WorkTypeName string
	WorkID       string
}

// WorkRequestSubmitResult describes accepted request metadata.
type WorkRequestSubmitResult struct {
	RequestID    string
	TraceID      string
	WorkID       string
	Name         string
	WorkTypeName string
	Accepted     bool
	Works        []WorkRequestSubmittedWork
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

// WorkContentPart is the backend-owned canonical work content shape mirrored from the public API contract.
type WorkContentPart struct {
	Type        WorkContentPartType `json:"type"`
	Text        string              `json:"text,omitempty"`
	URL         string              `json:"url,omitempty"`
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

// InvocationArguments carries transport-independent invocation parameter normalization data.
type InvocationArguments struct {
	Arguments map[string]InvocationArgument `json:"-"`
}

type InvocationArgument struct {
	Values    []string                   `json:"-"`
	ValueMode string                     `json:"-"`
	Sensitive bool                       `json:"-"`
	Sources   []InvocationArgumentSource `json:"-"`
}

type InvocationArgumentSource struct {
	Kind   string `json:"-"`
	Name   string `json:"-"`
	Redact bool   `json:"-"`
}

type WorkRelationType string

const (
	WorkRelationDependsOn   WorkRelationType = "DEPENDS_ON"
	WorkRelationParentChild WorkRelationType = "PARENT_CHILD"
)

type WorkRelation struct {
	Type           WorkRelationType `json:"type"`
	SourceWorkName string           `json:"sourceWorkName"`
	TargetWorkName string           `json:"targetWorkName"`
	RequiredState  string           `json:"requiredState,omitempty"`
}

type WorkRequestNormalizeOptions struct {
	DefaultWorkTypeID string
	ValidWorkTypes    map[string]bool
	ValidStatesByType map[string]map[string]bool
}

// Relation defines a typed relationship between runtime work items.
type Relation struct {
	Type          RelationType `json:"type"`
	TargetWorkID  string       `json:"target_work_id"`
	RequiredState string       `json:"required_state,omitempty"`
}

type RelationType string

const (
	RelationDependsOn   RelationType = "DEPENDS_ON"
	RelationParentChild RelationType = "PARENT_CHILD"
	RelationSpawnedBy   RelationType = "SPAWNED_BY"
)

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

// WorkRequestRecord stores the batch-level request observed before runtime injection.
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

// FactorySubmissionRecord stores when a submit request became visible to the runtime.
type FactorySubmissionRecord struct {
	SubmissionID string
	ObservedTick int
	Request      SubmitRequest
	Source       string
}

// WorkStateChangeSource identifies who initiated a work-state change.
type WorkStateChangeSource string

const (
	WorkStateChangeSourceAPI              WorkStateChangeSource = "api"
	WorkStateChangeSourceCLI              WorkStateChangeSource = "cli"
	WorkStateChangeSourceCascadingFailure WorkStateChangeSource = "cascading-failure"
)

type WorkStateChangeRecord struct {
	WorkID, WorkTypeID, WorkTypeName string
	FromState, ToState               string
	FromPlaceID, ToPlaceID           string
	Source                           WorkStateChangeSource
	RequestID, TriggerWorkID, Reason string
}

type OperatorMoveResult struct {
	WorkID, WorkTypeID     string
	FromState, ToState     string
	FromPlaceID, ToPlaceID string
	TokenID                string
}

package requestadmission

import "encoding/json"

type RequestType string

const RequestTypeFactoryRequestBatch RequestType = "FACTORY_REQUEST_BATCH"

type WorkRelationType string

const (
	WorkRelationDependsOn   WorkRelationType = "DEPENDS_ON"
	WorkRelationParentChild WorkRelationType = "PARENT_CHILD"
)

type RelationType string

const (
	RelationDependsOn   RelationType = "DEPENDS_ON"
	RelationParentChild RelationType = "PARENT_CHILD"
)

type ContentPartType string

const (
	ContentPartTypeText   ContentPartType = "text"
	ContentPartTypeImage  ContentPartType = "image"
	ContentPartTypeAudio  ContentPartType = "AUDIO"
	ContentPartTypeJSON   ContentPartType = "JSON"
	ContentPartTypeBinary ContentPartType = "BINARY"
)

func (t ContentPartType) Normalized() ContentPartType {
	switch t {
	case "TEXT":
		return ContentPartTypeText
	case "IMAGE":
		return ContentPartTypeImage
	default:
		return t
	}
}

type ContentPart struct {
	Type        ContentPartType `json:"type"`
	Text        string          `json:"text,omitempty"`
	URL         string          `json:"url,omitempty"`
	File        string          `json:"file,omitempty"`
	JSON        json.RawMessage `json:"json,omitempty"`
	Slot        string          `json:"slot,omitempty"`
	Label       string          `json:"label,omitempty"`
	Role        string          `json:"role,omitempty"`
	ContentType string          `json:"contentType,omitempty"`
	ArtifactID  string          `json:"artifactId,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

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

type Request struct {
	RequestID              string          `json:"requestId"`
	CurrentChainingTraceID string          `json:"currentChainingTraceId,omitempty"`
	Type                   RequestType     `json:"type"`
	Works                  []Work          `json:"works,omitempty"`
	Relations              []WorkRelation  `json:"relations,omitempty"`
}

type Work struct {
	Name                     string               `json:"name"`
	WorkID                   string               `json:"workId,omitempty"`
	RequestID                string               `json:"requestId,omitempty"`
	WorkTypeID               string               `json:"workTypeName,omitempty"`
	State                    string               `json:"state,omitempty"`
	ChainingTraceDepth       int                  `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string               `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string             `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string               `json:"traceId,omitempty"`
	Content                  []ContentPart        `json:"content,omitempty"`
	Payload                  any                  `json:"payload,omitempty"`
	Tags                     map[string]string    `json:"tags,omitempty"`
	ExecutionID              string               `json:"-"`
	RuntimeRelations         []Relation           `json:"-"`
	InvocationArguments      *InvocationArguments `json:"-"`
}

type WorkRelation struct {
	Type           WorkRelationType `json:"type"`
	SourceWorkName string           `json:"sourceWorkName"`
	TargetWorkName string           `json:"targetWorkName"`
	RequiredState  string           `json:"requiredState,omitempty"`
}

type Relation struct {
	Type          RelationType `json:"type"`
	TargetWorkID  string       `json:"target_work_id"`
	RequiredState string       `json:"required_state,omitempty"`
}

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
	Content                  []ContentPart        `json:"content,omitempty"`
	Payload                  []byte               `json:"payload"`
	Tags                     map[string]string    `json:"tags"`
	Relations                []Relation           `json:"relations"`
	ExecutionID              string               `json:"executionId,omitempty"`
	InvocationArguments      *InvocationArguments `json:"-"`
}

type NormalizeOptions struct {
	DefaultWorkTypeID string
	ValidWorkTypes    map[string]bool
	ValidStatesByType map[string]map[string]bool
	IDGenerator       IDGenerator
}

type IDGenerator func() string

type GeneratedSubmissionBatch struct {
	Request     Request                      `json:"request"`
	Metadata    GeneratedSubmissionBatchMetadata `json:"metadata"`
	Submissions []SubmitRequest              `json:"submissions"`
}

type GeneratedSubmissionBatchMetadata struct {
	Source        string   `json:"source"`
	ParentLineage []string `json:"parentLineage"`
}

type FactoryWorkItem struct {
	ID                       string
	WorkTypeID               string
	State                    string
	DisplayName              string
	ChainingTraceDepth       int
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	TraceID                  string
	Content                  []ContentPart
	Tags                     map[string]string
}

type FactoryRelation struct {
	Type           string
	SourceWorkID   string
	SourceWorkName string
	TargetWorkID   string
	TargetWorkName string
	RequiredState  string
	RequestID      string
	TraceID        string
}

type RequestRecord struct {
	RequestID string
	Type      RequestType
	TraceID   string
	Source    string
	WorkItems []FactoryWorkItem
	Relations []FactoryRelation
}

type SubmittedWork struct {
	Name         string
	WorkTypeName string
	WorkID       string
}

type SubmitResult struct {
	RequestID    string
	TraceID      string
	WorkID       string
	Name         string
	WorkTypeName string
	Accepted     bool
	Works        []SubmittedWork
}

type PreparedFactoryRequestBatch struct {
	Request       Request
	CanonicalJSON []byte
}

type WorkRequestPreparation struct {
	Request           Request
	CanonicalJSON     []byte
	DefaultWorkTypeID string
}

type SingleWorkTarget struct {
	WorkID     string
	WorkTypeID string
}

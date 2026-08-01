package lineagegraph

import "encoding/json"

const (
	BatchRequestTypeFactoryRequestBatch = "FACTORY_REQUEST_BATCH"
	RelationDependsOn                   = "DEPENDS_ON"
	RelationParentChild                 = "PARENT_CHILD"
)

// WorkItem describes a unit of work at a point in lineage history.
type WorkItem struct {
	ID                       string
	WorkTypeID               string
	State                    string
	DisplayName              string
	ChainingTraceDepth       int
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	TraceID                  string
	Content                  []ContentPart
	ParentID                 string
	PlaceID                  string
	Tags                     map[string]string
}

// ContentPart is the lineage-owned copy of canonical work content.
type ContentPart struct {
	Type        string
	Text        string
	URL         string
	File        string
	JSON        json.RawMessage
	Slot        string
	Label       string
	Role        string
	ContentType string
	ArtifactID  string
	Metadata    map[string]any
}

// BatchRequest is the parsed batch shape consumed by dependency-graph derivation.
type BatchRequest struct {
	RequestID string
	Type      string
	Works     []BatchWork
	Relations []BatchRelation
}

// BatchWork is one item inside a BatchRequest.
type BatchWork struct {
	Name       string
	WorkID     string
	WorkTypeID string
}

// BatchRelation is one declared relationship inside a BatchRequest.
type BatchRelation struct {
	Type           string
	SourceWorkName string
	TargetWorkName string
}

package interfaces

import "time"

// DataType distinguishes resource tokens from work tokens.
type DataType string

const (
	DataTypeResource DataType = "resource"
	DataTypeWork     DataType = "work"
)

// TokenColor carries the domain data attached to a token.
type TokenColor struct {
	Name                     string            `json:"name"`
	RequestID                string            `json:"request_id"`
	WorkID                   string            `json:"work_id"`
	WorkTypeID               string            `json:"work_type_id"`
	DataType                 DataType          `json:"data_type"`
	ChainingTraceDepth       int               `json:"chaining_trace_depth,omitempty"`
	CurrentChainingTraceID   string            `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previous_chaining_trace_ids,omitempty"`
	TraceID                  string            `json:"trace_id"`
	ParentID                 string            `json:"parent_id"`
	Tags                     map[string]string `json:"tags"`
	Relations                []Relation        `json:"relations"`
	Payload                  []byte            `json:"payload"`
}

// Token is a colored token: a work item or resource with data flowing through
// the net.
type Token struct {
	ID        string       `json:"id"`
	PlaceID   string       `json:"place_id"`
	Color     TokenColor   `json:"color"`
	CreatedAt time.Time    `json:"created_at"`
	EnteredAt time.Time    `json:"entered_at"`
	History   TokenHistory `json:"history"`
}

// TokenHistory tracks a token's journey through the net.
type TokenHistory struct {
	TotalVisits         map[string]int  `json:"total_visits"`
	ConsecutiveFailures map[string]int  `json:"consecutive_failures"`
	PlaceVisits         map[string]int  `json:"place_visits"`
	TotalDuration       time.Duration   `json:"total_duration"`
	LastError           string          `json:"last_error"`
	FailureLog          []FailureRecord `json:"failure_log"`
}

// FailureRecord captures a single failure event for a token.
type FailureRecord struct {
	TransitionID string    `json:"transition_id"`
	Timestamp    time.Time `json:"timestamp"`
	Error        string    `json:"error"`
	Attempt      int       `json:"attempt"`
}

// Relation defines a typed relationship between work items.
type Relation struct {
	Type          RelationType `json:"type"`
	TargetWorkID  string       `json:"target_work_id"`
	RequiredState string       `json:"required_state,omitempty"`
}

// RelationType classifies the relationship between two work items.
type RelationType string

const (
	RelationDependsOn   RelationType = "DEPENDS_ON"
	RelationParentChild RelationType = "PARENT_CHILD"
	RelationSpawnedBy   RelationType = "SPAWNED_BY"
)

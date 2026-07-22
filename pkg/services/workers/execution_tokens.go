package workers

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// DataType distinguishes runtime capacity inputs from dispatched Work inputs.
type DataType string

const (
	DataTypeResource DataType = "resource"
	DataTypeWork     DataType = "work"
)

// Color carries the provider-neutral Work data attached to a dispatched input.
type Color struct {
	Name                     string                    `json:"name"`
	RequestID                string                    `json:"request_id"`
	WorkID                   string                    `json:"work_id"`
	WorkTypeID               string                    `json:"work_type_id"`
	DataType                 DataType                  `json:"data_type"`
	ChainingTraceDepth       int                       `json:"chaining_trace_depth,omitempty"`
	CurrentChainingTraceID   string                    `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string                  `json:"previous_chaining_trace_ids,omitempty"`
	TraceID                  string                    `json:"trace_id"`
	ParentID                 string                    `json:"parent_id"`
	Tags                     map[string]string         `json:"tags"`
	Relations                []work.Relation           `json:"relations"`
	Content                  []work.WorkContentPart    `json:"content,omitempty"`
	Payload                  []byte                    `json:"payload"`
	InvocationArguments      *work.InvocationArguments `json:"-"`
}

// Token is the Worker-facing view of one runtime dispatch input.
type Token struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	Color     Color     `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	EnteredAt time.Time `json:"entered_at"`
	History   History   `json:"history"`
}

// History contains the execution history needed for Worker prompt policy.
type History struct {
	TotalVisits         map[string]int `json:"total_visits"`
	ConsecutiveFailures map[string]int `json:"consecutive_failures"`
	PlaceVisits         map[string]int `json:"place_visits"`
	TotalDuration       time.Duration  `json:"total_duration"`
	LastError           string         `json:"last_error"`
	FailureLog          []Failure      `json:"failure_log"`
}

// Failure is one prior failed execution attempt exposed to Worker policy.
type Failure struct {
	TransitionID string    `json:"transition_id"`
	Timestamp    time.Time `json:"timestamp"`
	Error        string    `json:"error"`
	Attempt      int       `json:"attempt"`
}

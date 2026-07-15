// Package token owns the Factory token contracts that flow through runtime
// dispatch and worker-result boundaries.
package token

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/work"
)

// DataType distinguishes resource tokens from work tokens.
type DataType string

const (
	DataTypeResource DataType = "resource"
	DataTypeWork     DataType = "work"
)

// Color carries the domain data attached to a token.
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

// Token is a colored work item or resource flowing through the Factory net.
type Token struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	Color     Color     `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	EnteredAt time.Time `json:"entered_at"`
	History   History   `json:"history"`
}

// History tracks a token's journey through the net.
type History struct {
	TotalVisits         map[string]int `json:"total_visits"`
	ConsecutiveFailures map[string]int `json:"consecutive_failures"`
	PlaceVisits         map[string]int `json:"place_visits"`
	TotalDuration       time.Duration  `json:"total_duration"`
	LastError           string         `json:"last_error"`
	FailureLog          []Failure      `json:"failure_log"`
}

// Failure captures a single failed transition attempt for a token.
type Failure struct {
	TransitionID string    `json:"transition_id"`
	Timestamp    time.Time `json:"timestamp"`
	Error        string    `json:"error"`
	Attempt      int       `json:"attempt"`
}

// ClearGuardBlockingFields resets guard counters while retaining failure history.
func ClearGuardBlockingFields(history *History) {
	if history == nil {
		return
	}
	history.TotalVisits = make(map[string]int)
	history.ConsecutiveFailures = make(map[string]int)
	history.PlaceVisits = make(map[string]int)
}

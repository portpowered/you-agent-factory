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

func PreviousChainingTraceIDs(tokens []Token) []string {
	colors := make([]Color, len(tokens))
	for i := range tokens {
		colors[i] = tokens[i].Color
	}
	return PreviousChainingTraceIDsFromColors(colors)
}

func PreviousChainingTraceIDsFromColors(colors []Color) []string {
	traceIDs := make([]string, 0, len(colors))
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyChainingTrace(color.CurrentChainingTraceID, color.TraceID))
	}
	return work.CanonicalChainingTraceIDs(traceIDs)
}

func CurrentChainingTraceID(tokens []Token, ignoredWorkTypeIDs ...string) string {
	colors := make([]Color, len(tokens))
	for i := range tokens {
		colors[i] = tokens[i].Color
	}
	return CurrentChainingTraceIDFromColors(colors, ignoredWorkTypeIDs...)
}

func CurrentChainingTraceIDFromColors(colors []Color, ignoredWorkTypeIDs ...string) string {
	for _, color := range colors {
		if color.DataType == DataTypeResource || containsWorkTypeID(ignoredWorkTypeIDs, color.WorkTypeID) {
			continue
		}
		return firstNonEmptyChainingTrace(color.CurrentChainingTraceID, color.TraceID)
	}
	for _, color := range colors {
		if color.DataType != DataTypeResource {
			return firstNonEmptyChainingTrace(color.CurrentChainingTraceID, color.TraceID)
		}
	}
	return ""
}

func ChainingTraceDepthFromColors(colors []Color) int {
	depth := 0
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		candidate := color.ChainingTraceDepth
		if candidate == 0 && firstNonEmptyChainingTrace(color.CurrentChainingTraceID, color.TraceID) != "" {
			candidate = 1
		}
		if candidate > depth {
			depth = candidate
		}
	}
	if depth > 0 {
		return depth + 1
	}
	return 0
}

func firstNonEmptyChainingTrace(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func containsWorkTypeID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

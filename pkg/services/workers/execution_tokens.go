package workers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
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
	StructuredResult         any                       `json:"structured_result,omitempty"`
	InvocationArguments      *work.InvocationArguments `json:"-"`
	// StructuredResultPresent distinguishes a present JSON null from an absent
	// result in token/checkpoint serialization.
	StructuredResultPresent bool `json:"-"`
}

// MarshalJSON preserves an explicitly present structured result, including
// JSON null, without making unstructured tokens emit the optional field.
func (value Color) MarshalJSON() ([]byte, error) {
	type alias Color
	return jsonvalue.MarshalOptionalField(alias(value), value.StructuredResult, value.StructuredResultPresent, "structured_result")
}

// UnmarshalJSON restores structured-result presence from token snapshots.
func (value *Color) UnmarshalJSON(data []byte) error {
	type alias Color
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	structured, present, err := jsonvalue.UnmarshalOptionalField(data, "structured_result")
	if err != nil {
		return err
	}
	*value = Color(decoded)
	value.StructuredResult = structured
	value.StructuredResultPresent = present
	return nil
}

// Token is the Worker-facing view of one Work dispatch input. Runtime place
// identifiers are translated into State before this value reaches Workers.
type Token struct {
	ID        string    `json:"id"`
	State     string    `json:"state,omitempty"`
	Color     Color     `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	EnteredAt time.Time `json:"entered_at"`
	History   History   `json:"history"`
}

// UnmarshalJSON keeps persisted dispatch inputs readable after State replaced
// the public place identifier. Historical snapshots used place_id for the
// same authored state, so decode that value only at this compatibility seam.
func (value *Token) UnmarshalJSON(data []byte) error {
	type tokenAlias Token
	var decoded tokenAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var legacy struct {
		PlaceID string `json:"place_id"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*value = Token(decoded)
	if strings.TrimSpace(value.State) == "" {
		value.State = stateFromLegacyPlaceID(legacy.PlaceID)
	}
	return nil
}

func stateFromLegacyPlaceID(placeID string) string {
	trimmed := strings.TrimSpace(placeID)
	if index := strings.LastIndexByte(trimmed, ':'); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

// History contains the execution history needed for Worker prompt policy.
type History struct {
	TotalVisits         map[string]int `json:"total_visits"`
	ConsecutiveFailures map[string]int `json:"consecutive_failures"`
	PlaceVisits         map[string]int `json:"place_visits"`
	TotalDuration       time.Duration  `json:"total_duration"`
	LastError           string         `json:"last_error"`
	FailureLog          []Failure      `json:"failure_log"`
	// FailureLogDroppedCount records failures omitted by a persistence-only
	// retention boundary. It is zero for live, unbounded histories and for
	// histories that fit within the configured durable snapshot capacity.
	FailureLogDroppedCount int `json:"failure_log_dropped_count,omitempty"`
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

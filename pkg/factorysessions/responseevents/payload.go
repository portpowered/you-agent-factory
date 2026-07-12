package responseevents

import "encoding/json"

// ContentBlockKind identifies one provider-neutral message content block.
type ContentBlockKind string

const (
	ContentBlockText              ContentBlockKind = "TEXT"
	ContentBlockReasoningSummary  ContentBlockKind = "REASONING_SUMMARY"
	ContentBlockToolRequest       ContentBlockKind = "TOOL_REQUEST"
	ContentBlockImageRef          ContentBlockKind = "IMAGE_REF"
	ContentBlockResourceRef       ContentBlockKind = "RESOURCE_REF"
	ContentBlockStructuredOutput  ContentBlockKind = "STRUCTURED_OUTPUT"
)

// ContentBlock carries one typed slice of assistant-visible message content.
type ContentBlock struct {
	Kind               ContentBlockKind `json:"kind"`
	Text               string           `json:"text,omitempty"`
	ToolCallID         string           `json:"toolCallId,omitempty"`
	ToolName           string           `json:"toolName,omitempty"`
	ArgumentsSummary   json.RawMessage  `json:"argumentsSummary,omitempty"`
	ImageRef           string           `json:"imageRef,omitempty"`
	ResourceRef        string           `json:"resourceRef,omitempty"`
	StructuredOutput   json.RawMessage  `json:"structuredOutput,omitempty"`
}

// SessionPayload records session-scoped lifecycle and capability metadata.
type SessionPayload struct {
	Status       string        `json:"status,omitempty"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}

// RunPayload records run-scoped lifecycle metadata.
type RunPayload struct {
	Status string `json:"status,omitempty"`
}

// TurnPayload records turn-scoped lifecycle metadata.
type TurnPayload struct {
	TurnIndex int    `json:"turnIndex,omitempty"`
	Status    string `json:"status,omitempty"`
}

// MessagePayload carries a message snapshot with typed content blocks.
type MessagePayload struct {
	Role          string         `json:"role"`
	ContentBlocks []ContentBlock `json:"contentBlocks"`
}

// MessageDeltaPayload carries incremental message content for one block.
type MessageDeltaPayload struct {
	ContentBlockIndex int              `json:"contentBlockIndex"`
	ContentBlockKind  ContentBlockKind `json:"contentBlockKind"`
	TextDelta         string           `json:"textDelta,omitempty"`
}

// ReasoningPayload carries reasoning summary content or deltas.
type ReasoningPayload struct {
	Summary      string `json:"summary,omitempty"`
	SummaryDelta string `json:"summaryDelta,omitempty"`
}

// ToolPayload carries tool lifecycle metadata and bounded summaries.
type ToolPayload struct {
	ToolCallID         string          `json:"toolCallId"`
	ToolName           string          `json:"toolName"`
	Status             string          `json:"status,omitempty"`
	ArgumentsSummary   json.RawMessage `json:"argumentsSummary,omitempty"`
	ResultSummary      json.RawMessage `json:"resultSummary,omitempty"`
}

// ToolDeltaPayload carries incremental tool output.
type ToolDeltaPayload struct {
	ToolCallID  string `json:"toolCallId"`
	OutputDelta string `json:"outputDelta"`
}

// FileChangePayload records one observed file mutation.
type FileChangePayload struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Summary   string `json:"summary,omitempty"`
}

// PlanStep records one step in a published plan snapshot.
type PlanStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status,omitempty"`
}

// PlanPayload carries a plan update snapshot.
type PlanPayload struct {
	Steps   []PlanStep `json:"steps,omitempty"`
	Summary string     `json:"summary,omitempty"`
}

// ProgressPayload carries a coarse progress notification.
type ProgressPayload struct {
	Label           string   `json:"label"`
	Message         string   `json:"message,omitempty"`
	PercentComplete *float64 `json:"percentComplete,omitempty"`
}

// UsagePayload carries token or model usage accounting.
type UsagePayload struct {
	InputTokens  int64  `json:"inputTokens,omitempty"`
	OutputTokens int64  `json:"outputTokens,omitempty"`
	TotalTokens  int64  `json:"totalTokens,omitempty"`
	Model        string `json:"model,omitempty"`
}

// ErrorPayload carries a provider-neutral error with optional retry metadata.
type ErrorPayload struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable,omitempty"`
	RetryAfterSeconds *int64 `json:"retryAfterSeconds,omitempty"`
	RetryAttempt      *int   `json:"retryAttempt,omitempty"`
}

// StreamGapPayload records a discontinuity in the retained response-event stream.
type StreamGapPayload struct {
	FromSequence int64  `json:"fromSequence"`
	ToSequence   int64  `json:"toSequence"`
	Reason       string `json:"reason,omitempty"`
}

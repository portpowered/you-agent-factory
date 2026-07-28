// Package operatorsettingsmcp exposes MCP tool operations for Operator Settings
// service root contracts backed by an injected Operator Settings Service root.
package operatorsettingsmcp

import "encoding/json"

// Tool names use Operator Settings vocabulary and align with accepted root operations.
const (
	ToolLoadDocument         = "you.operator_settings.load_document"
	ToolApplyDocumentUpdate  = "you.operator_settings.apply_document_update"
	ToolResolveEffective     = "you.operator_settings.resolve_effective"
)

// Stable error envelope fields shared by every Operator Settings MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.details",
}

// ToolErrorEnvelope is the stable MCP failure shape for Operator Settings tools.
type ToolErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

// ToolResponse wraps one tool outcome with either a typed result or a stable error.
type ToolResponse[T any] struct {
	Result *T                 `json:"result,omitempty"`
	Error  *ToolErrorEnvelope `json:"error,omitempty"`
}

// ToolDefinition is one discoverable MCP tool with typed schemas and documented
// stable response fields for success and error envelopes.
type ToolDefinition struct {
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	InputSchema         map[string]any `json:"inputSchema"`
	OutputSchema        map[string]any `json:"outputSchema"`
	SuccessStableFields []string       `json:"successStableFields"`
	ErrorStableFields   []string       `json:"errorStableFields"`
}

// MarshalJSON encodes one tool definition for MCP hosts and mock clients.
func (t ToolDefinition) MarshalJSON() ([]byte, error) {
	type alias ToolDefinition
	return json.Marshal(alias(t))
}

// Package workmcp exposes MCP tool operations for Work service root contracts
// backed by an injected Work Service root.
package workmcp

import "encoding/json"

// Tool names use Work vocabulary and align with accepted root operations.
const (
	ToolSubmit = "you.work.submit"
	ToolList   = "you.work.list"
	ToolGet    = "you.work.get"
	ToolMove   = "you.work.move"
)

// Stable error envelope fields shared by every Work MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.details",
}

// ToolErrorEnvelope is the stable MCP failure shape for Work tools.
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

const (
	// SuccessTextEncodingSerializedJSON documents serialized JSON tool payloads in text content.
	SuccessTextEncodingSerializedJSON = "serialized-json"
)

// EncodeSuccessCallToolResult builds the live MCP tools/call success envelope for one
// serialized tool-response payload. Callers use this shape to document current server
// transport without mutating handlers.
func EncodeSuccessCallToolResult(toolResponse json.RawMessage) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(toolResponse),
			},
		},
	}
}

// MarshalSuccessCallToolResultJSON encodes one success CallToolResult with stable key order.
func MarshalSuccessCallToolResultJSON(toolResponse json.RawMessage) ([]byte, error) {
	return json.Marshal(EncodeSuccessCallToolResult(toolResponse))
}

// Package mcp exposes MCP tool operations for Factory Runtime backed by the
// accepted factory.Service root contract.
package mcp

import "encoding/json"

// Tool names use Factory Runtime vocabulary for owner-local discovery.
const (
	ToolControlPause = "you.factory_runtime.control_pause"
	ToolObserve      = "you.factory_runtime.observe"
)

// Stable error envelope fields shared by every Factory Runtime MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.details",
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

// ToolErrorEnvelope is the stable MCP failure shape for Factory Runtime tools.
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

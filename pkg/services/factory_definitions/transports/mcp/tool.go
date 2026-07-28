// Package factorydefinition exposes MCP tool operations for Factory Definitions
// backed by the accepted Definitions root contract.
package factorydefinition

import (
	"encoding/json"
)

// Tool names use Factory Definition vocabulary and align with HTTP-DEF routes.
const (
	ToolValidate        = "you.factory_definition.validate"
	ToolGetCurrent      = "you.factory_definition.get_current"
	ToolSaveCurrent     = "you.factory_definition.save_current"
	ToolInstallPackaged = "you.factory_definition.install_packaged"
)

// Stable error envelope fields shared by every Factory Definition MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.details",
}

// ToolErrorEnvelope is the stable MCP failure shape for Factory Definition tools.
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

// MarshalJSON encodes one tool definition for MCP hosts and mock clients.
func (t ToolDefinition) MarshalJSON() ([]byte, error) {
	type alias ToolDefinition
	return json.Marshal(alias(t))
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

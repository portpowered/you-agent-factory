// Package mcp exposes MCP tool operations for Factory Runtime backed by the
// accepted factory.Service root contract.
package mcp

// Tool names use Factory Runtime vocabulary for owner-local discovery.
const (
	ToolControlPause = "you.factory_runtime.control_pause"
	ToolObserve      = "you.factory_runtime.observe"
)

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

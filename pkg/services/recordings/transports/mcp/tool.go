// Package recordingmcp exposes MCP tool operations for Recordings service root
// contracts backed by an injected Recordings Service root.
package recordingmcp

import "encoding/json"

// Tool names use Recordings vocabulary and align with accepted root operations.
const (
	ToolQueryStatus           = "you.recording.query_status"
	ToolAppendEvent           = "you.recording.append_event"
	ToolLoadReplay            = "you.recording.load_replay"
	ToolReadPortableArtifact  = "you.recording.read_portable_artifact"
)

// ToolErrorEnvelope is the stable MCP failure shape for Recordings tools.
type ToolErrorEnvelope struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Retryable  bool           `json:"retryable"`
	RecordingID string        `json:"recordingId,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
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

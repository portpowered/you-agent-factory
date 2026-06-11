package factorysession

import (
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// Client is a deterministic mock MCP client for Factory Session tool calls.
type Client struct{}

// NewClient constructs one mock MCP client backed by in-process tool handlers.
func NewClient() *Client {
	return &Client{}
}

// CallTool invokes one discovered Factory Session MCP tool by stable name.
// Workflow-named compatibility aliases resolve to the same canonical handlers.
func (c *Client) CallTool(name string, input json.RawMessage) (json.RawMessage, error) {
	switch ResolveToolName(name) {
	case ToolValidateSource:
		var request factoryapi.FactoryPreviewRequest
		if err := json.Unmarshal(input, &request); err != nil {
			return nil, fmt.Errorf("decode validate source input: %w", err)
		}
		response := ValidateSource(request)
		return json.Marshal(response)
	default:
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
}

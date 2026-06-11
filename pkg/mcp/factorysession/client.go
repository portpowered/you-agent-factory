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
func (c *Client) CallTool(name string, input json.RawMessage) (json.RawMessage, error) {
	switch name {
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

// ValidateSource calls you.factory_session.validate_source through the mock client.
func (c *Client) ValidateSource(input factoryapi.FactoryPreviewRequest) (ToolResponse[factoryapi.FactoryPreviewResult], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.FactoryPreviewResult]{}, err
	}
	raw, err := c.CallTool(ToolValidateSource, encoded)
	if err != nil {
		return ToolResponse[factoryapi.FactoryPreviewResult]{}, err
	}
	var response ToolResponse[factoryapi.FactoryPreviewResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.FactoryPreviewResult]{}, err
	}
	return response, nil
}

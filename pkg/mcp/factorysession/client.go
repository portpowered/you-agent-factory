package factorysession

import (
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// Client is a deterministic mock MCP client for Factory Session tool calls.
type Client struct {
	service factorysessionexecution.Service
}

// NewClient constructs one mock MCP client backed by in-process tool handlers.
func NewClient() *Client {
	return &Client{}
}

// NewClientWithService constructs one mock MCP client backed by the supplied
// durable Factory Session execution service.
func NewClientWithService(service factorysessionexecution.Service) *Client {
	return &Client{service: service}
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
	case ToolStartSync:
		var request factoryapi.FactorySessionExecutionRequest
		if err := json.Unmarshal(input, &request); err != nil {
			return nil, fmt.Errorf("decode start sync input: %w", err)
		}
		response := StartSync(c.service, request)
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

// StartSync calls you.factory_session.start_sync through the mock client.
func (c *Client) StartSync(input factoryapi.FactorySessionExecutionRequest) (ToolResponse[factoryapi.FactorySessionSyncExecutionResponse], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{}, err
	}
	raw, err := c.CallTool(ToolStartSync, encoded)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{}, err
	}
	var response ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{}, err
	}
	return response, nil
}

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

func marshalToolCall[T any](input json.RawMessage, decodeMsg string, invoke func(T) any) (json.RawMessage, error) {
	var request T
	if err := json.Unmarshal(input, &request); err != nil {
		return nil, fmt.Errorf("%s: %w", decodeMsg, err)
	}
	return json.Marshal(invoke(request))
}

// CallTool invokes one discovered Factory Session MCP tool by stable name.
func (c *Client) CallTool(name string, input json.RawMessage) (json.RawMessage, error) {
	switch name {
	case ToolListSessions:
		return marshalToolCall(input, "decode list sessions input", func(request ListSessionsInput) any {
			return ListSessions(c.service, request)
		})
	case ToolValidateSource:
		return marshalToolCall(input, "decode validate source input", func(request factoryapi.FactoryPreviewRequest) any {
			return ValidateSource(request)
		})
	case ToolStartSync:
		return marshalToolCall(input, "decode start sync input", func(request factoryapi.FactorySessionExecutionRequest) any {
			return StartSync(c.service, request)
		})
	case ToolStartAsync:
		return marshalToolCall(input, "decode start async input", func(request factoryapi.FactorySessionExecutionRequest) any {
			return StartAsync(c.service, request)
		})
	case ToolGetSession:
		return marshalToolCall(input, "decode get session input", func(request GetSessionInput) any {
			return GetSession(c.service, request)
		})
	case ToolGetResult:
		return marshalToolCall(input, "decode get result input", func(request GetResultInput) any {
			return GetResult(c.service, request)
		})
	case ToolListDispatches:
		return marshalToolCall(input, "decode list dispatches input", func(request ListDispatchesInput) any {
			return ListDispatches(c.service, request)
		})
	case ToolListArtifacts:
		return marshalToolCall(input, "decode list artifacts input", func(request ListArtifactsInput) any {
			return ListArtifacts(c.service, request)
		})
	default:
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
}

// ListSessions calls you.factory_session.list through the mock client.
func (c *Client) ListSessions(input ListSessionsInput) (ToolResponse[factoryapi.ListFactorySessionsResponse], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{}, err
	}
	raw, err := c.CallTool(ToolListSessions, encoded)
	if err != nil {
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{}, err
	}
	var response ToolResponse[factoryapi.ListFactorySessionsResponse]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{}, err
	}
	return response, nil
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

// StartAsync calls you.factory_session.start_async through the mock client.
func (c *Client) StartAsync(input factoryapi.FactorySessionExecutionRequest) (ToolResponse[factoryapi.FactorySessionExecutionResponse], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{}, err
	}
	raw, err := c.CallTool(ToolStartAsync, encoded)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{}, err
	}
	var response ToolResponse[factoryapi.FactorySessionExecutionResponse]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{}, err
	}
	return response, nil
}

// GetSession calls you.factory_session.get through the mock client.
func (c *Client) GetSession(input GetSessionInput) (ToolResponse[factoryapi.FactorySessionDurableReadModel], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{}, err
	}
	raw, err := c.CallTool(ToolGetSession, encoded)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{}, err
	}
	var response ToolResponse[factoryapi.FactorySessionDurableReadModel]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{}, err
	}
	return response, nil
}

// GetResult calls you.factory_session.get_result through the mock client.
func (c *Client) GetResult(input GetResultInput) (ToolResponse[factoryapi.FactorySessionResult], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionResult]{}, err
	}
	raw, err := c.CallTool(ToolGetResult, encoded)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionResult]{}, err
	}
	var response ToolResponse[factoryapi.FactorySessionResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.FactorySessionResult]{}, err
	}
	return response, nil
}

// ListDispatches calls you.factory_session.list_dispatches through the mock client.
func (c *Client) ListDispatches(input ListDispatchesInput) (ToolResponse[factoryapi.ListFactorySessionDispatchesResponse], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{}, err
	}
	raw, err := c.CallTool(ToolListDispatches, encoded)
	if err != nil {
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{}, err
	}
	var response ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{}, err
	}
	return response, nil
}

// ListArtifacts calls you.factory_session.list_artifacts through the mock client.
func (c *Client) ListArtifacts(input ListArtifactsInput) (ToolResponse[factoryapi.ListFactorySessionArtifactsResponse], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{}, err
	}
	raw, err := c.CallTool(ToolListArtifacts, encoded)
	if err != nil {
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{}, err
	}
	var response ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{}, err
	}
	return response, nil
}

package factorysession

import (
	"encoding/json"
	"fmt"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

func callToolJSON[Input any, Response any](
	input json.RawMessage,
	decodeErr string,
	handler func(Input) Response,
) (json.RawMessage, error) {
	var request Input
	if err := json.Unmarshal(input, &request); err != nil {
		return nil, fmt.Errorf("%s: %w", decodeErr, err)
	}
	return json.Marshal(handler(request))
}

type canonicalToolHandler func(*Client, json.RawMessage) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolListSessions: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list sessions input", func(request ListSessionsInput) ToolResponse[factoryapi.ListFactorySessionsResponse] {
			return ListSessions(c.service, request)
		})
	},
	ToolValidateSource: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode validate source input", ValidateSource)
	},
	ToolStartSync: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start sync input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionSyncExecutionResponse] {
			return StartSync(c.service, request)
		})
	},
	ToolStartAsync: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start async input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionExecutionResponse] {
			return StartAsync(c.service, request)
		})
	},
	ToolGetSession: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get session input", func(request GetSessionInput) ToolResponse[factoryapi.FactorySessionDurableReadModel] {
			return GetSession(c.service, request)
		})
	},
	ToolGetResult: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get result input", func(request GetResultInput) ToolResponse[factoryapi.FactorySessionResult] {
			return GetResult(c.service, request)
		})
	},
	ToolListDispatches: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list dispatches input", func(request ListDispatchesInput) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
			return ListDispatches(c.service, request)
		})
	},
	ToolListArtifacts: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list artifacts input", func(request ListArtifactsInput) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
			return ListArtifacts(c.service, request)
		})
	},
	ToolReadEvents: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode read events input", func(request ReadEventsInput) ToolResponse[ReadEventsResult] {
			return ReadEvents(c.service, request)
		})
	},
	ToolControl: func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode control input", func(request ControlInput) ToolResponse[factoryapi.FactorySessionLifecycleControlResponse] {
			return Control(c.service, request)
		})
	},
}

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Factory Session tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

// CallTool invokes one discovered Factory Session MCP tool by stable name.
// Workflow-named compatibility aliases resolve to the same canonical handlers.
func (c *Client) CallTool(name string, input json.RawMessage) (json.RawMessage, error) {
	resolved := ResolveToolName(name)
	handler, ok := canonicalToolHandlers[resolved]
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
	return handler(c, input)
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

// ReadEvents calls you.factory_session.read_events through the mock client.
func (c *Client) ReadEvents(input ReadEventsInput) (ToolResponse[ReadEventsResult], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[ReadEventsResult]{}, err
	}
	raw, err := c.CallTool(ToolReadEvents, encoded)
	if err != nil {
		return ToolResponse[ReadEventsResult]{}, err
	}
	var response ToolResponse[ReadEventsResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[ReadEventsResult]{}, err
	}
	return response, nil
}

// Control calls you.factory_session.control through the mock client.
func (c *Client) Control(input ControlInput) (ToolResponse[factoryapi.FactorySessionLifecycleControlResponse], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{}, err
	}
	raw, err := c.CallTool(ToolControl, encoded)
	if err != nil {
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{}, err
	}
	var response ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]
	if err := json.Unmarshal(raw, &response); err != nil {
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{}, err
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

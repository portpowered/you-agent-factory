package factorysession

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
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

type canonicalToolBinding struct {
	handlerID string
	handler   canonicalToolHandler
}

// ToolHandlerBinding identifies the contracted tool and handwritten handler
// selected for one canonical tool name or compatibility alias.
type ToolHandlerBinding struct {
	ToolID    string
	HandlerID string
}

// ProjectCanonicalToolHandlerBindings returns the handwritten stable-ID
// registry as a sorted, read-only identity projection. It deliberately omits
// executable handler functions and compatibility aliases.
func ProjectCanonicalToolHandlerBindings() []ToolHandlerBinding {
	bindings := make([]ToolHandlerBinding, 0, len(canonicalToolHandlersByID))
	for toolID, binding := range canonicalToolHandlersByID {
		bindings = append(bindings, ToolHandlerBinding{
			ToolID:    toolID,
			HandlerID: binding.handlerID,
		})
	}
	slices.SortFunc(bindings, func(left, right ToolHandlerBinding) int {
		if left.ToolID != right.ToolID {
			return strings.Compare(left.ToolID, right.ToolID)
		}
		return strings.Compare(left.HandlerID, right.HandlerID)
	})
	return bindings
}

const (
	stableToolIDPrefix    = "mcp.tool."
	stableHandlerIDPrefix = "mcp.handler."
)

// Handwritten handlers stay keyed by stable catalog tool IDs. Handler IDs are
// recorded alongside them so catalog identity never moves business logic into
// generated discovery code.
var canonicalToolHandlersByID = map[string]canonicalToolBinding{
	stableToolID(ToolListSessions): handwrittenToolBinding(ToolListSessions, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list sessions input", func(request ListSessionsInput) ToolResponse[factoryapi.ListFactorySessionsResponse] {
			return ListSessions(c.service, request)
		})
	}),
	stableToolID(ToolValidateSource): handwrittenToolBinding(ToolValidateSource, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode validate source input", ValidateSource)
	}),
	stableToolID(ToolStartSync): handwrittenToolBinding(ToolStartSync, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start sync input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionSyncExecutionResponse] {
			return StartSync(c.service, request)
		})
	}),
	stableToolID(ToolStartAsync): handwrittenToolBinding(ToolStartAsync, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start async input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionExecutionResponse] {
			return StartAsync(c.service, request)
		})
	}),
	stableToolID(ToolGetSession): handwrittenToolBinding(ToolGetSession, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get session input", func(request GetSessionInput) ToolResponse[factoryapi.FactorySessionDurableReadModel] {
			return GetSession(c.service, request)
		})
	}),
	stableToolID(ToolGetResult): handwrittenToolBinding(ToolGetResult, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get result input", func(request GetResultInput) ToolResponse[factoryapi.FactorySessionResult] {
			return GetResult(c.service, request)
		})
	}),
	stableToolID(ToolListDispatches): handwrittenToolBinding(ToolListDispatches, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list dispatches input", func(request ListDispatchesInput) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
			return ListDispatches(c.service, request)
		})
	}),
	stableToolID(ToolListArtifacts): handwrittenToolBinding(ToolListArtifacts, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list artifacts input", func(request ListArtifactsInput) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
			return ListArtifacts(c.service, request)
		})
	}),
	stableToolID(ToolReadEvents): handwrittenToolBinding(ToolReadEvents, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode read events input", func(request ReadEventsInput) ToolResponse[ReadEventsResult] {
			return ReadEvents(c.service, request)
		})
	}),
	stableToolID(ToolControl): handwrittenToolBinding(ToolControl, func(c *Client, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode control input", func(request ControlInput) ToolResponse[factoryapi.FactorySessionLifecycleControlResponse] {
			return Control(c.service, request)
		})
	}),
}

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Factory Session tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	if ResolveToolName(name) != name {
		return false
	}
	_, ok := ResolveToolHandlerBinding(name)
	return ok
}

// ResolveToolHandlerBinding resolves a canonical name or compatibility alias
// through generated catalog identity into the handwritten stable-ID registry.
func ResolveToolHandlerBinding(name string) (ToolHandlerBinding, bool) {
	canonicalName := ResolveToolName(name)
	toolID, ok := generatedToolIDByName(canonicalName)
	if !ok {
		return ToolHandlerBinding{}, false
	}
	binding, ok := canonicalToolHandlersByID[toolID]
	if !ok {
		return ToolHandlerBinding{}, false
	}
	return ToolHandlerBinding{ToolID: toolID, HandlerID: binding.handlerID}, true
}

// CallTool invokes one discovered Factory Session MCP tool by stable name.
// Workflow-named compatibility aliases resolve to the same canonical handlers.
func (c *Client) CallTool(name string, input json.RawMessage) (json.RawMessage, error) {
	bindingIdentity, ok := ResolveToolHandlerBinding(name)
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
	binding := canonicalToolHandlersByID[bindingIdentity.ToolID]
	return binding.handler(c, input)
}

func generatedToolIDByName(name string) (string, bool) {
	for _, tool := range mcpgenerated.PrimaryDiscovery() {
		if tool.Name == name {
			return tool.ID, true
		}
	}
	return "", false
}

func stableToolID(name string) string {
	return stableToolIDPrefix + name
}

func stableHandlerID(name string) string {
	return stableHandlerIDPrefix + name
}

func handwrittenToolBinding(name string, handler canonicalToolHandler) canonicalToolBinding {
	return canonicalToolBinding{handlerID: stableHandlerID(name), handler: handler}
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

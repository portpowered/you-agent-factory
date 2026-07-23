package factorysession_test

import (
	"context"
	"encoding/json"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// testClient is a test-local typed script over the production ToolOperation.
// It deliberately is not a production MCP entrypoint or composition surface.
type testClient struct {
	call mcpfactorysession.ToolOperation
}

func newTestClient() *testClient {
	return &testClient{call: mcpfactorysession.BindToolOperation(nil, nil, nil)}
}

func newTestClientWithWorkflows(workflows factoryruntime.WorkflowPreviewOperation) *testClient {
	return &testClient{call: mcpfactorysession.BindToolOperation(nil, nil, workflows)}
}

func newTestClientWithService(
	service factorysessions.ExecutionService,
	prepare mcpfactorysession.RequestPreparation,
	workflows ...factoryruntime.WorkflowPreviewOperation,
) *testClient {
	var workflow factoryruntime.WorkflowPreviewOperation
	if len(workflows) > 0 {
		workflow = workflows[0]
	}
	return &testClient{call: mcpfactorysession.BindToolOperation(service, prepare, workflow)}
}

func (c *testClient) CallTool(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	return c.call(ctx, name, input)
}

func callTestTool[Input, Output any](
	ctx context.Context,
	client *testClient,
	name string,
	input Input,
) (mcpfactorysession.ToolResponse[Output], error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return mcpfactorysession.ToolResponse[Output]{}, err
	}
	raw, err := client.CallTool(ctx, name, encoded)
	if err != nil {
		return mcpfactorysession.ToolResponse[Output]{}, err
	}
	var response mcpfactorysession.ToolResponse[Output]
	if err := json.Unmarshal(raw, &response); err != nil {
		return mcpfactorysession.ToolResponse[Output]{}, err
	}
	return response, nil
}

func (c *testClient) ListSessions(ctx context.Context, input mcpfactorysession.ListSessionsInput) (mcpfactorysession.ToolResponse[factoryapi.ListFactorySessionsResponse], error) {
	return callTestTool[mcpfactorysession.ListSessionsInput, factoryapi.ListFactorySessionsResponse](ctx, c, mcpfactorysession.ToolListSessions, input)
}

func (c *testClient) StartSync(ctx context.Context, input factoryapi.FactorySessionExecutionRequest) (mcpfactorysession.ToolResponse[factoryapi.FactorySessionSyncExecutionResponse], error) {
	return callTestTool[factoryapi.FactorySessionExecutionRequest, factoryapi.FactorySessionSyncExecutionResponse](ctx, c, mcpfactorysession.ToolStartSync, input)
}

func (c *testClient) StartAsync(ctx context.Context, input factoryapi.FactorySessionExecutionRequest) (mcpfactorysession.ToolResponse[factoryapi.FactorySessionExecutionResponse], error) {
	return callTestTool[factoryapi.FactorySessionExecutionRequest, factoryapi.FactorySessionExecutionResponse](ctx, c, mcpfactorysession.ToolStartAsync, input)
}

func (c *testClient) GetSession(ctx context.Context, input mcpfactorysession.GetSessionInput) (mcpfactorysession.ToolResponse[factoryapi.FactorySessionDurableReadModel], error) {
	return callTestTool[mcpfactorysession.GetSessionInput, factoryapi.FactorySessionDurableReadModel](ctx, c, mcpfactorysession.ToolGetSession, input)
}

func (c *testClient) ListDispatches(ctx context.Context, input mcpfactorysession.ListDispatchesInput) (mcpfactorysession.ToolResponse[factoryapi.ListFactorySessionDispatchesResponse], error) {
	return callTestTool[mcpfactorysession.ListDispatchesInput, factoryapi.ListFactorySessionDispatchesResponse](ctx, c, mcpfactorysession.ToolListDispatches, input)
}

func (c *testClient) ListArtifacts(ctx context.Context, input mcpfactorysession.ListArtifactsInput) (mcpfactorysession.ToolResponse[factoryapi.ListFactorySessionArtifactsResponse], error) {
	return callTestTool[mcpfactorysession.ListArtifactsInput, factoryapi.ListFactorySessionArtifactsResponse](ctx, c, mcpfactorysession.ToolListArtifacts, input)
}

func (c *testClient) ReadEvents(ctx context.Context, input mcpfactorysession.ReadEventsInput) (mcpfactorysession.ToolResponse[mcpfactorysession.ReadEventsResult], error) {
	return callTestTool[mcpfactorysession.ReadEventsInput, mcpfactorysession.ReadEventsResult](ctx, c, mcpfactorysession.ToolReadEvents, input)
}

func (c *testClient) Control(ctx context.Context, input mcpfactorysession.ControlInput) (mcpfactorysession.ToolResponse[factoryapi.FactorySessionLifecycleControlResponse], error) {
	return callTestTool[mcpfactorysession.ControlInput, factoryapi.FactorySessionLifecycleControlResponse](ctx, c, mcpfactorysession.ToolControl, input)
}

func (c *testClient) GetResult(ctx context.Context, input mcpfactorysession.GetResultInput) (mcpfactorysession.ToolResponse[factoryapi.FactorySessionResult], error) {
	return callTestTool[mcpfactorysession.GetResultInput, factoryapi.FactorySessionResult](ctx, c, mcpfactorysession.ToolGetResult, input)
}

package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
)

var errMissingRequestContext = errors.New("MCP request context is required")

type RequestPreparation interface {
	PrepareStart(factorysessionexecution.StartRequest) (factorysessionexecution.StartRequest, error)
	PrepareControl(factorysessionexecution.ControlRequest) (factorysessionexecution.ControlRequest, error)
	PrepareApprove(factorysessionexecution.ApproveRequest) (factorysessionexecution.ApproveRequest, error)
	PrepareRetryDispatch(factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.RetryDispatchRequest, error)
	PrepareInterruptDispatch(factorysessionexecution.InterruptDispatchRequest) (factorysessionexecution.InterruptDispatchRequest, error)
	PrepareListSessions(factorysessionexecution.ListSessionsRequest) (factorysessionexecution.ListSessionsRequest, error)
	PrepareResult(factorysessionexecution.ResultRequest) (factorysessionexecution.ResultRequest, error)
	PrepareEventReconnect(factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReconnectRequest, error)
}

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Factory Sessions dependencies once;
// protocol tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// RootDependencies are the accepted Factory Sessions root roles consumed by
// the MCP adapter. Execution is the durable-session execution slice of the
// singular Service root; transports inject an implementation or test fake
// rather than importing Sessions internals or constructing canonical state.
type RootDependencies struct {
	Execution factorysessionexecution.ExecutionService
	Prepare   RequestPreparation
	Workflows factoryruntime.WorkflowPreviewOperation
}

// Bind constructs the canonical ToolOperation from explicit Sessions root
// dependencies. Adapter tests replace Execution with a root-shaped fake
// without constructing real session durability or live runtime state.
func Bind(deps RootDependencies) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, deps.Execution, deps.Prepare, deps.Workflows, name, input)
	}
}

// BindToolOperation binds the canonical tool registry to explicit Factory
// Sessions and workflow roles without constructing an alternate MCP client.
func BindToolOperation(
	service factorysessionexecution.ExecutionService,
	prepare RequestPreparation,
	workflows factoryruntime.WorkflowPreviewOperation,
) ToolOperation {
	return Bind(RootDependencies{
		Execution: service,
		Prepare:   prepare,
		Workflows: workflows,
	})
}

func callToolJSON[Input any, Output any](
	input json.RawMessage,
	decodeErr string,
	handler func(Input) ToolResponse[Output],
) (json.RawMessage, error) {
	var request Input
	if err := json.Unmarshal(input, &request); err != nil {
		envelope := decodeInputErrorEnvelope(decodeErr, err)
		return json.Marshal(ToolResponse[Output]{Error: &envelope})
	}
	return json.Marshal(handler(request))
}

type canonicalToolHandler func(
	context.Context,
	factorysessionexecution.ExecutionService,
	RequestPreparation,
	factoryruntime.WorkflowPreviewOperation,
	json.RawMessage,
) (json.RawMessage, error)

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
	stableToolID(ToolListSessions): handwrittenToolBinding(ToolListSessions, func(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list sessions input", func(request ListSessionsInput) ToolResponse[factoryapi.ListFactorySessionsResponse] {
			return ListSessions(ctx, service, prepare, request)
		})
	}),
	stableToolID(ToolValidateSource): handwrittenToolBinding(ToolValidateSource, func(ctx context.Context, _ factorysessionexecution.ExecutionService, _ RequestPreparation, workflows factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode validate source input", func(request factoryapi.FactoryPreviewRequest) ToolResponse[factoryapi.FactoryPreviewResult] {
			return ValidateSource(ctx, workflows, request)
		})
	}),
	stableToolID(ToolStartSync): handwrittenToolBinding(ToolStartSync, func(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start sync input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionSyncExecutionResponse] {
			return StartSync(ctx, service, prepare, request)
		})
	}),
	stableToolID(ToolStartAsync): handwrittenToolBinding(ToolStartAsync, func(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start async input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionExecutionResponse] {
			return StartAsync(ctx, service, prepare, request)
		})
	}),
	stableToolID(ToolGetSession): handwrittenToolBinding(ToolGetSession, func(ctx context.Context, service factorysessionexecution.ExecutionService, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get session input", func(request GetSessionInput) ToolResponse[factoryapi.FactorySessionDurableReadModel] {
			return GetSession(ctx, service, request)
		})
	}),
	stableToolID(ToolGetResult): handwrittenToolBinding(ToolGetResult, func(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get result input", func(request GetResultInput) ToolResponse[factoryapi.FactorySessionResult] {
			return GetResult(ctx, service, prepare, request)
		})
	}),
	stableToolID(ToolListDispatches): handwrittenToolBinding(ToolListDispatches, func(ctx context.Context, service factorysessionexecution.ExecutionService, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list dispatches input", func(request ListDispatchesInput) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
			return ListDispatches(ctx, service, request)
		})
	}),
	stableToolID(ToolListArtifacts): handwrittenToolBinding(ToolListArtifacts, func(ctx context.Context, service factorysessionexecution.ExecutionService, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list artifacts input", func(request ListArtifactsInput) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
			return ListArtifacts(ctx, service, request)
		})
	}),
	stableToolID(ToolReadEvents): handwrittenToolBinding(ToolReadEvents, func(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode read events input", func(request ReadEventsInput) ToolResponse[ReadEventsResult] {
			return ReadEvents(ctx, service, prepare, request)
		})
	}),
	stableToolID(ToolControl): handwrittenToolBinding(ToolControl, func(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode control input", func(request ControlInput) ToolResponse[factoryapi.FactorySessionLifecycleControlResponse] {
			return Control(ctx, service, prepare, request)
		})
	}),
}

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Factory Session tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := ResolveToolHandlerBinding(name)
	return ok
}

// ResolveToolHandlerBinding resolves a canonical name through generated catalog
// identity into the handwritten stable-ID registry.
func ResolveToolHandlerBinding(name string) (ToolHandlerBinding, bool) {
	toolID, ok := generatedToolIDByName(name)
	if !ok {
		return ToolHandlerBinding{}, false
	}
	binding, ok := canonicalToolHandlersByID[toolID]
	if !ok {
		return ToolHandlerBinding{}, false
	}
	return ToolHandlerBinding{ToolID: toolID, HandlerID: binding.handlerID}, true
}

// CallTool invokes one Factory Session tool against explicitly supplied
// durable execution and workflow roles. Protocol servers receive the bound
// ToolOperation rather than choosing between construction paths.
func CallTool(
	ctx context.Context,
	service factorysessionexecution.ExecutionService,
	prepare RequestPreparation,
	workflows factoryruntime.WorkflowPreviewOperation,
	name string,
	input json.RawMessage,
) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("call MCP tool: %w", errMissingRequestContext)
	}
	bindingIdentity, ok := ResolveToolHandlerBinding(name)
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
	binding := canonicalToolHandlersByID[bindingIdentity.ToolID]
	return binding.handler(ctx, service, prepare, workflows, input)
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

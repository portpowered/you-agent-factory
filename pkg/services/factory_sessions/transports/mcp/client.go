package factorysession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
}

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Factory Sessions dependencies once;
// protocol tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// ProviderIdentityResolver canonicalizes one explicitly requested provider
// selection before a Factory Session starts. Production binds the
// authoritative Providers catalog so this adapter never owns a second list of
// accepted provider ids or aliases; protocol tests replace this exact function
// role.
type ProviderIdentityResolver func(context.Context, string) (string, error)

// BindToolOperation binds the canonical tool registry to explicit Factory
// Sessions and workflow roles without constructing an alternate MCP client.
// Adapter tests bind root-shaped fakes directly instead of constructing real
// session durability or live runtime state.
func BindToolOperation(
	recordingsService RecordingsInspection,
	prepare RequestPreparation,
	workflows factoryruntime.WorkflowPreviewOperation,
	sessions factorysessionexecution.Service,
	workingRoot string,
	generateID factorysessionexecution.SessionIDGenerator,
	resolveProvider ProviderIdentityResolver,
) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, prepare, workflows, name, input, recordingsService, sessions, workingRoot, generateID, resolveProvider)
	}
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

func callSubagentJSON(input json.RawMessage, handler func(SubagentInput) ToolResponse[SubagentResult]) (json.RawMessage, error) {
	var request SubagentInput
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		envelope := decodeInputErrorEnvelope("decode subagent input", err)
		return json.Marshal(ToolResponse[SubagentResult]{Error: &envelope})
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		envelope := decodeInputErrorEnvelope("decode subagent input", fmt.Errorf("unexpected trailing JSON value"))
		return json.Marshal(ToolResponse[SubagentResult]{Error: &envelope})
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		envelope := decodeInputErrorEnvelope("decode subagent input", err)
		return json.Marshal(ToolResponse[SubagentResult]{Error: &envelope})
	}
	if fields == nil {
		envelope := decodeInputErrorEnvelope("decode subagent input", errors.New("input must be an object"))
		return json.Marshal(ToolResponse[SubagentResult]{Error: &envelope})
	}
	for _, field := range []string{"prompt", "provider", "model", "reasoningEffort", "timeoutMillis", "workingRoot"} {
		if value, present := fields[field]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			envelope := decodeInputErrorEnvelope("decode subagent input", fmt.Errorf("%s must not be null", field))
			return json.Marshal(ToolResponse[SubagentResult]{Error: &envelope})
		}
	}
	return json.Marshal(handler(request))
}

// ValidateSubagentArguments applies RUN decoding before provider configuration.
// A nil response means validation succeeded; failures retain Factory errors.
func ValidateSubagentArguments(input json.RawMessage) (json.RawMessage, error) {
	response, err := callSubagentJSON(input, func(request SubagentInput) ToolResponse[SubagentResult] {
		if err := validateSubagentValues(request); err != nil {
			envelope := requestValidationErrorEnvelope(err)
			return ToolResponse[SubagentResult]{Error: &envelope}
		}
		return ToolResponse[SubagentResult]{}
	})
	if err != nil || string(response) != "{}" {
		return response, err
	}
	return nil, nil
}

type canonicalToolHandler func(
	context.Context,
	RequestPreparation,
	factoryruntime.WorkflowPreviewOperation,
	RecordingsInspection,
	factorysessionexecution.Service,
	string,
	factorysessionexecution.SessionIDGenerator,
	ProviderIdentityResolver,
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
	stableToolID(ToolListSessions): handwrittenToolBinding(ToolListSessions, func(ctx context.Context, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, sessions factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list sessions input", func(request ListSessionsInput) ToolResponse[factoryapi.ListFactorySessionsResponse] {
			return listSessionsCanonical(ctx, sessions, prepare, request)
		})
	}),
	stableToolID(ToolValidateSource): handwrittenToolBinding(ToolValidateSource, func(ctx context.Context, _ RequestPreparation, workflows factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, _ factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode validate source input", func(request factoryapi.FactoryPreviewRequest) ToolResponse[factoryapi.FactoryPreviewResult] {
			return ValidateSource(ctx, workflows, request)
		})
	}),
	stableToolID(ToolStartSync): handwrittenToolBinding(ToolStartSync, func(ctx context.Context, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, sessions factorysessionexecution.Service, workingRoot string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start sync input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionSyncExecutionResponse] {
			return startSyncCanonical(ctx, sessions, prepare, workingRoot, request)
		})
	}),
	stableToolID(ToolSubagent): handwrittenToolBinding(ToolSubagent, func(ctx context.Context, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, sessions factorysessionexecution.Service, workingRoot string, generateID factorysessionexecution.SessionIDGenerator, resolveProvider ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callSubagentJSON(input, func(request SubagentInput) ToolResponse[SubagentResult] {
			return Subagent(ctx, sessions, workingRoot, generateID, resolveProvider, request)
		})
	}),
	stableToolID(ToolStartAsync): handwrittenToolBinding(ToolStartAsync, func(ctx context.Context, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, sessions factorysessionexecution.Service, workingRoot string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode start async input", func(request factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionExecutionResponse] {
			return startAsyncCanonical(ctx, sessions, prepare, workingRoot, request)
		})
	}),
	stableToolID(ToolGetSession): handwrittenToolBinding(ToolGetSession, func(ctx context.Context, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, sessions factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get session input", func(request GetSessionInput) ToolResponse[factoryapi.FactorySessionDurableReadModel] {
			return getSessionCanonical(ctx, sessions, request)
		})
	}),
	stableToolID(ToolGetResult): handwrittenToolBinding(ToolGetResult, func(ctx context.Context, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, sessions factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode get result input", func(request GetResultInput) ToolResponse[factoryapi.FactorySessionResult] {
			return getResultCanonical(ctx, sessions, prepare, request)
		})
	}),
	stableToolID(ToolListDispatches): handwrittenToolBinding(ToolListDispatches, func(ctx context.Context, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, recordingsService RecordingsInspection, sessions factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list dispatches input", func(request ListDispatchesInput) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
			return listDispatches(ctx, sessions, recordingsService, request)
		})
	}),
	stableToolID(ToolListArtifacts): handwrittenToolBinding(ToolListArtifacts, func(ctx context.Context, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, recordingsService RecordingsInspection, _ factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode list artifacts input", func(request ListArtifactsInput) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
			return ListArtifacts(ctx, recordingsService, request)
		})
	}),
	stableToolID(ToolReadEvents): handwrittenToolBinding(ToolReadEvents, func(ctx context.Context, _ RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, recordingsService RecordingsInspection, sessions factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode read events input", func(request ReadEventsInput) ToolResponse[ReadEventsResult] {
			return readEventsCanonical(ctx, sessions, recordingsService, request)
		})
	}),
	stableToolID(ToolControl): handwrittenToolBinding(ToolControl, func(ctx context.Context, prepare RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, _ RecordingsInspection, sessions factorysessionexecution.Service, _ string, _ factorysessionexecution.SessionIDGenerator, _ ProviderIdentityResolver, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode control input", func(request ControlInput) ToolResponse[factoryapi.FactorySessionLifecycleControlResponse] {
			return controlCanonical(ctx, sessions, prepare, request)
		})
	}),
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
	prepare RequestPreparation,
	workflows factoryruntime.WorkflowPreviewOperation,
	name string,
	input json.RawMessage,
	recordingsService RecordingsInspection,
	sessions factorysessionexecution.Service,
	workingRoot string,
	generateID factorysessionexecution.SessionIDGenerator,
	resolveProvider ProviderIdentityResolver,
) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("call MCP tool: %w", errMissingRequestContext)
	}
	bindingIdentity, ok := ResolveToolHandlerBinding(name)
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
	binding := canonicalToolHandlersByID[bindingIdentity.ToolID]
	return binding.handler(ctx, prepare, workflows, recordingsService, sessions, workingRoot, generateID, resolveProvider, input)
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

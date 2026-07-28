package operatorsettingsmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Operator Settings dependencies once;
// protocol tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// RootDependencies are the accepted Operator Settings root roles consumed by
// the MCP adapter. Operator Settings is the singular Service root; transports
// inject an implementation or test fake rather than importing Operator Settings
// internals or constructing canonical state.
type RootDependencies struct {
	Settings operatorsettings.Service
}

// Bind constructs the canonical ToolOperation from explicit Operator Settings
// root dependencies. Adapter tests replace Settings with a root-shaped fake
// without constructing real document, resolution, or service-local Wire graphs.
func Bind(deps RootDependencies) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, deps.Settings, name, input)
	}
}

// BindToolOperation binds the canonical tool registry to an explicit Operator
// Settings Service root without constructing an alternate MCP client.
func BindToolOperation(service operatorsettings.Service) ToolOperation {
	return Bind(RootDependencies{Settings: service})
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
	operatorsettings.Service,
	json.RawMessage,
) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolLoadDocument:        handleLoadDocument,
	ToolApplyDocumentUpdate: handleApplyDocumentUpdate,
	ToolResolveEffective:    handleResolveEffective,
}

func handleLoadDocument(
	ctx context.Context,
	service operatorsettings.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode load document input", func(request LoadDocumentInput) ToolResponse[operatorsettings.LoadDocumentResult] {
		return LoadDocument(ctx, service, request)
	})
}

func handleApplyDocumentUpdate(
	ctx context.Context,
	service operatorsettings.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode apply document update input", func(request ApplyDocumentUpdateInput) ToolResponse[operatorsettings.ApplyDocumentUpdateResult] {
		return ApplyDocumentUpdate(ctx, service, request)
	})
}

func handleResolveEffective(
	ctx context.Context,
	service operatorsettings.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode resolve effective input", func(request ResolveEffectiveInput) ToolResponse[operatorsettings.ResolveEffectiveResult] {
		return ResolveEffective(ctx, service, request)
	})
}

// CallTool invokes one Operator Settings tool against an explicitly supplied
// Service root. Protocol servers receive the bound ToolOperation rather than
// choosing between construction paths.
func CallTool(
	ctx context.Context,
	service operatorsettings.Service,
	name string,
	input json.RawMessage,
) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("call MCP tool: %w", errMissingRequestContext)
	}
	handler, ok := canonicalToolHandlers[name]
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
	return handler(ctx, service, input)
}

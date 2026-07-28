package providersmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Providers dependencies once; protocol
// tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// Bind constructs the canonical ToolOperation from explicit Providers root
// dependencies. Adapter tests replace Providers with a root-shaped fake without
// constructing real catalog, execution, or service-local Wire graphs.
func Bind(deps RootDependencies) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, deps.Providers, name, input)
	}
}

// BindToolOperation binds the canonical tool registry to an explicit Providers
// Service root without constructing an alternate MCP client.
func BindToolOperation(service providers.Service) ToolOperation {
	return Bind(RootDependencies{Providers: service})
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
	providers.Service,
	json.RawMessage,
) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolListProviders: handleListProviders,
	ToolGetProvider:   handleGetProvider,
	ToolExecute:       handleExecute,
}

func handleListProviders(
	ctx context.Context,
	service providers.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode list providers input", func(request ListProvidersInput) ToolResponse[providers.ListProvidersResult] {
		return ListProviders(ctx, service, request)
	})
}

func handleGetProvider(
	ctx context.Context,
	service providers.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode get provider input", func(request GetProviderInput) ToolResponse[providers.GetProviderResult] {
		return GetProvider(ctx, service, request)
	})
}

func handleExecute(
	ctx context.Context,
	service providers.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode execute input", func(request ExecuteInput) ToolResponse[providers.ExecuteResult] {
		return Execute(ctx, service, request)
	})
}

// CallTool invokes one Providers tool against an explicitly supplied Service root.
// Protocol servers receive the bound ToolOperation rather than choosing between
// construction paths.
func CallTool(
	ctx context.Context,
	service providers.Service,
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

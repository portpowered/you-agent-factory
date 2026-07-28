package modelmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Models dependencies once; protocol
// tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// Bind constructs the canonical ToolOperation from an injected Models root.
// Adapter tests replace Models with a root-shaped fake without constructing
// real catalog, assets, host, lease, inference graphs, or service-local Wire.
func Bind(binding RootBinding) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, binding.Models, name, input)
	}
}

// BindToolOperation binds the canonical tool registry to an explicit Models
// Service root without constructing an alternate MCP client.
func BindToolOperation(service models.Service) ToolOperation {
	return Bind(RootBinding{Models: service})
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
	models.Service,
	json.RawMessage,
) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolListCatalog:      handleListCatalog,
	ToolPrepareAssets:    handlePrepareAssets,
	ToolAcquireLease:     handleAcquireLease,
	ToolInvokeWithLease:  handleInvokeWithLease,
}

func handleListCatalog(
	ctx context.Context,
	service models.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode list catalog input", func(request ListCatalogInput) ToolResponse[models.ListModelsResult] {
		return ListCatalog(ctx, service, request)
	})
}

func handlePrepareAssets(
	ctx context.Context,
	service models.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode prepare assets input", func(request PrepareAssetsInput) ToolResponse[models.PrepareModelAssetsResult] {
		return PrepareAssets(ctx, service, request)
	})
}

func handleAcquireLease(
	ctx context.Context,
	service models.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode acquire lease input", func(request AcquireLeaseInput) ToolResponse[AcquireLeaseResult] {
		return AcquireLease(ctx, service, request)
	})
}

func handleInvokeWithLease(
	ctx context.Context,
	service models.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode invoke with lease input", func(request InvokeWithLeaseInput) ToolResponse[InvokeWithLeaseResult] {
		return InvokeWithLease(ctx, service, request)
	})
}

// CallTool invokes one Models tool against an explicitly supplied Service root.
// Protocol servers receive the bound ToolOperation rather than choosing
// between construction paths.
func CallTool(
	ctx context.Context,
	service models.Service,
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

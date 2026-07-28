package workmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Work dependencies once; protocol tests
// replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// Bind constructs the canonical ToolOperation from explicit Work root
// dependencies. Adapter tests replace Work with a root-shaped fake without
// constructing real admission, content-staging, content-materialization,
// state-access graphs, or service-local Wire.
func Bind(deps RootDependencies) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, deps.Work, name, input)
	}
}

// BindToolOperation binds the canonical tool registry to an explicit Work
// Service root without constructing an alternate MCP client.
func BindToolOperation(service work.Service) ToolOperation {
	return Bind(RootDependencies{Work: service})
}

type canonicalToolHandler func(
	context.Context,
	work.Service,
	json.RawMessage,
) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolSubmit: handleSubmit,
	ToolList:   handleList,
	ToolGet:    handleGet,
}

func handleSubmit(
	ctx context.Context,
	service work.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode submit work input", func(request SubmitInput) ToolResponse[work.WorkRequestSubmitResult] {
		return Submit(ctx, service, request)
	})
}

func handleList(
	ctx context.Context,
	service work.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode list work input", func(request ListInput) ToolResponse[work.ListResult] {
		return List(ctx, service, request)
	})
}

func handleGet(
	ctx context.Context,
	service work.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode get work input", func(request GetInput) ToolResponse[work.ReadModel] {
		return Get(ctx, service, request)
	})
}

// CallTool invokes one Work tool against an explicitly supplied Service root.
// Protocol servers receive the bound ToolOperation rather than choosing
// between construction paths.
func CallTool(
	ctx context.Context,
	service work.Service,
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

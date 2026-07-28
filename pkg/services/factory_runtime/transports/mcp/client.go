package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Factory Runtime dependencies once;
// protocol tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// RootDependencies are the accepted Factory Runtime root roles consumed by the
// MCP adapter. Transports inject factory.Service or a root-shaped fake rather
// than importing Runtime internals or constructing canonical state.
type RootDependencies struct {
	Runtime factoryruntime.Service
}

// Bind constructs the canonical ToolOperation from explicit Runtime root
// dependencies. Adapter tests replace Runtime with a root-shaped fake without
// constructing live Factory Runtime lifecycle or durable engine state.
func Bind(deps RootDependencies) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, deps.Runtime, name, input)
	}
}

// BindRuntime binds the canonical tool registry to an explicit Factory Runtime
// root without constructing an alternate MCP client.
func BindRuntime(runtime factoryruntime.Service) ToolOperation {
	return Bind(RootDependencies{Runtime: runtime})
}

type canonicalToolHandler func(
	context.Context,
	factoryruntime.Service,
	json.RawMessage,
) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolControlPause: func(ctx context.Context, runtime factoryruntime.Service, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode control pause input", func(struct{}) ToolResponse[factoryruntime.PauseResult] {
			return ControlPause(ctx, runtime)
		})
	},
	ToolObserve: func(ctx context.Context, runtime factoryruntime.Service, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode observe input", func(request ObserveInput) ToolResponse[factoryruntime.ObserveResult] {
			return Observe(ctx, runtime, request)
		})
	},
	ToolPlanDispatch: func(ctx context.Context, runtime factoryruntime.Service, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode plan dispatch input", func(request PlanDispatchInput) ToolResponse[factoryruntime.PlanDispatchResult] {
			return PlanDispatch(ctx, runtime, request)
		})
	},
	ToolAcceptDispatchResult: func(ctx context.Context, runtime factoryruntime.Service, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode accept dispatch result input", func(request AcceptDispatchResultInput) ToolResponse[factoryruntime.AcceptDispatchResultResult] {
			return AcceptDispatchResult(ctx, runtime, request)
		})
	},
}

func callToolJSON[Input any, Output any](
	input json.RawMessage,
	decodeErr string,
	handler func(Input) ToolResponse[Output],
) (json.RawMessage, error) {
	var request Input
	if len(input) > 0 {
		if err := json.Unmarshal(input, &request); err != nil {
			envelope := decodeInputErrorEnvelope(decodeErr, err)
			return json.Marshal(ToolResponse[Output]{Error: &envelope})
		}
	}
	return json.Marshal(handler(request))
}

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Factory Runtime tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

// CallTool invokes one Factory Runtime tool against an explicitly supplied
// Runtime root. Protocol servers receive the bound ToolOperation rather than
// choosing between construction paths.
func CallTool(
	ctx context.Context,
	runtime factoryruntime.Service,
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
	return handler(ctx, runtime, input)
}

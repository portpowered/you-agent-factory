// Package factoryvisualization exposes MCP tool execution for Factory
// Visualization operations backed by the accepted Visualization root contract.
package factoryvisualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Factory Visualization root once;
// protocol tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// RootDependencies are the accepted Factory Visualization root roles consumed
// by the MCP adapter. Transports inject an implementation or test fake rather
// than importing Visualization internals or constructing canonical state.
type RootDependencies struct {
	Root factoryvisualization.Root
}

// Bind constructs the canonical ToolOperation from an injected Visualization
// root. Adapter tests replace Root with a root-shaped fake without constructing
// real activation_lifecycle, live_view_projection, or presentation owners.
func Bind(deps RootDependencies) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, deps.Root, name, input)
	}
}

type canonicalToolHandler func(
	context.Context,
	factoryvisualization.Root,
	json.RawMessage,
) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolActivate: func(ctx context.Context, root factoryvisualization.Root, input json.RawMessage) (json.RawMessage, error) {
		return callToolJSON(input, "decode activate input", func(request ActivateInput) ToolResponse[factoryvisualization.ActivateResult] {
			return Activate(ctx, root, request)
		})
	},
}

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Factory Visualization tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

// CallTool invokes one Factory Visualization tool against an explicitly
// supplied root. Protocol servers receive the bound ToolOperation rather than
// choosing between construction paths.
func CallTool(
	ctx context.Context,
	root factoryvisualization.Root,
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
	return handler(ctx, root, input)
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

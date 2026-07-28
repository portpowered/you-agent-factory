package mcp

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ObserveInput is the MCP request shape for you.factory_runtime.observe.
type ObserveInput struct {
	Scope *string `json:"scope,omitempty"`
}

// Observe returns one live Factory Runtime observation through the
// you.factory_runtime.observe MCP tool.
func Observe(
	ctx context.Context,
	runtime factoryruntime.Service,
	input ObserveInput,
) ToolResponse[factoryruntime.ObserveResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryruntime.ObserveResult]{Error: &envelope}
	}
	if runtime == nil {
		envelope := unavailableRuntimeErrorEnvelope()
		return ToolResponse[factoryruntime.ObserveResult]{Error: &envelope}
	}

	request := factoryruntime.ObserveRequest{}
	if input.Scope != nil {
		request.Scope = factoryruntime.ObservationScope(*input.Scope)
	}

	result, err := runtime.Observe(ctx, request)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryruntime.ObserveResult]{Error: &envelope}
	}
	return ToolResponse[factoryruntime.ObserveResult]{Result: &result}
}

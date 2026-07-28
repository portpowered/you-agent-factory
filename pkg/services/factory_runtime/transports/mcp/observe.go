package mcp

import (
	"context"
	"fmt"

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
	if response, done := requestContextErrorResponse[factoryruntime.ObserveResult](ctx); done {
		return response
	}
	if runtime == nil {
		envelope := unavailableRuntimeErrorEnvelope()
		return ToolResponse[factoryruntime.ObserveResult]{Error: &envelope}
	}

	request, err := toObserveRequest(input)
	if err != nil {
		envelope := validationErrorEnvelope(err)
		return ToolResponse[factoryruntime.ObserveResult]{Error: &envelope}
	}

	result, err := runtime.Observe(ctx, request)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryruntime.ObserveResult]{Error: &envelope}
	}
	return ToolResponse[factoryruntime.ObserveResult]{Result: &result}
}

func toObserveRequest(input ObserveInput) (factoryruntime.ObserveRequest, error) {
	request := factoryruntime.ObserveRequest{}
	if input.Scope == nil {
		return request, nil
	}
	scope := factoryruntime.ObservationScope(*input.Scope)
	if !validObservationScope(scope) {
		return factoryruntime.ObserveRequest{}, fmt.Errorf(
			`unsupported Factory Runtime observation scope %q`,
			*input.Scope,
		)
	}
	request.Scope = scope
	return request, nil
}

func validObservationScope(scope factoryruntime.ObservationScope) bool {
	switch scope {
	case "", factoryruntime.ObservationScopeFull, factoryruntime.ObservationScopeStatus,
		factoryruntime.ObservationScopeProgress, factoryruntime.ObservationScopeDispatches,
		factoryruntime.ObservationScopeResults, factoryruntime.ObservationScopeResources,
		factoryruntime.ObservationScopeHealth:
		return true
	default:
		return false
	}
}

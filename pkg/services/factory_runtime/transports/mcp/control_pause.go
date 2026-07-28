package mcp

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ControlPause applies the Runtime root pause control through the
// you.factory_runtime.control_pause MCP tool.
func ControlPause(ctx context.Context, runtime factoryruntime.Service) ToolResponse[factoryruntime.PauseResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryruntime.PauseResult]{Error: &envelope}
	}
	if runtime == nil {
		envelope := unavailableRuntimeErrorEnvelope()
		return ToolResponse[factoryruntime.PauseResult]{Error: &envelope}
	}

	result, err := runtime.ControlPause(ctx, factoryruntime.PauseRequest{})
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryruntime.PauseResult]{Error: &envelope}
	}
	return ToolResponse[factoryruntime.PauseResult]{Result: &result}
}

package factoryvisualization

import (
	"context"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// Tool names use Factory Visualization vocabulary.
const (
	ToolActivate             = "you.factory_visualization.activate"
	ToolJoin                 = "you.factory_visualization.join"
	ToolStopDrain            = "you.factory_visualization.stop_drain"
	ToolObserve              = "you.factory_visualization.observe"
	ToolOpenPresentation     = "you.factory_visualization.open_presentation"
	ToolPresentProgress      = "you.factory_visualization.present_progress"
	ToolFinalizePresentation = "you.factory_visualization.finalize_presentation"
	ToolClosePresentation    = "you.factory_visualization.close_presentation"
)

// ToolErrorEnvelope is the stable MCP failure shape for Visualization tools.
type ToolErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

// ToolResponse wraps one tool outcome with either a typed result or a stable error.
type ToolResponse[T any] struct {
	Result *T                 `json:"result,omitempty"`
	Error  *ToolErrorEnvelope `json:"error,omitempty"`
}

// ActivateInput is the MCP request shape for you.factory_visualization.activate.
type ActivateInput struct {
	Mode string `json:"mode"`
}

// Activate runs the Visualization root Activate contract for the activate MCP tool.
func Activate(
	ctx context.Context,
	root factoryvisualization.Root,
	input ActivateInput,
) ToolResponse[factoryvisualization.ActivateResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.ActivateResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	if input.Mode == "" {
		envelope := requestValidationErrorEnvelope("activate Factory visualization: required request parameters are missing")
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	result, err := root.Activate(ctx, factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateMode(input.Mode),
	})
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.ActivateResult]{Result: &result}
}

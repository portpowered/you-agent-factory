package factoryvisualization

import (
	"encoding/json"
)

// Stable error envelope fields shared by every Visualization MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.details",
}

// ToolDefinition is one discoverable MCP tool with typed schemas and documented
// stable response fields for success and error envelopes.
type ToolDefinition struct {
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	InputSchema         map[string]any `json:"inputSchema"`
	OutputSchema        map[string]any `json:"outputSchema"`
	SuccessStableFields []string       `json:"successStableFields"`
	ErrorStableFields   []string       `json:"errorStableFields"`
}

// MarshalJSON encodes one tool definition for MCP hosts and mock clients.
func (t ToolDefinition) MarshalJSON() ([]byte, error) {
	type alias ToolDefinition
	return json.Marshal(alias(t))
}

// DiscoverTools returns the canonical Factory Visualization MCP tool catalog
// in stable discovery order.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		activateTool(),
		joinTool(),
		stopDrainTool(),
		observeTool(),
		openPresentationTool(),
		presentProgressTool(),
		finalizePresentationTool(),
		closePresentationTool(),
	}
}

// ToolNames returns stable canonical tool names in discovery order.
func ToolNames() []string {
	tools := DiscoverTools()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

// ToolByName returns one canonical tool definition by stable name.
func ToolByName(name string) (ToolDefinition, bool) {
	for _, tool := range DiscoverTools() {
		if tool.Name == name {
			return tool, true
		}
	}
	return ToolDefinition{}, false
}

func activateTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolActivate,
		Description: "Activate Factory Visualization through retained-then-live projection and leave the inert constructed state.",
		InputSchema: activateInputSchema(),
		OutputSchema: toolResponseSchema(activateResultSchema()),
		SuccessStableFields: []string{
			"result.State",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func joinTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolJoin,
		Description: "Join the Visualization live subscription and wait for it to exit after Activate.",
		InputSchema: joinInputSchema(),
		OutputSchema: toolResponseSchema(joinResultSchema()),
		SuccessStableFields: []string{
			"result.State",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func stopDrainTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolStopDrain,
		Description: "Stop the Visualization live subscription and drain one final projected view through the Visualization-owned drain path.",
		InputSchema: stopDrainInputSchema(),
		OutputSchema: toolResponseSchema(stopDrainResultSchema()),
		SuccessStableFields: []string{
			"result.State",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func observeTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolObserve,
		Description: "Observe one detached retained-then-live Factory view projection through Visualization-owned plain contracts.",
		InputSchema: observeInputSchema(),
		OutputSchema: toolResponseSchema(observeResultSchema()),
		SuccessStableFields: []string{
			"result.View.TickCount",
			"result.View.RetainedEventCount",
			"result.View.ObservedAt",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func openPresentationTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolOpenPresentation,
		Description: "Open one Visualization-owned presentation session using best-effort or lossless drain policy.",
		InputSchema: openPresentationInputSchema(),
		OutputSchema: toolResponseSchema(openPresentationResultSchema()),
		SuccessStableFields: []string{
			"result.SessionID",
			"result.Mode",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func presentProgressTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolPresentProgress,
		Description: "Enqueue ordered progress records onto an opened Visualization presentation session.",
		InputSchema: presentProgressInputSchema(),
		OutputSchema: toolResponseSchema(presentProgressResultSchema()),
		SuccessStableFields: []string{
			"result.AcceptedCount",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func finalizePresentationTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolFinalizePresentation,
		Description: "Finalize one Visualization presentation session after draining accepted progress and committing a terminal write.",
		InputSchema: finalizePresentationInputSchema(),
		OutputSchema: toolResponseSchema(finalizePresentationResultSchema()),
		SuccessStableFields: []string{
			"result.Finalized",
			"result.ProgressSeen",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func closePresentationTool() ToolDefinition {
	return ToolDefinition{
		Name:        ToolClosePresentation,
		Description: "Close and drain one Visualization presentation session without committing a terminal write.",
		InputSchema: closePresentationInputSchema(),
		OutputSchema: toolResponseSchema(closePresentationResultSchema()),
		SuccessStableFields: []string{
			"result.DroppedCount",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

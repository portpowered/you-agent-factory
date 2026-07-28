package recordingmcp

// DiscoverTools returns the canonical Recordings MCP tool catalog in stable
// discovery order. Schemas mirror accepted Recordings root request contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		queryStatusTool(),
		appendEventTool(),
		loadReplayTool(),
		readPortableArtifactTool(),
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

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Recordings tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

func queryStatusTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolQueryStatus,
		Description: "Query one recording's detached lifecycle status facts through the accepted " +
			"Recordings root without constructing Recordings internals.",
		InputSchema:  queryStatusInputSchema(),
		OutputSchema: toolResponseSchema(recordingStatusResultSchema()),
		SuccessStableFields: []string{
			"result.Status.RecordingID",
			"result.Status.State",
			"result.Status.AcceptedEvents",
			"result.Status.LastEvent",
			"result.Status.FinalizedAt",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func appendEventTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolAppendEvent,
		Description: "Append one canonical Factory Event through the accepted Recordings root " +
			"append/subscribe slice without constructing Recordings internals.",
		InputSchema:  appendEventInputSchema(),
		OutputSchema: toolResponseSchema(appendRecordedEventResultSchema()),
		SuccessStableFields: []string{
			"result.Event.ID",
			"result.Event.Sequence",
			"result.Event.Kind",
			"result.Event.Payload",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func loadReplayTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolLoadReplay,
		Description: "Load finalized canonical replay facts for one recording through the accepted " +
			"Recordings root replay slice without HTTP or CLI transport paths.",
		InputSchema:  loadReplayInputSchema(),
		OutputSchema: toolResponseSchema(loadReplayRecordingResultSchema()),
		SuccessStableFields: []string{
			"result.Recording.RecordingID",
			"result.Recording.Scope",
			"result.Recording.Events",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func readPortableArtifactTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolReadPortableArtifact,
		Description: "Read and validate one published portable recording artifact through the accepted " +
			"Recordings root artifact-export slice without HTTP or CLI transport paths.",
		InputSchema:  readPortableArtifactInputSchema(),
		OutputSchema: toolResponseSchema(readPortableArtifactResultSchema()),
		SuccessStableFields: []string{
			"result.Artifact.schemaVersion",
			"result.Artifact.summary.recordingId",
			"result.Artifact.summary.state",
			"result.Artifact.events",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

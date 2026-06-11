package factorysession

// DiscoverTools returns the canonical dynamic workflow Factory Session MCP tool
// catalog in stable discovery order. Schemas mirror durable REST and Factory
// preview contracts; deprecated /workflow-previews is not a primary surface.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		listSessionsTool(),
		validateSourceTool(),
		startSyncTool(),
		startAsyncTool(),
		getSessionTool(),
		getResultTool(),
		listDispatchesTool(),
		listArtifactsTool(),
		controlTool(),
		readEventsTool(),
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

func listSessionsTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListSessions,
		Description: "List Factory Sessions for one scope (live workspace, persisted durable execution, or all). " +
			"Uses GET /factory-sessions durable listing vocabulary.",
		InputSchema: objectSchema(map[string]any{
			"scope": enumStringProperty(
				"Session list scope. Defaults to live when omitted.",
				"live", "persisted", "all",
			),
		}),
		OutputSchema: toolResponseSchema(listFactorySessionsResponseSchema()),
		SuccessStableFields: []string{
			"result.scope",
			"result.sessions",
			"result.durableSessions",
			"result.durableSessions[].sessionId",
			"result.durableSessions[].status",
			"result.durableSessions[].orchestratorKind",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func validateSourceTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolValidateSource,
		Description: "Validate JavaScript orchestrator factory source through the canonical Factory preview contract " +
			"(POST /factories/preview) before starting a Factory Session.",
		InputSchema:  factoryPreviewRequestSchema(),
		OutputSchema: toolResponseSchema(factoryPreviewResultSchema()),
		SuccessStableFields: []string{
			"result.valid",
			"result.sourceResolution.sourceHash",
			"result.sourceResolution.sourceRef",
			"result.policyPreview.effectivePolicyHash",
			"result.sourceValidationIssues",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func startSyncTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolStartSync,
		Description: "Start one mock-backed sync Factory Session and wait for terminal completion or timeout. " +
			"Maps to POST /factory-sessions/sync.",
		InputSchema:  factorySessionExecutionRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionSyncExecutionResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.status",
			"result.syncOutcome",
			"result.sourceHash",
			"result.effectivePolicyHash",
			"result.result.resultStatus",
			"result.links.results",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func startAsyncTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolStartAsync,
		Description: "Start one mock-backed async Factory Session and return an accepted or running summary for polling. " +
			"Maps to POST /factory-sessions/async.",
		InputSchema:  factorySessionExecutionRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionExecutionResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.status",
			"result.orchestratorKind",
			"result.sourceHash",
			"result.effectivePolicyHash",
			"result.links.session",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func getSessionTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolGetSession,
		Description: "Get one durable Factory Session inspection read model with lifecycle status, source identity, " +
			"phase, progress, and result summary. Maps to GET /factory-sessions/{session_id}.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(factorySessionDurableReadModelSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.status",
			"result.orchestratorKind",
			"result.resolvedSource",
			"result.phase",
			"result.progress",
			"result.resultSummary.resultStatus",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func getResultTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolGetResult,
		Description: "Retrieve one durable Factory Session result in final or partial mode with optional artifact metadata. " +
			"Maps to GET /factory-sessions/{session_id}/results.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
			"mode": enumStringProperty(
				"Result retrieval mode. Defaults to final when omitted.",
				"final", "partial",
			),
			"includeArtifacts": booleanProperty(
				"When true, include FactoryArtifact metadata refs in the response.",
			),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(factorySessionResultSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.resultStatus",
			"result.sessionStatus",
			"result.primaryResult",
			"result.artifactIds",
			"result.availability",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func listDispatchesTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListDispatches,
		Description: "List dispatch summaries for one Factory Session, including dispatch id, status, kind, phase, " +
			"and provider-session correlation metadata when available. Maps to GET /factory-sessions/{session_id}/dispatches.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(listFactorySessionDispatchesResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.dispatches",
			"result.dispatches[].dispatchId",
			"result.dispatches[].status",
			"result.dispatches[].kind",
			"result.dispatches[].phase",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func listArtifactsTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListArtifacts,
		Description: "List FactoryArtifact summaries for one Factory Session, including artifact id, kind, visibility, " +
			"size or hash metadata, and dispatch linkage when available. Maps to GET /factory-sessions/{session_id}/artifacts.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(listFactorySessionArtifactsResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.artifacts",
			"result.artifacts[].artifactId",
			"result.artifacts[].kind",
			"result.artifacts[].visibility",
			"result.artifacts[].dispatchId",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func controlTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolControl,
		Description: "Apply one durable Factory Session lifecycle control such as approve, pause, resume, cancel, " +
			"terminate, or retry-dispatch. Maps to POST /factory-sessions/{session_id}/{control}.",
		InputSchema:  factorySessionLifecycleControlRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionLifecycleControlResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.operation",
			"result.outcome",
			"result.status",
			"result.links.session",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func readEventsTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolReadEvents,
		Description: "Read ordered Factory Session event facts for reconnect and inspection without exposing " +
			"internal Petri-net terminology as the primary public vocabulary. Maps to GET /factory-sessions/{session_id}/events.",
		InputSchema:  factorySessionEventReadRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionEventReadResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.events",
			"result.events[].id",
			"result.events[].type",
			"result.events[].context.sessionId",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

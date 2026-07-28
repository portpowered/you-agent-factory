package modelmcp

// DiscoverTools returns the canonical Models MCP tool catalog in stable
// discovery order. Schemas mirror accepted Models root request contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		listCatalogTool(),
		prepareAssetsTool(),
		acquireLeaseTool(),
		invokeWithLeaseTool(),
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
// registers a handler for one canonical Models tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

func listCatalogTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListCatalog,
		Description: "List detached catalog summaries for one open runtime scope through the accepted " +
			"Models root without constructing Models internals.",
		InputSchema:  listCatalogInputSchema(),
		OutputSchema: toolResponseSchema(listCatalogResultSchema()),
		SuccessStableFields: []string{
			"result.Models",
			"result.Models[].Name",
			"result.Models[].Status",
			"result.Models[].LoadState",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func prepareAssetsTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolPrepareAssets,
		Description: "Prepare scoped model assets through the accepted Models root and distinguish " +
			"already-available assets from newly prepared assets without HTTP or CLI transport paths.",
		InputSchema:  prepareAssetsInputSchema(),
		OutputSchema: toolResponseSchema(prepareAssetsResultSchema()),
		SuccessStableFields: []string{
			"result.Asset.ModelName",
			"result.Asset.Readiness",
			"result.Outcome",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func acquireLeaseTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolAcquireLease,
		Description: "Acquire detached Models lease capability for one scoped model and holder through " +
			"the accepted Models root without constructing host or lease internals.",
		InputSchema:  acquireLeaseInputSchema(),
		OutputSchema: toolResponseSchema(acquireLeaseResultSchema()),
		SuccessStableFields: []string{
			"result.Lease.Lease",
			"result.Lease.ModelName",
			"result.Lease.Holder",
			"result.Lease.Status",
			"result.Lease.HostReadiness",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func invokeWithLeaseTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolInvokeWithLease,
		Description: "Invoke one scoped model operation under an issued Models lease through the accepted " +
			"Models root and return detached content, artifact, identity, and lease-disposition facts.",
		InputSchema:  invokeWithLeaseInputSchema(),
		OutputSchema: toolResponseSchema(invokeWithLeaseResultSchema()),
		SuccessStableFields: []string{
			"result.Invocation",
			"result.ModelName",
			"result.Operation",
			"result.Status",
			"result.Content",
			"result.Artifacts",
			"result.LeaseDisposition",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

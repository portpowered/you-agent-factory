package operatorsettingsmcp

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func booleanProperty(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
	}
}

func toolResponseSchema(resultSchema map[string]any) map[string]any {
	return objectSchema(map[string]any{
		"result": resultSchema,
		"error":  toolErrorEnvelopeSchema(),
	})
}

func toolErrorEnvelopeSchema() map[string]any {
	return objectSchema(map[string]any{
		"code":      stringProperty("Stable machine-readable failure code."),
		"message":   stringProperty("Human-readable failure summary for MCP hosts."),
		"retryable": map[string]any{"type": "boolean", "description": "Whether the caller should retry the same request later."},
		"details": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional structured diagnostics such as validation issues or conflict facts.",
		},
	}, "code", "message", "retryable")
}

func documentDefaultsInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"workerModelProvider": stringProperty("Document baseline worker model provider."),
		"workerModel":         stringProperty("Document baseline worker model."),
	})
}

func documentProviderModelInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"provider": stringProperty("Optional provider replacement; omitted preserves the current value."),
		"model":    stringProperty("Optional model replacement; omitted preserves the current value."),
	})
}

func documentWorkerPresetInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"id":              stringProperty("Worker preset identifier."),
		"modelProvider":   stringProperty("Preset model provider."),
		"model":           stringProperty("Preset model."),
		"reasoningEffort": stringProperty("Preset reasoning effort."),
	}, "id")
}

func effectiveOverrideInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"workerModelProvider": stringProperty("Override worker model provider."),
		"workerModel":         stringProperty("Override worker model."),
		"workerPresetId":      stringProperty("Override worker preset identifier."),
	})
}

func documentSchema() map[string]any {
	return objectSchema(map[string]any{
		"BackendScopeID": stringProperty("Detached backend scope identifier."),
		"Defaults":       documentDefaultsInputSchema(),
		"Runtime": map[string]any{
			"type": "object",
		},
		"WorkerPresets": map[string]any{
			"type":  "array",
			"items": documentWorkerPresetInputSchema(),
		},
	})
}

func loadDocumentInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"path":            stringProperty("Operator document path."),
		"requireExisting": booleanProperty("When true, a missing document fails instead of returning an empty document."),
	}, "path")
}

func loadDocumentResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Document": documentSchema(),
		"Path":     stringProperty("Resolved operator document path."),
		"Found":    booleanProperty("Whether a stored document was found."),
	})
}

func applyDocumentUpdateInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"path":                 stringProperty("Operator document path."),
		"expectedBackendScope": stringProperty("Expected backend scope identifier for optimistic concurrency."),
		"providerModel":        documentProviderModelInputSchema(),
	}, "path", "providerModel")
}

func applyDocumentUpdateResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Document":  documentSchema(),
		"Path":      stringProperty("Resolved operator document path."),
		"Persisted": booleanProperty("Whether the updated document was persisted."),
	})
}

func resolveEffectiveInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"documentBaseline":         documentDefaultsInputSchema(),
		"backendScopeId":           stringProperty("Detached backend scope identifier."),
		"workerPresets": map[string]any{
			"type":  "array",
			"items": documentWorkerPresetInputSchema(),
		},
		"expectedDocumentBaseline": documentDefaultsInputSchema(),
		"environmentOverrides":     effectiveOverrideInputSchema(),
		"invocationOverrides":      effectiveOverrideInputSchema(),
		"configPath":               stringProperty("Operator document path used for diagnostics."),
	}, "documentBaseline", "configPath")
}

func effectiveSelectionSchema() map[string]any {
	return objectSchema(map[string]any{
		"BackendScopeID":            stringProperty("Effective backend scope identifier."),
		"WorkerPresets":             map[string]any{"type": "array", "items": documentWorkerPresetInputSchema()},
		"WorkerModelProvider":       stringProperty("Effective worker model provider."),
		"WorkerModel":               stringProperty("Effective worker model."),
		"WorkerModelProviderSource": stringProperty("Precedence layer that supplied the effective provider."),
		"WorkerModelSource":         stringProperty("Precedence layer that supplied the effective model."),
		"ConfigPath":                stringProperty("Operator document path used for diagnostics."),
	})
}

func resolveEffectiveResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Selection": effectiveSelectionSchema(),
	})
}

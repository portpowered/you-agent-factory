package factorydefinition

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

func enumStringProperty(description string, values ...string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
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
			"description":          "Optional structured diagnostics such as validation issues or availability hints.",
		},
	}, "code", "message", "retryable")
}

func factoryDefinitionInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"name":        stringProperty("Factory name for the submitted definition."),
		"description": stringProperty("Optional customer-facing explanation of this Factory."),
		"id":          stringProperty("Factory identifier used as the factory-level template context fallback."),
		"version": map[string]any{
			"type":        "object",
			"description": "Server-managed current-factory version metadata echoed on replacement saves.",
		},
		"workTypes": map[string]any{
			"type":        "array",
			"description": "Customer-authored work item categories and lifecycle states.",
			"items":       map[string]any{"type": "object"},
		},
		"workers": map[string]any{
			"type":        "array",
			"description": "Workers that execute work at workstations.",
			"items":       map[string]any{"type": "object"},
		},
		"workstations": map[string]any{
			"type":        "array",
			"description": "Workstations that bind workers to work-type transitions.",
			"items":       map[string]any{"type": "object"},
		},
		"resources": map[string]any{
			"type":        "array",
			"description": "Shared capacity pools consumed while work executes.",
			"items":       map[string]any{"type": "object"},
		},
		"guards": map[string]any{
			"type":        "array",
			"description": "Root-level guards that apply across the factory.",
			"items":       map[string]any{"type": "object"},
		},
	}, "name")
}

func factoryValidationResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"targets": map[string]any{
			"type":        "array",
			"description": "Canonical validation targets for the submitted factory definition.",
			"items": objectSchema(map[string]any{
				"code":     stringProperty("Validation issue code."),
				"message":  stringProperty("Validation issue message."),
				"severity": stringProperty("Validation severity."),
				"subject": map[string]any{
					"type":        "object",
					"description": "Factory-domain component referenced by one validation target.",
				},
			}),
		},
	}, "targets")
}

func factoryPayloadSchema() map[string]any {
	return objectSchema(map[string]any{
		"name": stringProperty("Factory name for the current-factory payload."),
		"version": map[string]any{
			"type":        "object",
			"description": "Current-factory version metadata for stale-write detection.",
		},
		"workTypes": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "object"},
		},
		"workers": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "object"},
		},
		"workstations": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "object"},
		},
	}, "name")
}

func saveCurrentFactoryInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable Factory Session identifier whose current factory is being saved."),
		"mode": enumStringProperty(
			"Explicit save mode. Defaults to REPLACE_CURRENT when omitted.",
			"REPLACE_CURRENT", "UPSERT_NAMED_AND_ACTIVATE",
		),
		"factory": factoryPayloadSchema(),
	}, "sessionId", "factory")
}

func getCurrentFactoryInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable Factory Session identifier whose current factory should be read."),
	}, "sessionId")
}

func installPackagedFactoryInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"package": stringProperty("Built-in packaged Factory identity, such as @you/example."),
		"dir":     stringProperty("Destination named-factory root directory. Defaults to the caller home directory when omitted."),
		"format": enumStringProperty(
			"Portable editable format for the installed factory layout.",
			"json", "yaml", "toml",
		),
		"replace": booleanProperty("When true, replace an existing installed factory at the destination."),
	}, "package")
}

func installPackagedFactoryResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"name":       stringProperty("Installed packaged Factory name."),
		"factoryDir": stringProperty("Destination directory containing the installed factory layout."),
		"outcome":    stringProperty("Definitions-owned install outcome such as CREATED, SKIPPED, or REPLACED."),
		"format":     stringProperty("Portable editable format used for the installed factory layout."),
	}, "name", "outcome")
}

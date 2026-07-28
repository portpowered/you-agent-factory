package workmcp

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

func integerProperty(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
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

func workContentPartSchema() map[string]any {
	return objectSchema(map[string]any{
		"type":        stringProperty("Canonical Work content part kind."),
		"text":        stringProperty("Text content when type is text."),
		"url":         stringProperty("Remote content URL when type is image or binary."),
		"file":        stringProperty("Local file path reference when supplied by adapters."),
		"json":        map[string]any{"description": "Structured JSON payload for JSON content parts."},
		"slot":        stringProperty("Optional slot name for multi-part content."),
		"label":       stringProperty("Optional human-readable label."),
		"role":        stringProperty("Optional content role such as user or assistant."),
		"contentType": stringProperty("Optional MIME type for binary content."),
		"artifactId":  stringProperty("Optional artifact identifier for staged content."),
		"metadata": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional structured metadata for the content part.",
		},
	}, "type")
}

func workItemSchema() map[string]any {
	return objectSchema(map[string]any{
		"name":                     stringProperty("Customer-authored work item name within the batch."),
		"workId":                   stringProperty("Optional stable Work identifier."),
		"requestId":                stringProperty("Optional per-item request identifier."),
		"workTypeName":             stringProperty("Configured work type name from the Factory definition."),
		"state":                    stringProperty("Optional authored state name for the submitted work item."),
		"chainingTraceDepth":       integerProperty("Optional chaining trace depth."),
		"currentChainingTraceId":   stringProperty("Optional current chaining trace identifier."),
		"previousChainingTraceIds": map[string]any{
			"type":        "array",
			"description": "Optional prior chaining trace identifiers.",
			"items":       stringProperty("Chaining trace identifier."),
		},
		"traceId": stringProperty("Optional trace identifier for the submitted work item."),
		"content": map[string]any{
			"type":        "array",
			"description": "Detached Work content parts attached to the item.",
			"items":       workContentPartSchema(),
		},
		"payload": map[string]any{"description": "Optional opaque JSON payload for the work item."},
		"tags": map[string]any{
			"type":                 "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "Optional string tags attached to the work item.",
		},
	}, "name")
}

func workRelationSchema() map[string]any {
	return objectSchema(map[string]any{
		"type":           enumStringProperty("Relationship kind between work items.", "DEPENDS_ON", "PARENT_CHILD"),
		"sourceWorkName": stringProperty("Source work item name in the relationship."),
		"targetWorkId":   stringProperty("Optional target work identifier."),
		"targetWorkName": stringProperty("Target work item name in the relationship."),
		"requiredState":  stringProperty("Optional required target state for dependency edges."),
	}, "type", "sourceWorkName", "targetWorkName")
}

func workRequestSchema() map[string]any {
	return objectSchema(map[string]any{
		"requestId":              stringProperty("Stable client-provided request identifier for idempotent admission."),
		"currentChainingTraceId": stringProperty("Optional default chaining trace applied to batch items."),
		"type": enumStringProperty(
			"Canonical Work Request contract type.",
			"FACTORY_REQUEST_BATCH",
		),
		"works": map[string]any{
			"type":        "array",
			"description": "Batch of work items submitted together.",
			"items":       workItemSchema(),
		},
		"relations": map[string]any{
			"type":        "array",
			"description": "Relationships between submitted work items.",
			"items":       workRelationSchema(),
		},
	}, "requestId", "type")
}

func listOptionsSchema() map[string]any {
	return objectSchema(map[string]any{
		"stateName":    stringProperty("Filter by authored state name."),
		"stateType":    stringProperty("Filter by state category such as INITIAL or TERMINAL."),
		"name":         stringProperty("Filter by work item name."),
		"workTypeName": stringProperty("Filter by configured work type name."),
		"traceId":      stringProperty("Filter by trace identifier."),
		"sortBy":       stringProperty("Optional sort field such as state.type."),
		"maxResults":   integerProperty("Maximum number of results to return."),
		"nextToken":    stringProperty("Opaque pagination token from a prior list response."),
	})
}

func readModelStateSchema() map[string]any {
	return objectSchema(map[string]any{
		"Name": stringProperty("Authored state name."),
		"Type": stringProperty("State category such as INITIAL, PROCESSING, TERMINAL, or FAILED."),
	})
}

func readModelSchema() map[string]any {
	return objectSchema(map[string]any{
		"CursorID":                 stringProperty("Opaque cursor identifier for stable pagination."),
		"Name":                     stringProperty("Customer-authored work item name."),
		"WorkID":                   stringProperty("Stable Work identifier."),
		"WorkTypeName":             stringProperty("Configured work type name."),
		"State":                    readModelStateSchema(),
		"ChainingTraceDepth":       integerProperty("Chaining trace depth for the work item."),
		"CurrentChainingTraceID":   stringProperty("Current chaining trace identifier."),
		"PreviousChainingTraceIDs": map[string]any{
			"type":        "array",
			"description": "Prior chaining trace identifiers.",
			"items":       stringProperty("Chaining trace identifier."),
		},
		"TraceID": stringProperty("Trace identifier for the work item."),
		"Content": map[string]any{
			"type":        "array",
			"description": "Detached Work content parts.",
			"items":       workContentPartSchema(),
		},
		"Tags": map[string]any{
			"type":                 "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "String tags attached to the work item.",
		},
	})
}

func submitInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId":   stringProperty("Factory Session that should accept the Work Request."),
		"workRequest": workRequestSchema(),
	}, "sessionId", "workRequest")
}

func listInputSchema() map[string]any {
	properties := map[string]any{
		"sessionId": stringProperty("Factory Session whose Work items should be listed."),
	}
	for key, value := range listOptionsSchema()["properties"].(map[string]any) {
		properties[key] = value
	}
	return objectSchema(properties, "sessionId")
}

func getInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Factory Session that owns the Work item."),
		"workId":    stringProperty("Stable Work identifier to read."),
	}, "sessionId", "workId")
}

func moveInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Factory Session that owns the Work item."),
		"workId":    stringProperty("Stable Work identifier to move."),
		"stateName": stringProperty("Authored target state name for the operator move."),
		"requestId": stringProperty("Stable idempotency key for the operator move request."),
	}, "sessionId", "workId", "stateName", "requestId")
}

func workRequestSubmitResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"RequestID":    stringProperty("Accepted request identifier."),
		"TraceID":      stringProperty("Trace identifier assigned to the submission."),
		"WorkID":       stringProperty("Primary accepted Work identifier."),
		"Name":         stringProperty("Primary accepted work item name."),
		"WorkTypeName": stringProperty("Primary accepted work type name."),
		"Accepted":     map[string]any{"type": "boolean", "description": "Whether admission accepted the Work Request."},
		"Works": map[string]any{
			"type":        "array",
			"description": "Detached facts for each accepted work item in the batch.",
			"items": objectSchema(map[string]any{
				"Name":         stringProperty("Accepted work item name."),
				"WorkTypeName": stringProperty("Accepted work type name."),
				"WorkID":       stringProperty("Accepted Work identifier."),
			}),
		},
	})
}

func listResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Results": map[string]any{
			"type":        "array",
			"description": "Detached Work ReadModel projections.",
			"items":       readModelSchema(),
		},
		"MaxResults": integerProperty("Maximum results requested for this page."),
		"NextToken":  stringProperty("Opaque pagination token when more results remain."),
	})
}

func operatorMoveResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"WorkID":      stringProperty("Moved Work identifier."),
		"WorkTypeID":  stringProperty("Configured work type name for the moved item."),
		"FromState":   stringProperty("State name before the move."),
		"ToState":     stringProperty("State name after the move."),
		"FromPlaceID": stringProperty("Internal place identifier before the move."),
		"ToPlaceID":   stringProperty("Internal place identifier after the move."),
		"TokenID":     stringProperty("Internal token identifier for the moved work item."),
	})
}

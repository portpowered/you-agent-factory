package factorysession

func subagentResponseSchema() map[string]any {
	result := map[string]any{"type": "object", "$schema": "https://json-schema.org/draft/2020-12/schema", "oneOf": []any{subagentStartResultSchema(), subagentListResultSchema(), subagentReadResultSchema(), subagentControlResultSchema(), subagentInterruptResultSchema(), subagentSendResultSchema(), subagentInboxResultSchema(), subagentReviveResultSchema()}}
	schema := toolResponseSchema(result)
	schema["properties"].(map[string]any)["error"].(map[string]any)["properties"].(map[string]any)["workerSessionId"] = stringProperty("Selected Worker Session identity when the failure is scoped to one Worker Session.")
	return schema
}

func subagentStartResultSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"description":          "START admission result; execution remains server-owned.",
		"properties": map[string]any{
			"requestId": map[string]any{
				"type": "string",
			},
			"workerSessionId": map[string]any{
				"type": "string",
			},
			"accepted": map[string]any{
				"type": "boolean",
			},
			"state": map[string]any{
				"type": "string",
			},
			"eventTopic": map[string]any{
				"type": "string",
			},
		},
		"required": []any{"requestId", "workerSessionId", "accepted", "state", "eventTopic"},
	}
}

func subagentListResultSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"workerSessions"},
		"properties": map[string]any{
			"workerSessions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
			"nextToken": map[string]any{
				"type": []any{"string", "null"},
			},
		},
		"additionalProperties": false,
	}
}

func subagentReadResultSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"session"},
		"properties": map[string]any{
			"session": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"logs": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"transcript": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"events": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
		"additionalProperties": false,
	}
}

func subagentControlResultSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"workerSessionId", "action"},
		"properties": map[string]any{
			"workerSessionId": map[string]any{
				"type": "string",
			},
			"action": map[string]any{
				"type": "string",
			},
			"outcome": map[string]any{
				"type": "string",
			},
			"state": map[string]any{
				"type": "string",
			},
			"forced": map[string]any{
				"type": "boolean",
			},
			"dispatchId": map[string]any{
				"type": []any{"string", "null"},
			},
		},
		"additionalProperties": false,
	}
}

func subagentInterruptResultSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"accepted", "phase"},
		"properties": map[string]any{
			"accepted": map[string]any{
				"type": "boolean",
			},
			"phase": map[string]any{
				"type": "string",
			},
			"requestId": map[string]any{
				"type": "string",
			},
			"sourceWorkerSessionId": map[string]any{
				"type": "string",
			},
			"successorWorkerSessionId": map[string]any{
				"type": "string",
			},
			"source": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"successor": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
		"additionalProperties": false,
	}
}

func subagentSendResultSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"message": subagentMessageResultSchema(),
		},
		"required": []any{"message"},
	}
}

func subagentInboxResultSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"messages"},
		"properties": map[string]any{
			"messages": map[string]any{
				"type":  "array",
				"items": subagentMessageResultSchema(),
			},
			"nextToken": map[string]any{
				"type": []any{"string", "null"},
			},
		},
	}
}

func subagentReviveResultSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"requestId", "sourceWorkerSessionId", "successorWorkerSessionId", "predecessorWorkerSessionId", "accepted", "state", "eventTopic"},
		"description":          "Admission acknowledgment for a Worker Session continuation. The response exposes the source-to-successor lineage needed to inspect or stream the successor without exposing provider selection inputs.",
		"properties": map[string]any{
			"requestId": map[string]any{
				"type":        "string",
				"description": "Caller idempotency key echoed for correlation.",
			},
			"sourceWorkerSessionId": map[string]any{
				"type":        "string",
				"description": "Stable terminal Worker Session identity used as the source.",
			},
			"successorWorkerSessionId": map[string]any{
				"type":        "string",
				"description": "Stable Worker Session identity reserved for the successor.",
			},
			"predecessorWorkerSessionId": map[string]any{
				"type":        "string",
				"description": "Stable predecessor identity recorded on the successor; equal to sourceWorkerSessionId.",
			},
			"accepted": map[string]any{
				"type":        "boolean",
				"description": "Always true for a 202 response.",
			},
			"state": map[string]any{
				"type": "string",
				"enum": []any{"RESERVED", "STARTING", "RUNNING", "PAUSED", "COMPLETED", "FAILED", "CANCELED", "TERMINATED"},
			},
			"eventTopic": map[string]any{
				"type":        "string",
				"description": "Deterministic Events topic whose retained opening record is ready to read and subscribe.",
			},
		},
	}
}

func subagentMessageResultSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"messageId", "threadId", "from", "to", "body", "bodySha256", "bodyRedactionCount", "delivery", "ifEnded", "replyIfEnded", "status", "hop", "sentAt", "expiresAt"},
		"properties": map[string]any{
			"messageId": map[string]any{
				"type":        "string",
				"description": "Globally unique msg-<ULID> identity.",
			},
			"threadId": map[string]any{
				"type":        "string",
				"description": "Identity of the first message in the thread.",
			},
			"inReplyTo": map[string]any{
				"type": []any{"string", "null"},
			},
			"from": subagentMessageFromSchema(),
			"to":   subagentMessageToSchema(),
			"correlation": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"workId": map[string]any{
						"type": "string",
					},
					"factorySessionId": map[string]any{
						"type": "string",
					},
				},
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Privacy-normalized body only. Unauthorized workers receive no record.",
			},
			"bodySha256": map[string]any{
				"type":        "string",
				"pattern":     "^[a-f0-9]{64}$",
				"description": "SHA-256 of the redacted UTF-8 body.",
			},
			"bodyRedactionCount": map[string]any{
				"type":    "integer",
				"minimum": 0,
			},
			"delivery": map[string]any{
				"type": "string",
				"enum": []any{"QUEUE", "INTERRUPT"},
			},
			"ifEnded": map[string]any{
				"type": "string",
				"enum": []any{"REVIVE", "HOLD"},
			},
			"replyIfEnded": map[string]any{
				"type": "string",
				"enum": []any{"REVIVE", "HOLD"},
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []any{"QUEUED", "READ", "DELIVERED_REVIVE", "DELIVERED_INTERRUPT", "REPLIED", "EXPIRED", "REJECTED"},
				"description": "Messaging produces QUEUED, READ, DELIVERED_REVIVE, REPLIED and EXPIRED. DELIVERED_REVIVE requires provider-confirmed resumed progress and durable delivery status. DELIVERED_INTERRUPT and REJECTED remain reserved.",
			},
			"reason": map[string]any{
				"type":        []any{"string", "null"},
				"description": "Typed code only. QUEUED may report REVIVE_PENDING, REVIVE_UNSUPPORTED, REVIVE_FAILED or REVIVE_LIMIT. REVIVE_PENDING awaits provider-confirmed resumed progress. Provider-classified resume rejection reports REVIVE_UNSUPPORTED. Uncertain evidence remains queued with REVIVE_FAILED.",
			},
			"hop": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 3,
			},
			"sentAt": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
			"expiresAt": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
			"revivedWorkerSessionId": map[string]any{
				"type":        []any{"string", "null"},
				"description": "Exact successor. Published only after provider-confirmed resumed progress and durable delivery commit.",
			},
			"repliedByMessageId": map[string]any{
				"type": []any{"string", "null"},
			},
		},
	}
}

func subagentMessageFromSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"principal", "workerSessionId"},
		"properties": map[string]any{
			"principal": map[string]any{
				"type": "string",
				"enum": []any{"WORKER"},
			},
			"workerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"factorySessionId": map[string]any{
				"type": []any{"string", "null"},
			},
		},
	}
}

func subagentMessageToSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"kind", "workerSessionId"},
		"properties": map[string]any{
			"kind": map[string]any{
				"type": "string",
				"enum": []any{"WORKER_SESSION"},
			},
			"workerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"factorySessionId": map[string]any{
				"type": []any{"string", "null"},
			},
			"deliveredWorkerSessionId": map[string]any{
				"type":        []any{"string", "null"},
				"description": "Exact successor reached after provider-confirmed resumed progress and durable delivery.",
			},
		},
	}
}

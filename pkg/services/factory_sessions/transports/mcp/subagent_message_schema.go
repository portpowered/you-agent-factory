package factorysession

func subagentMessageActionSchemas() []any {
	return []any{subagentSendSchema(), subagentInboxSchema(), subagentThreadSchema(), subagentReviveSchema()}
}

func subagentSendSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{
				"type":  "string",
				"const": "MESSAGE",
			},
			"message": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": 8192,
			},
			"requestId": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": 200,
			},
			"workerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"inReplyTo": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"factorySessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"bodySecret": map[string]any{
				"type":    "boolean",
				"default": false,
			},
			"delivery": map[string]any{
				"type":    "string",
				"enum":    []any{"QUEUE", "INTERRUPT"},
				"default": "QUEUE",
			},
			"ifEnded": map[string]any{
				"type": "string",
				"enum": []any{"REVIVE", "HOLD"},
			},
			"replyIfEnded": map[string]any{
				"type": "string",
				"enum": []any{"REVIVE", "HOLD"},
			},
			"expiresInSeconds": map[string]any{
				"type":    "integer",
				"minimum": 60,
				"maximum": 604800,
				"default": 86400,
			},
			"correlation": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"workId": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
					"factorySessionId": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
				},
			},
		},
		"required": []any{"action", "message", "requestId"},
		"oneOf": []any{
			map[string]any{
				"required": []any{"workerSessionId"},
				"not": map[string]any{
					"required": []any{"inReplyTo"},
				},
			},
			map[string]any{
				"required": []any{"inReplyTo"},
				"not": map[string]any{
					"required": []any{"workerSessionId"},
				},
			},
		},
	}
}

func subagentInboxSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{
				"type":  "string",
				"const": "LIST",
			},
			"messages": map[string]any{
				"type":  "boolean",
				"const": true,
			},
			"factorySessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"threadId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"status": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
					"enum": []any{"QUEUED", "READ", "DELIVERED_REVIVE", "REPLIED", "EXPIRED"},
				},
			},
			"maxResults": map[string]any{
				"type":    "integer",
				"minimum": 1,
				"maximum": 500,
				"default": 100,
			},
			"nextToken": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"markRead": map[string]any{
				"type":    "boolean",
				"default": false,
			},
			"fromWorkerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"correlationWorkId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"correlationFactorySessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
		"required": []any{"action", "messages"},
	}
}

func subagentThreadSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{
				"type":  "string",
				"const": "READ",
			},
			"workerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"messages": map[string]any{
				"type":  "boolean",
				"const": true,
			},
			"factorySessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"threadId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"status": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
					"enum": []any{"QUEUED", "READ", "DELIVERED_REVIVE", "REPLIED", "EXPIRED"},
				},
			},
			"maxResults": map[string]any{
				"type":    "integer",
				"minimum": 1,
				"maximum": 500,
				"default": 100,
			},
			"nextToken": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"markRead": map[string]any{
				"type":    "boolean",
				"default": false,
			},
		},
		"required": []any{"action", "messages", "workerSessionId"},
	}
}

func subagentReviveSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{
				"type":  "string",
				"const": "REVIVE",
			},
			"workerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"message": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"requestId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"successorWorkerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"factorySessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
		"required": []any{"action", "workerSessionId", "message", "requestId", "successorWorkerSessionId"},
	}
}

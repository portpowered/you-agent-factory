package modelmcp

import (
	"strings"
)

const errorCodeBadRequest = "BAD_REQUEST"

func decodeInputErrorEnvelope(context string, err error) ToolErrorEnvelope {
	message := context
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = context + ": " + trimmed
		}
		details["reason"] = err.Error()
	}
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   message,
		Retryable: false,
		Details:   details,
	}
}

func unavailableServiceErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "model.service.unavailable",
		Message:   "models service is unavailable",
		Retryable: false,
	}
}

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	message := "models execution failed"
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = trimmed
		}
		details["reason"] = err.Error()
	}
	return ToolErrorEnvelope{
		Code:      "model.execution.internal",
		Message:   message,
		Retryable: false,
		Details:   details,
	}
}

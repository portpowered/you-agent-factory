package mcp

import (
	"fmt"
	"strings"
)

const errorCodeBadRequest = "BAD_REQUEST"

func decodeInputErrorEnvelope(context string, err error) ToolErrorEnvelope {
	message := context
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = fmt.Sprintf("%s: %s", context, trimmed)
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

func validationErrorEnvelope(err error) ToolErrorEnvelope {
	message := "invalid tool input"
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = trimmed
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

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "factory_runtime.execution.failed",
		Message:   err.Error(),
		Retryable: false,
	}
}

func unavailableRuntimeErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "factory_runtime.runtime.unavailable",
		Message:   "factory runtime is unavailable",
		Retryable: false,
	}
}

package mcp

import (
	"fmt"
)

const errorCodeBadRequest = "BAD_REQUEST"

func decodeInputErrorEnvelope(context string, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   fmt.Sprintf("%s: %v", context, err),
		Retryable: false,
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

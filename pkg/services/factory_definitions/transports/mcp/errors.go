package factorydefinition

import (
	"fmt"
)

const errorCodeBadRequest = "BAD_REQUEST"

func decodeInputErrorEnvelope(operation string, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   fmt.Sprintf("%s: %v", operation, err),
		Retryable: false,
	}
}

func unavailableValidationErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "factory_definition.service.unavailable",
		Message:   "factory definition validation is unavailable",
		Retryable: false,
	}
}

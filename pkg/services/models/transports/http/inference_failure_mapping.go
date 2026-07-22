package http

import (
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func inferenceFailureHTTPStatus(failure *workers.InferenceFailure) int {
	switch failure.Class {
	case workers.InferenceFailureClassMissingModel:
		return http.StatusNotFound
	case workers.InferenceFailureClassLoadingModel:
		return http.StatusConflict
	case workers.InferenceFailureClassUnsupportedOperation:
		return http.StatusBadRequest
	case workers.InferenceFailureClassTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

func inferenceFailureErrorCode(failure *workers.InferenceFailure) string {
	switch failure.Class {
	case workers.InferenceFailureClassMissingModel:
		return "MODEL_NOT_AVAILABLE"
	case workers.InferenceFailureClassLoadingModel:
		return "MODEL_RUNTIME_LOADING"
	case workers.InferenceFailureClassUnsupportedOperation:
		return "BAD_REQUEST"
	case workers.InferenceFailureClassTimeout:
		return "MODEL_INFERENCE_TIMEOUT"
	default:
		return "MODEL_INFERENCE_RUNTIME_FAILURE"
	}
}

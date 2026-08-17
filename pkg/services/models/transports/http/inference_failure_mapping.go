package http

import (
	"errors"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func inferenceFailureHTTPStatus(failure *models.InferenceFailure) int {
	switch failure.Class {
	case models.InferenceFailureClassMissingModel:
		return http.StatusNotFound
	case models.InferenceFailureClassLoadingModel:
		return http.StatusConflict
	case models.InferenceFailureClassUnsupportedOperation:
		return http.StatusBadRequest
	case models.InferenceFailureClassTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

func inferenceFailureErrorCode(failure *models.InferenceFailure) string {
	switch failure.Class {
	case models.InferenceFailureClassMissingModel:
		return "MODEL_NOT_AVAILABLE"
	case models.InferenceFailureClassLoadingModel:
		return "MODEL_RUNTIME_LOADING"
	case models.InferenceFailureClassUnsupportedOperation:
		return "BAD_REQUEST"
	case models.InferenceFailureClassTimeout:
		return "MODEL_INFERENCE_TIMEOUT"
	default:
		return "MODEL_INFERENCE_RUNTIME_FAILURE"
	}
}

// classifiedInferenceErrorResponse maps an already-classified inference failure
// anywhere in err's chain to its public response. It reports false when err
// carries no classification, leaving the caller's sentinel matching to decide.
func classifiedInferenceErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	var failure *models.InferenceFailure
	if !errors.As(err, &failure) || failure == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}
	return inferenceFailureErrorResponse(failure)
}

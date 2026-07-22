package factorysession

import (
	"errors"
	"net/http"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ExecutionErrorResponse maps durable execution contract errors to HTTP status and
// the public ErrorResponse shape. It returns false when err is not a known
// execution contract failure.
func ExecutionErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	var requestValidationErr *apisurface.RequestValidationError
	if errors.As(err, &requestValidationErr) {
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: requestValidationErr.Error(),
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCodeBADREQUEST,
		}, true
	}

	var validationErr *factorysessionexecution.ExecutionValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: validationErr.Message,
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCodeBADREQUEST,
		}, true
	}

	if errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
		return http.StatusConflict, factoryapi.ErrorResponse{
			Message: "requestId was already used with different execution inputs.",
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCodeEXECUTIONREQUESTIDCONFLICT,
		}, true
	}

	var resumeErr *factorysessionexecution.ResumeError
	if errors.As(err, &resumeErr) {
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: resumeErr.Error(),
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCodeBADREQUEST,
		}, true
	}

	return 0, factoryapi.ErrorResponse{}, false
}

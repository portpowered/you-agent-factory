package factorysession

import (
	"errors"
	"net/http"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// DurableHistoryFailure classifies one durable Factory Session history failure
// so a compatibility transport can choose its response without naming the
// Factory Sessions error contract.
type DurableHistoryFailure int

const (
	// DurableHistoryFailureUnclassified reports that the error is not a known
	// durable history failure.
	DurableHistoryFailureUnclassified DurableHistoryFailure = iota
	// DurableHistoryFailureSessionNotFound reports a missing live or durable session.
	DurableHistoryFailureSessionNotFound
	// DurableHistoryFailureDispatchNotFound reports a missing dispatch.
	DurableHistoryFailureDispatchNotFound
	// DurableHistoryFailureArtifactNotFound reports a missing session artifact.
	DurableHistoryFailureArtifactNotFound
	// DurableHistoryFailureReconnectCursorNotFound reports a stale reconnect cursor.
	DurableHistoryFailureReconnectCursorNotFound
)

// ClassifyDurableHistoryFailure classifies one durable Factory Session history
// error. Missing-session detection is checked first so a wrapped session
// failure never reports as a narrower dispatch or artifact failure.
func ClassifyDurableHistoryFailure(err error) DurableHistoryFailure {
	if err == nil {
		return DurableHistoryFailureUnclassified
	}
	switch {
	case errors.Is(err, factorysessionexecution.ErrDurableSessionNotFound),
		errors.Is(err, factorysessionexecution.ErrSessionNotFound):
		return DurableHistoryFailureSessionNotFound
	case errors.Is(err, factorysessionexecution.ErrDispatchNotFound):
		return DurableHistoryFailureDispatchNotFound
	case errors.Is(err, factorysessionexecution.ErrArtifactNotFound):
		return DurableHistoryFailureArtifactNotFound
	case errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound):
		return DurableHistoryFailureReconnectCursorNotFound
	default:
		return DurableHistoryFailureUnclassified
	}
}

// NewExecutionValidationError builds the shared durable execution validation
// failure for transports that reject a request field before it reaches the
// service. ExecutionErrorResponse maps the result to 400 BAD_REQUEST.
func NewExecutionValidationError(field, message string) error {
	return &factorysessionexecution.ExecutionValidationError{Field: field, Message: message}
}

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

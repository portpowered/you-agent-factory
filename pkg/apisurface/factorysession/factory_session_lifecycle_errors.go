package factorysession

import (
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// LifecycleControlSuccessStatus maps one accepted lifecycle-control result to the
// HTTP success status for the public durable control routes.
func LifecycleControlSuccessStatus(result factorysessionexecution.LifecycleControlResult) int {
	switch result.Outcome {
	case factorysessionexecution.LifecycleControlOutcomeNoOp:
		return http.StatusOK
	case factorysessionexecution.LifecycleControlOutcomeAccepted:
		if result.Status == factorysessionexecution.LifecycleStatusCanceling {
			return http.StatusAccepted
		}
		return http.StatusOK
	default:
		return http.StatusOK
	}
}

// LifecycleControlErrorResponse maps durable lifecycle-control contract errors to
// HTTP status and the public response shape. It returns false when err is not a
// known lifecycle-control contract failure.
func LifecycleControlErrorResponse(sessionID string, err error) (int, any, bool) {
	if err == nil {
		return 0, nil, false
	}

	var controlErr *factorysessionexecution.ControlError
	if errors.As(err, &controlErr) {
		return http.StatusConflict, ControlErrorToAPI(sessionID, controlErr), true
	}

	if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "factory session not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.NOTFOUND,
		}, true
	}

	if errors.Is(err, factorysessionexecution.ErrDispatchNotFound) {
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "dispatch not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.NOTFOUND,
		}, true
	}

	var validationErr *factorysessionexecution.ValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: validationErr.Message,
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.BADREQUEST,
		}, true
	}

	return 0, nil, false
}

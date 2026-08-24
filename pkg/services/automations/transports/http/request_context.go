package http

import (
	"context"
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Request-context outcomes at the Automations HTTP adapter edge:
//
//   - Canceled requests terminate without an ErrorResponse body.
//   - Deadline exhaustion returns 504 with a public ErrorResponse.
//   - Context cancellation and deadline exhaustion must not be mapped to
//     INTERNAL_ERROR.
//
// automationsRequestContextErrorResponse maps request-context cancellation and
// deadline failures to the adapter's documented HTTP outcomes.
func automationsRequestContextErrorResponse(err error) (status int, response any, handled bool) {
	if err == nil {
		return 0, nil, false
	}
	switch {
	case errors.Is(err, context.Canceled):
		return 0, nil, true
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, factoryapi.ErrorResponse{
			Message: "automations request timed out",
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCodeINTERNALERROR,
		}, true
	default:
		return 0, nil, false
	}
}

func (a *Adapter) writeAutomationsRequestContextOutcome(w http.ResponseWriter, err error) bool {
	status, response, ok := automationsRequestContextErrorResponse(err)
	if !ok {
		return false
	}
	if response == nil {
		return true
	}
	a.writeJSON(w, status, response)
	return true
}

func guardRequestContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

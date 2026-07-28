package http

import (
	"context"
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const operatorSettingsRequestTimedOutMessage = "operator settings request timed out"

// Request-context outcomes at the Operator Settings HTTP adapter edge:
//
//   - Canceled requests terminate without an ErrorResponse body.
//   - Deadline exhaustion returns 504 with a public ErrorResponse.
//   - Context cancellation and deadline exhaustion must not be mapped to
//     INTERNAL_ERROR.
func isRequestContextEnded(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func requestContextEnded(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return isRequestContextEnded(ctx.Err())
}

func shouldEndOnRequestContext(ctx context.Context, err error) bool {
	return requestContextEnded(ctx) || isRequestContextEnded(err)
}

// settingsRequestContextErrorResponse maps request-context cancellation and
// deadline failures to the adapter's documented HTTP outcomes.
func settingsRequestContextErrorResponse(err error) (status int, response any, handled bool) {
	if err == nil {
		return 0, nil, false
	}
	switch {
	case errors.Is(err, context.Canceled):
		return 0, nil, true
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, factoryapi.ErrorResponse{
			Message: operatorSettingsRequestTimedOutMessage,
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCodeINTERNALERROR,
		}, true
	default:
		return 0, nil, false
	}
}

func (a *Adapter) writeSettingsRequestContextOutcome(w http.ResponseWriter, err error) bool {
	status, response, ok := settingsRequestContextErrorResponse(err)
	if !ok {
		return false
	}
	if response == nil {
		return true
	}
	a.writeJSON(w, status, response)
	return true
}

func (a *Adapter) guardSettingsRequestContext(w http.ResponseWriter, r *http.Request) bool {
	return a.writeSettingsRequestContextOutcome(w, r.Context().Err())
}

func guardRequestContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

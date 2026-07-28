package http

import (
	"context"
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// runtimeRequestContextErrorResponse maps request-context cancellation and
// deadline failures to the adapter's documented HTTP outcomes. Canceled
// requests terminate without an error body; deadline failures return 504.
func runtimeRequestContextErrorResponse(err error) (status int, response any, handled bool) {
	if err == nil {
		return 0, nil, false
	}
	switch {
	case errors.Is(err, context.Canceled):
		return 0, nil, true
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, factoryapi.ErrorResponse{
			Message: "factory runtime request timed out",
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCodeINTERNALERROR,
		}, true
	default:
		return 0, nil, false
	}
}

func requestContextEnded(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return isRequestContextEnded(ctx.Err())
}

func isRequestContextEnded(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func shouldEndOnRequestContext(ctx context.Context, err error) bool {
	return requestContextEnded(ctx) || isRequestContextEnded(err)
}

func (a *Adapter) writeRuntimeRequestContextOutcome(w http.ResponseWriter, err error) bool {
	status, response, ok := runtimeRequestContextErrorResponse(err)
	if !ok {
		return false
	}
	if response == nil {
		return true
	}
	a.writeJSON(w, status, response)
	return true
}

func (a *Adapter) guardRuntimeRequestContext(w http.ResponseWriter, r *http.Request) bool {
	return a.writeRuntimeRequestContextOutcome(w, r.Context().Err())
}

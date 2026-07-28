package http

import (
	"context"
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// visualizationRequestContextErrorResponse maps request-context cancellation and
// deadline failures to the adapter's documented HTTP outcomes. Canceled
// requests terminate without an error body; deadline failures return 504.
func visualizationRequestContextErrorResponse(err error) (status int, response any, handled bool) {
	if err == nil {
		return 0, nil, false
	}
	switch {
	case errors.Is(err, context.Canceled):
		return 0, nil, true
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, factoryapi.ErrorResponse{
			Message: "factory visualization request timed out",
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCodeINTERNALERROR,
		}, true
	default:
		return 0, nil, false
	}
}

func (a *Adapter) writeVisualizationRequestContextOutcome(w http.ResponseWriter, err error) bool {
	status, response, ok := visualizationRequestContextErrorResponse(err)
	if !ok {
		return false
	}
	if response == nil {
		return true
	}
	a.writeJSON(w, status, response)
	return true
}

func visualizationContextBeforeRoot(ctx context.Context) error {
	return ctx.Err()
}

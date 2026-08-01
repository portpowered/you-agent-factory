package common

import (
	"context"
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RequestContextErrorResponse maps request cancellation and deadline failures
// to the adapter's documented HTTP outcomes. Canceled requests terminate
// without an error body; deadline failures return 504.
func RequestContextErrorResponse(err error) (status int, response any, handled bool) {
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

// RequestContextEnded reports whether a request context has been canceled or
// exceeded its deadline.
func RequestContextEnded(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return IsRequestContextEnded(ctx.Err())
}

// IsRequestContextEnded reports whether err represents request cancellation or
// deadline expiration.
func IsRequestContextEnded(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// ShouldEndOnRequestContext combines the context and operation error cases
// used by handler cancellation tests and response framing.
func ShouldEndOnRequestContext(ctx context.Context, err error) bool {
	return RequestContextEnded(ctx) || IsRequestContextEnded(err)
}

// WriteRequestContextOutcome writes the documented cancellation/deadline
// outcome and reports whether the request is complete.
func WriteRequestContextOutcome(w http.ResponseWriter, err error) bool {
	status, response, ok := RequestContextErrorResponse(err)
	if !ok {
		return false
	}
	if response == nil {
		return true
	}
	WriteJSON(w, status, response)
	return true
}

// GuardRequestContext ends a handler before any domain call when the request
// is already canceled or expired.
func GuardRequestContext(w http.ResponseWriter, r *http.Request) bool {
	return WriteRequestContextOutcome(w, r.Context().Err())
}

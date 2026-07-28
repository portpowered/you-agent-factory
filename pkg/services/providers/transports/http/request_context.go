package http

import (
	"context"
	"errors"
	"net/http"
)

// Request-context outcomes at the Providers execute HTTP adapter edge:
//
//   - Canceled requests surface the published execute cancel HTTP outcome.
//   - Deadline exhaustion surfaces the published execute timeout HTTP outcome.
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

// executeRequestContextErrorResponse maps request-context cancellation and
// deadline failures to the Providers execute HTTP outcomes documented at the
// adapter edge.
func executeRequestContextErrorResponse(err error) (status int, response any, handled bool) {
	if err == nil {
		return 0, nil, false
	}
	switch {
	case errors.Is(err, context.Canceled):
		status, resp, _ := internalErrorResponseWithCode(executeCanceledMessage, executeErrorCodeCanceled)
		return status, resp, true
	case errors.Is(err, context.DeadlineExceeded):
		status, resp, _ := gatewayTimeoutErrorResponse(executeTimeoutMessage, executeErrorCodeTimeout)
		return status, resp, true
	default:
		return 0, nil, false
	}
}

func (a *Adapter) writeExecuteRequestContextOutcome(w http.ResponseWriter, err error) bool {
	status, response, ok := executeRequestContextErrorResponse(err)
	if !ok {
		return false
	}
	a.writeJSON(w, status, response)
	return true
}

package http

import (
	"context"
	"errors"
)

// Request-context outcomes at the Recordings HTTP adapter edge:
//
//   - Stream operations end without writing an ErrorResponse once SSE headers
//     may already be committed. The handler returns promptly when the inbound
//     request context is canceled or its deadline is exceeded.
//   - Non-stream operations return without encoding a success or ErrorResponse
//     body when the request context ends before a response is written. Context
//     cancellation and deadline exhaustion must not be mapped to INTERNAL_ERROR.
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

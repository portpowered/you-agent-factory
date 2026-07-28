package http

import (
	"context"
)

func invokeWithRequestContext[T any](
	ctx context.Context,
	invoke func() (T, error),
) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if err := guardRequestContext(ctx); err != nil {
		return zero, err
	}

	type invokeResult struct {
		value T
		err   error
	}
	resultCh := make(chan invokeResult, 1)
	go func() {
		value, err := invoke()
		resultCh <- invokeResult{value: value, err: err}
	}()

	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case result := <-resultCh:
		return result.value, result.err
	}
}

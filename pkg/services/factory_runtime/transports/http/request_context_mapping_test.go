package http

import (
	"context"
	"errors"
	"testing"
)

func TestShouldEndOnRequestContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !shouldEndOnRequestContext(ctx, nil) {
		t.Fatal("canceled request context should end the handler")
	}
	if !shouldEndOnRequestContext(context.Background(), context.Canceled) {
		t.Fatal("context.Canceled error should end the handler")
	}
	if !shouldEndOnRequestContext(context.Background(), context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded error should end the handler")
	}
	if shouldEndOnRequestContext(context.Background(), errors.New("boom")) {
		t.Fatal("unrelated errors must not end the handler")
	}
}

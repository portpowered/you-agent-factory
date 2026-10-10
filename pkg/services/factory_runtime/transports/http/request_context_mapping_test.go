package http

import (
	"context"
	"errors"
	runtimehttpcommon "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	"testing"
)

func TestShouldEndOnRequestContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !runtimehttpcommon.ShouldEndOnRequestContext(ctx, nil) {
		t.Fatal("canceled request context should end the handler")
	}
	if !runtimehttpcommon.ShouldEndOnRequestContext(context.Background(), context.Canceled) {
		t.Fatal("context.Canceled error should end the handler")
	}
	if !runtimehttpcommon.ShouldEndOnRequestContext(context.Background(), context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded error should end the handler")
	}
	if runtimehttpcommon.ShouldEndOnRequestContext(context.Background(), errors.New("boom")) {
		t.Fatal("unrelated errors must not end the handler")
	}
}

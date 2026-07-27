package service

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestRootPullModelForScopeValidatesBeforeRuntimeResolution(t *testing.T) {
	t.Parallel()

	root := &Root{}
	if _, err := root.PullModelForScope(t.Context(), models.PullModelRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("empty pull request error = %v, want ErrNotFound", err)
	}
	if _, err := root.PullModelForScope(t.Context(), models.PullModelRequest{Name: "voice"}); !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("unavailable scoped runtime error = %v, want ErrUnsupportedOperation", err)
	}
}

func TestRuntimeServicePullModelForScopeValidatesAndDelegates(t *testing.T) {
	t.Parallel()

	runtime := &runtimeService{}
	if _, err := runtime.PullModelForScope(context.Background(), models.PullModelRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("empty pull request error = %v, want ErrNotFound", err)
	}
	if _, err := runtime.PullModelForScope(context.Background(), models.PullModelRequest{Name: "voice"}); err == nil {
		t.Fatal("delegated pull error = nil, want unavailable runtime failure")
	}
}

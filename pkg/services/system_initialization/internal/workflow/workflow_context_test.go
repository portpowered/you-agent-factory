package workflow

import (
	"context"
	"errors"
	"reflect"
	"testing"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

func TestInitializeRequiresContext(t *testing.T) {
	t.Parallel()

	result, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).
		Initialize(nil, systeminitialization.Request{HomeDir: t.TempDir()})
	if err == nil {
		t.Fatal("Initialize(nil) error = nil")
	}
	if !reflect.DeepEqual(result, systeminitialization.Result{}) {
		t.Fatalf("Initialize(nil) result = %#v, want zero result", result)
	}
}

func TestInitializePreservesCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).
		Initialize(ctx, systeminitialization.Request{HomeDir: t.TempDir()})
	if !errors.Is(err, systeminitialization.ErrInitializeCancelled) {
		t.Fatalf("Initialize(canceled) error = %v, want ErrInitializeCancelled", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Initialize(canceled) error = %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(result, systeminitialization.Result{}) {
		t.Fatalf("Initialize(canceled) result = %#v, want zero result", result)
	}
}

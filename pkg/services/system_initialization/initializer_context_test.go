package systeminitialization

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestInitializeRequiresContext(t *testing.T) {
	t.Parallel()

	result, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).Initialize(nil, Request{HomeDir: t.TempDir()})
	if err == nil {
		t.Fatal("Initialize(nil) error = nil")
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("Initialize(nil) result = %#v, want zero result", result)
	}
}

func TestInitializePreservesCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).Initialize(ctx, Request{HomeDir: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Initialize(canceled) error = %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("Initialize(canceled) result = %#v, want zero result", result)
	}
}

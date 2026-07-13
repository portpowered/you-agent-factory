package startup

import (
	"context"
	"errors"
	"testing"
)

func TestLifecycleFuncRunsWithSuppliedContextAndResult(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), lifecycleContextKey{}, "value")
	wantErr := errors.New("lifecycle failed")
	called := 0
	lifecycle := LifecycleFunc(func(got context.Context) error {
		called++
		if got != ctx {
			t.Fatal("LifecycleFunc.Run did not preserve context identity")
		}
		return wantErr
	})

	if err := lifecycle.Run(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("LifecycleFunc.Run() error = %v, want %v", err, wantErr)
	}
	if called != 1 {
		t.Fatalf("lifecycle calls = %d, want 1", called)
	}
}

type lifecycleContextKey struct{}

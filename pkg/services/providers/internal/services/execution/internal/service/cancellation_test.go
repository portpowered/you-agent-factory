package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestExecutePropagatesCancellationAndDeadlineAndCleansUpOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newContext func(t *testing.T, started <-chan struct{}) (context.Context, context.CancelFunc)
		want       error
		wantKind   providers.ExecuteFailureKind
	}{
		{
			name: "canceled",
			newContext: func(_ *testing.T, started <-chan struct{}) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					<-started
					cancel()
				}()
				return ctx, cancel
			},
			want:     providers.ErrExecuteCancelled,
			wantKind: providers.ExecuteFailureKindCanceled,
		},
		{
			name: "deadline",
			newContext: func(_ *testing.T, _ <-chan struct{}) (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 100*time.Millisecond)
			},
			want:     providers.ErrExecuteTimeout,
			wantKind: providers.ExecuteFailureKindTimeout,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			started := make(chan struct{})
			cleanupCalls := 0
			executionService := mustExecutionService(t, func(
				ctx context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				close(started)
				defer func() { cleanupCalls++ }()
				<-ctx.Done()
				return providers.ExecuteResult{Content: "must not escape"}, ctx.Err()
			})
			ctx, cancel := test.newContext(t, started)
			defer cancel()
			result, executeErr := executionService.Execute(ctx, providers.ExecuteRequest{
				Provider:  providers.IDCodex,
				AttemptID: "attempt-1",
			})
			if !errors.Is(executeErr, test.want) {
				t.Fatalf("Execute() error = %v, want %v", executeErr, test.want)
			}
			var failure providers.ExecuteFailure
			if !errors.As(executeErr, &failure) || failure.Kind != test.wantKind {
				t.Fatalf("Execute() error = %#v, want kind %q", executeErr, test.wantKind)
			}
			if cleanupCalls != 1 {
				t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
			}
			if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
				t.Fatalf("Execute() result = %#v, want zero terminal result", result)
			}
		})
	}
}

func TestExecuteNormalizesCancellationDuringCatalogLookup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapterCalls := 0
	catalogService := &recordingCatalog{
		get: func(
			lookupContext context.Context,
			_ providers.GetProviderRequest,
		) (providers.GetProviderResult, error) {
			cancel()
			return providers.GetProviderResult{}, lookupContext.Err()
		},
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{Content: "must not escape"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	result, executeErr := executionService.Execute(ctx, providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
	})
	if !errors.Is(executeErr, providers.ErrExecuteCancelled) {
		t.Fatalf("Execute() error = %v, want %v", executeErr, providers.ErrExecuteCancelled)
	}
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("Execute() error = %#v, want canceled ExecuteFailure", executeErr)
	}
	if adapterCalls != 0 || !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("Execute() = (%#v, %d adapter calls), want zero result and no adapter I/O", result, adapterCalls)
	}
}

func TestExecuteRejectsPreTerminatedContextBeforeAdapterIO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		want       error
		wantKind   providers.ExecuteFailureKind
	}{
		{
			name: "canceled",
			newContext: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			want:     providers.ErrExecuteCancelled,
			wantKind: providers.ExecuteFailureKindCanceled,
		},
		{
			name: "deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Unix(0, 0))
			},
			want:     providers.ErrExecuteTimeout,
			wantKind: providers.ExecuteFailureKindTimeout,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapterCalls := 0
			executionService := mustExecutionService(t, func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{Content: "must not escape"}, nil
			})
			ctx, cancel := test.newContext()
			defer cancel()

			result, executeErr := executionService.Execute(ctx, providers.ExecuteRequest{
				Provider:  providers.IDCodex,
				AttemptID: "attempt-1",
			})
			if !errors.Is(executeErr, test.want) {
				t.Fatalf("Execute() error = %v, want %v", executeErr, test.want)
			}
			var failure providers.ExecuteFailure
			if !errors.As(executeErr, &failure) || failure.Kind != test.wantKind {
				t.Fatalf("Execute() error = %#v, want kind %q", executeErr, test.wantKind)
			}
			if adapterCalls != 0 || !reflect.DeepEqual(result, providers.ExecuteResult{}) {
				t.Fatalf("Execute() = (%#v, %d adapter calls), want zero result and no adapter I/O", result, adapterCalls)
			}
		})
	}
}

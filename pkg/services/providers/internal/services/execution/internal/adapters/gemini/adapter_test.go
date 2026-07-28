package gemini_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	gemini "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/gemini"
)

const geminiFailureSecret = "prompt-secret-that-must-not-escape"

func TestGeminiRootPreservesRequestAndFinalStdout(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDGemini,
		AttemptID:        "attempt-gemini-success",
		Model:            "gemini-2.5-pro",
		UserMessage:      "perform the accepted work",
		WorkingDirectory: "C:/factory",
	}
	const content = "gemini final answer"
	var received providers.ExecuteRequest
	effect := gemini.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (gemini.EffectResult, error) {
		received = got.Clone()
		if err := observe([]byte(content)); err != nil {
			return gemini.EffectResult{}, err
		}
		return gemini.EffectResult{DurationMillis: 17}, nil
	})
	root := newGeminiRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(received, request) {
		t.Fatalf("native request = %#v, want %#v", received, request)
	}
	if result.Content != content {
		t.Fatalf("Content = %q, want %q", result.Content, content)
	}
	if result.SessionRef != nil {
		t.Fatalf("SessionRef = %#v, want nil", result.SessionRef)
	}
	if result.Diagnostics == nil || result.Diagnostics.DurationMillis != 17 {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
}

func TestGeminiRootNormalizesFailureStagesAndSuppressesResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		effectErr error
		wantKind  providers.ExecuteFailureKind
	}{
		{
			name:      "native exit",
			effectErr: errors.New("exit included " + geminiFailureSecret),
			wantKind:  providers.ExecuteFailureKindUnknown,
		},
		{
			name: "declared throttling beats native exit",
			effectErr: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindThrottled,
			},
			wantKind: providers.ExecuteFailureKindThrottled,
		},
		{
			name:      "recognized native failure",
			effectErr: providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication},
			wantKind:  providers.ExecuteFailureKindAuthentication,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var cleanups atomic.Int32
			effect := gemini.EffectFunc(func(
				_ context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (gemini.EffectResult, error) {
				defer cleanups.Add(1)
				return gemini.EffectResult{}, test.effectErr
			})

			for iteration := 0; iteration < 10; iteration++ {
				result, err := newGeminiRoot(t, effect).Execute(
					t.Context(),
					geminiFailureRequest(),
				)
				assertGeminiFailure(t, result, err, test.wantKind)
			}
			if got := cleanups.Load(); got != 10 {
				t.Fatalf("cleanup calls = %d, want 10", got)
			}
		})
	}
}

func TestGeminiRootCancellationAndDeadlineReachEffectAndCleanUpOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		want       error
	}{
		{
			name: "cancellation",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(t.Context())
			},
			want: providers.ErrExecuteCancelled,
		},
		{
			name: "deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(t.Context(), 50*time.Millisecond)
			},
			want: providers.ErrExecuteTimeout,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			started := make(chan struct{})
			var cleanups atomic.Int32
			effect := gemini.EffectFunc(func(
				ctx context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (gemini.EffectResult, error) {
				close(started)
				defer cleanups.Add(1)
				<-ctx.Done()
				return gemini.EffectResult{}, ctx.Err()
			})
			ctx, cancel := test.newContext()
			defer cancel()
			root := newGeminiRoot(t, effect)
			outcome := make(chan error, 1)
			go func() {
				_, err := root.Execute(ctx, geminiFailureRequest())
				outcome <- err
			}()
			<-started
			if test.want == providers.ErrExecuteCancelled {
				cancel()
			}

			select {
			case err := <-outcome:
				if !errors.Is(err, test.want) {
					t.Fatalf("Execute() error = %v, want %v", err, test.want)
				}
			case <-time.After(time.Second):
				t.Fatal("Execute() did not stop after context ended")
			}
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("cleanup calls = %d, want 1", got)
			}
		})
	}
}

func assertGeminiFailure(
	t *testing.T,
	result providers.ExecuteResult,
	err error,
	wantKind providers.ExecuteFailureKind,
) {
	t.Helper()
	if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("failed Execute() result = %#v, want zero result", result)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != wantKind {
		t.Fatalf("Execute() error = %#v, want kind %q", err, wantKind)
	}
	if strings.Contains(err.Error(), geminiFailureSecret) {
		t.Fatalf("Execute() error leaked sensitive facts: %v", err)
	}
}

func geminiFailureRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:    providers.IDGemini,
		AttemptID:   "attempt-gemini-failure",
		UserMessage: "deterministic failure prompt",
	}
}

func newGeminiRoot(t *testing.T, effect gemini.Effect) providers.Service {
	t.Helper()
	root, err := newGeminiConformanceRoot(gemini.NewRegistration(effect).Attempt)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

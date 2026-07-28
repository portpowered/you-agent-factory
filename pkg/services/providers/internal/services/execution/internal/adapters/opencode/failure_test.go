package opencode_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	opencode "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
)

const openCodeFailureSecret = "prompt-secret-that-must-not-escape"

func TestOpenCodeRootNormalizesFailureStagesAndSuppressesResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stream    string
		effectErr error
		wantKind  providers.ExecuteFailureKind
		wantStage string
	}{
		{
			name:      "native exit",
			effectErr: errors.New("exit included " + openCodeFailureSecret),
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "native",
		},
		{
			name:      "malformed JSONL",
			stream:    `{"type":"text","part":{"secret":"` + openCodeFailureSecret,
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "decode",
		},
		{
			name:      "incomplete final state",
			stream:    "{\"type\":\"step_start\",\"sessionID\":\"session-partial\"}\n",
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "final_parse",
		},
		{
			name: "declared stream failure beats native exit",
			stream: "{\"type\":\"error\",\"sessionID\":\"session-error\",\"error\":{\"name\":\"unexpected status 429 " +
				openCodeFailureSecret + "\"}}\n",
			effectErr: errors.New("exit included " + openCodeFailureSecret),
			wantKind:  providers.ExecuteFailureKindUnknown,
		},
		{
			name: "recognized native failure beats unknown stream failure",
			stream: "{\"type\":\"error\",\"error\":{\"name\":\"" +
				openCodeFailureSecret + "\"}}\n",
			effectErr: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindAuthentication,
			},
			wantKind: providers.ExecuteFailureKindAuthentication,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var cleanups atomic.Int32
			effect := opencode.EffectFunc(func(
				_ context.Context,
				_ providers.ExecuteRequest,
				observe func([]byte) error,
			) (opencode.EffectResult, error) {
				defer cleanups.Add(1)
				if test.stream != "" {
					if err := observe([]byte(test.stream)); err != nil {
						return opencode.EffectResult{}, err
					}
				}
				return opencode.EffectResult{}, test.effectErr
			})

			for iteration := 0; iteration < 10; iteration++ {
				result, err := newOpenCodeRoot(t, effect, opencode.ModeStructured).Execute(
					t.Context(),
					openCodeFailureRequest(),
				)
				assertOpenCodeFailure(t, result, err, test.wantKind, test.wantStage)
			}
			if got := cleanups.Load(); got != 10 {
				t.Fatalf("cleanup calls = %d, want 10", got)
			}
		})
	}
}

func TestOpenCodeRootCancellationAndDeadlineReachEffectAndCleanUpOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		want       error
		wantKind   providers.ExecuteFailureKind
	}{
		{
			name: "cancellation",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			want:     providers.ErrExecuteCancelled,
			wantKind: providers.ExecuteFailureKindCanceled,
		},
		{
			name: "deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 50*time.Millisecond)
			},
			want:     providers.ErrExecuteTimeout,
			wantKind: providers.ExecuteFailureKindTimeout,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			started := make(chan struct{})
			var cleanups atomic.Int32
			effect := opencode.EffectFunc(func(
				ctx context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (opencode.EffectResult, error) {
				close(started)
				defer cleanups.Add(1)
				<-ctx.Done()
				return opencode.EffectResult{}, ctx.Err()
			})
			ctx, cancel := test.newContext()
			defer cancel()
			root := newOpenCodeRoot(t, effect, opencode.ModeStructured)
			outcome := make(chan struct {
				result providers.ExecuteResult
				err    error
			}, 1)
			go func() {
				result, err := root.Execute(ctx, openCodeFailureRequest())
				outcome <- struct {
					result providers.ExecuteResult
					err    error
				}{result: result, err: err}
			}()
			<-started
			if test.wantKind == providers.ExecuteFailureKindCanceled {
				cancel()
			}

			select {
			case got := <-outcome:
				if !errors.Is(got.err, test.want) {
					t.Fatalf("Execute() error = %v, want %v", got.err, test.want)
				}
				assertOpenCodeFailure(t, got.result, got.err, test.wantKind, "")
			case <-time.After(time.Second):
				t.Fatal("Execute() did not stop after context termination")
			}
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("cleanup calls = %d, want 1", got)
			}
		})
	}
}

func TestOpenCodeRootFailureSuppressesPreviouslyObservedSuccess(t *testing.T) {
	t.Parallel()

	effect := opencode.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (opencode.EffectResult, error) {
		for _, chunk := range splitEvery(openCodeStructuredSuccessStream(), 17) {
			if err := observe(chunk); err != nil {
				return opencode.EffectResult{}, err
			}
		}
		return opencode.EffectResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindAuthentication,
			Message: "credential " + openCodeFailureSecret,
		}
	})
	result, err := newOpenCodeRoot(t, effect, opencode.ModeStructured).Execute(
		t.Context(),
		openCodeFailureRequest(),
	)
	assertOpenCodeFailure(
		t,
		result,
		err,
		providers.ExecuteFailureKindAuthentication,
		"",
	)
}

func openCodeFailureRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:     providers.IDOpenCode,
		AttemptID:    "attempt-opencode-failure",
		SystemPrompt: openCodeFailureSecret,
		UserMessage:  "run failure fixture",
	}
}

func assertOpenCodeFailure(
	t *testing.T,
	result providers.ExecuteResult,
	err error,
	wantKind providers.ExecuteFailureKind,
	wantStage string,
) {
	t.Helper()
	if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("Execute() result = %#v, want zero result", result)
	}
	if !errors.Is(err, sentinelForKind(wantKind)) {
		t.Fatalf("Execute() error = %v, want sentinel for %q", err, wantKind)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != wantKind {
		t.Fatalf("Execute() error = %#v, want failure kind %q", err, wantKind)
	}
	if strings.Contains(err.Error(), openCodeFailureSecret) {
		t.Fatalf("Execute() error leaked sensitive native data: %v", err)
	}
	gotStage := ""
	if failure.Diagnostics != nil {
		gotStage = failure.Diagnostics.Metadata["failure_stage"]
	}
	if gotStage != wantStage {
		t.Fatalf("failure stage = %q, want %q", gotStage, wantStage)
	}
}

func sentinelForKind(kind providers.ExecuteFailureKind) error {
	switch kind {
	case providers.ExecuteFailureKindCanceled:
		return providers.ErrExecuteCancelled
	case providers.ExecuteFailureKindTimeout:
		return providers.ErrExecuteTimeout
	default:
		return providers.ErrExecuteFailed
	}
}

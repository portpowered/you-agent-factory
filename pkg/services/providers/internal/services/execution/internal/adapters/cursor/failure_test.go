package cursor_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	cursor "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const cursorFailureSecret = "prompt-secret-that-must-not-escape"

func TestCursorRootNormalizesFailureStagesAndSuppressesResults(t *testing.T) {
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
			effectErr: errors.New("exit included " + cursorFailureSecret),
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "native",
		},
		{
			name:      "malformed stream-json",
			stream:    `{"type":"system","subtype":"init","secret":"` + cursorFailureSecret,
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "decode",
		},
		{
			name:      "incomplete final state",
			stream:    "{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-session-partial\"}\n",
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "final_parse",
		},
		{
			name: "declared stream failure beats native exit",
			stream: "{\"type\":\"result\",\"subtype\":\"error\",\"is_error\":true,\"result\":\"unexpected status 429 " +
				cursorFailureSecret + "\",\"session_id\":\"\"}\n",
			effectErr: errors.New("exit included " + cursorFailureSecret),
			wantKind:  providers.ExecuteFailureKindUnknown,
		},
		{
			name: "recognized native failure beats unknown stream failure",
			stream: "{\"type\":\"result\",\"subtype\":\"error\",\"is_error\":true,\"result\":\"" +
				cursorFailureSecret + "\",\"session_id\":\"\"}\n",
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
			effect := cursor.EffectFunc(func(
				_ context.Context,
				_ providers.ExecuteRequest,
				observe func([]byte) error,
			) (cursor.EffectResult, error) {
				defer cleanups.Add(1)
				if test.stream != "" {
					if err := observe([]byte(test.stream)); err != nil {
						return cursor.EffectResult{}, err
					}
				}
				return cursor.EffectResult{}, test.effectErr
			})

			for iteration := 0; iteration < 10; iteration++ {
				result, err := newCursorRoot(t, effect).Execute(
					t.Context(),
					cursorFailureRequest(),
				)
				assertCursorFailure(t, result, err, test.wantKind, test.wantStage)
			}
			if got := cleanups.Load(); got != 10 {
				t.Fatalf("cleanup calls = %d, want 10", got)
			}
		})
	}
}

func TestCursorRootCancellationAndDeadlineReachEffectAndCleanUpOnce(t *testing.T) {
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
			effect := cursor.EffectFunc(func(
				ctx context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (cursor.EffectResult, error) {
				close(started)
				defer cleanups.Add(1)
				<-ctx.Done()
				return cursor.EffectResult{}, ctx.Err()
			})
			ctx, cancel := test.newContext()
			defer cancel()
			root := newCursorRoot(t, effect)
			outcome := make(chan struct {
				result providers.ExecuteResult
				err    error
			}, 1)
			go func() {
				result, err := root.Execute(ctx, cursorFailureRequest())
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
				assertCursorFailure(t, got.result, got.err, test.wantKind, "")
			case <-time.After(time.Second):
				t.Fatal("Execute() did not stop after context termination")
			}
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("cleanup calls = %d, want 1", got)
			}
		})
	}
}

func TestCursorRootFailureSuppressesPreviouslyObservedSuccess(t *testing.T) {
	t.Parallel()

	effect := cursor.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (cursor.EffectResult, error) {
		if err := observe(cursorSuccessStream()); err != nil {
			return cursor.EffectResult{}, err
		}
		return cursor.EffectResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindAuthentication,
			Message: "credential " + cursorFailureSecret,
		}
	})
	result, err := newCursorRoot(t, effect).Execute(t.Context(), cursorFailureRequest())
	assertCursorFailure(
		t,
		result,
		err,
		providers.ExecuteFailureKindAuthentication,
		"",
	)
}

func TestCommandEffectCancellationCleansWindowsPromptFileOnce(t *testing.T) {
	t.Parallel()

	files := newPromptFileSystem()
	runner := &blockingCommandRunner{started: make(chan struct{})}
	effect := cursor.NewCommandEffect(runner, cursor.CommandEffectOptions{
		OperatingSystem: "windows",
		TemporaryDir:    `C:\cursor-temp`,
		TemporaryFiles:  files,
	})
	ctx, cancel := context.WithCancel(context.Background())
	outcome := make(chan error, 1)
	go func() {
		_, err := effect.Execute(ctx, providers.ExecuteRequest{
			Provider:    providers.IDCursor,
			AttemptID:   "attempt-cancel-prompt-file",
			UserMessage: strings.Repeat("x", cursor.CursorWindowsPromptArgumentLimit+1),
		}, func([]byte) error { return nil })
		outcome <- err
	}()
	<-runner.started
	cancel()

	select {
	case err := <-outcome:
		if !errors.Is(err, providers.ErrExecuteCancelled) {
			t.Fatalf("Execute() error = %v, want cancellation failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute() did not stop after cancellation")
	}
	if files.file.closes != 1 || files.removes != 1 {
		t.Fatalf(
			"prompt file cleanup = (closes %d, removes %d), want once",
			files.file.closes, files.removes,
		)
	}
}

func cursorFailureRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:     providers.IDCursor,
		AttemptID:    "attempt-cursor-failure",
		SystemPrompt: cursorFailureSecret,
		UserMessage:  "run failure fixture",
	}
}

func assertCursorFailure(
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
	if strings.Contains(err.Error(), cursorFailureSecret) {
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

type blockingCommandRunner struct {
	started chan struct{}
}

func (r *blockingCommandRunner) Run(
	ctx context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	close(r.started)
	<-ctx.Done()
	return workers.CommandResult{}, ctx.Err()
}

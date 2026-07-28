package claude_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
)

const claudeFailureSecret = "prompt-secret-that-must-not-escape"

func TestClaudeRootNormalizesFailureStagesAndSuppressesResults(t *testing.T) {
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
			effectErr: errors.New("exit included " + claudeFailureSecret),
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "native",
		},
		{
			name:      "malformed stream-json",
			stream:    `{"type":"system","subtype":"init","secret":"` + claudeFailureSecret,
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "decode",
		},
		{
			name:      "incomplete final state",
			stream:    "{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"claude-session-partial\"}\n",
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "final_parse",
		},
		{
			name: "declared throttling beats native exit",
			stream: "{\"type\":\"result\",\"subtype\":\"rate_limit_error\",\"is_error\":true,\"result\":\"unexpected status 429 " +
				claudeFailureSecret + "\",\"session_id\":\"\"}\n",
			effectErr: errors.New("exit included " + claudeFailureSecret),
			wantKind:  providers.ExecuteFailureKindThrottled,
		},
		{
			name: "recognized native failure beats unknown stream failure",
			stream: "{\"type\":\"result\",\"subtype\":\"error\",\"is_error\":true,\"result\":\"" +
				claudeFailureSecret + "\",\"session_id\":\"\"}\n",
			effectErr: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindAuthentication,
			},
			wantKind: providers.ExecuteFailureKindAuthentication,
		},
		{
			name: "invalid tool input does not leak with incomplete final",
			stream: "{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"claude-session-tool\"}\n" +
				"{\"type\":\"stream_event\",\"session_id\":\"claude-session-tool\",\"event\":{\"type\":\"message_start\",\"message\":{\"id\":\"msg_tool\",\"role\":\"assistant\",\"content\":[]}}}\n" +
				"{\"type\":\"stream_event\",\"session_id\":\"claude-session-tool\",\"event\":{\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_bad\",\"name\":\"lookup\",\"input\":{}}}}\n" +
				"{\"type\":\"stream_event\",\"session_id\":\"claude-session-tool\",\"event\":{\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{invalid" +
				claudeFailureSecret + "}\"}}}\n",
			wantKind:  providers.ExecuteFailureKindUnknown,
			wantStage: "final_parse",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var cleanups atomic.Int32
			effect := claude.EffectFunc(func(
				_ context.Context,
				_ providers.ExecuteRequest,
				observe func([]byte) error,
			) (claude.EffectResult, error) {
				defer cleanups.Add(1)
				if test.stream != "" {
					if err := observe([]byte(test.stream)); err != nil {
						return claude.EffectResult{}, err
					}
				}
				return claude.EffectResult{}, test.effectErr
			})

			for iteration := 0; iteration < 10; iteration++ {
				result, err := newClaudeRoot(t, effect).Execute(
					t.Context(),
					claudeFailureRequest(),
				)
				assertClaudeFailure(t, result, err, test.wantKind, test.wantStage)
			}
			if got := cleanups.Load(); got != 10 {
				t.Fatalf("cleanup calls = %d, want 10", got)
			}
		})
	}
}

func TestClaudeRootCancellationAndDeadlineReachEffectAndCleanUpOnce(t *testing.T) {
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
			effect := claude.EffectFunc(func(
				ctx context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (claude.EffectResult, error) {
				close(started)
				defer cleanups.Add(1)
				<-ctx.Done()
				return claude.EffectResult{}, ctx.Err()
			})
			ctx, cancel := test.newContext()
			defer cancel()
			root := newClaudeRoot(t, effect)
			outcome := make(chan struct {
				result providers.ExecuteResult
				err    error
			}, 1)
			go func() {
				result, err := root.Execute(ctx, claudeFailureRequest())
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
				assertClaudeFailure(t, got.result, got.err, test.wantKind, "")
			case <-time.After(time.Second):
				t.Fatal("Execute() did not stop after context termination")
			}
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("cleanup calls = %d, want 1", got)
			}
		})
	}
}

func TestClaudeRootFailureSuppressesPreviouslyObservedSuccess(t *testing.T) {
	t.Parallel()

	effect := claude.EffectFunc(func(
		_ context.Context,
		_ providers.ExecuteRequest,
		observe func([]byte) error,
	) (claude.EffectResult, error) {
		if err := observe(claudeSuccessStream()); err != nil {
			return claude.EffectResult{}, err
		}
		return claude.EffectResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindAuthentication,
			Message: "credential " + claudeFailureSecret,
		}
	})
	result, err := newClaudeRoot(t, effect).Execute(t.Context(), claudeFailureRequest())
	assertClaudeFailure(
		t,
		result,
		err,
		providers.ExecuteFailureKindAuthentication,
		"",
	)
}

func claudeFailureRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:     providers.IDClaude,
		AttemptID:    "attempt-claude-failure",
		SystemPrompt: claudeFailureSecret,
		UserMessage:  "run failure fixture",
	}
}

func assertClaudeFailure(
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
	if strings.Contains(err.Error(), claudeFailureSecret) {
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

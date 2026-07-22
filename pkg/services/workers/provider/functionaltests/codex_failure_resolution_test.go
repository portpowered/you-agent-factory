package providers

import (
	"context"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

func TestResolveCodexProviderFailure_PrecedenceTable(t *testing.T) {
	testCases := []struct {
		name   string
		result provider.CommandResult
		input  provider.CodexFailureResolutionInput
		want   provider.ProviderFailureResult
	}{
		{
			name: "structured_stream_wins_over_stderr_and_exit",
			result: provider.CommandResult{
				ExitCode: 1,
				Stdout: []byte(strings.Join([]string{
					`{"type":"thread.started","thread_id":"thread-1"}`,
					`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
				}, "\n") + "\n"),
				Stderr: []byte("ERROR: unexpected status 401\n"),
			},
			want: provider.ProviderFailureResult{
				Reason:  workerexecution.WorkFailureTypeThrottled,
				Message: codexThrottleFailureMessage,
			},
		},
		{
			name: "stderr_wins_when_structured_stream_unrecognized",
			result: provider.CommandResult{
				ExitCode: 1,
				Stdout:   []byte(`{"type":"error","message":"cleanup detail that must not win"}` + "\n"),
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			want: provider.ProviderFailureResult{
				Reason:  workerexecution.WorkFailureTypeAuthFailure,
				Message: codexAuthFailureMessage,
			},
		},
		{
			name: "exit_fallback_when_only_noise",
			result: provider.CommandResult{
				ExitCode: 17,
				Stdout:   []byte("ordinary transcript output\n"),
				Stderr:   []byte("cleanup finished\n"),
			},
			want: provider.ProviderFailureResult{
				Reason:  workerexecution.WorkFailureTypeUnknown,
				Message: codexUnknownFailureMessage,
			},
		},
		{
			name: "timeout_wins_over_structured_stderr_and_exit",
			result: provider.CommandResult{
				ExitCode: 124,
				Stdout: []byte(strings.Join([]string{
					`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
				}, "\n") + "\n"),
				Stderr: []byte("ERROR: unexpected status 401\n"),
			},
			input: provider.CodexFailureResolutionInput{
				CommandError: context.DeadlineExceeded,
			},
			want: provider.ProviderFailureResult{
				Reason:  workerexecution.WorkFailureTypeTimeout,
				Message: "Codex execution timed out.",
			},
		},
		{
			name: "cancel_wins_over_structured_stderr_and_exit",
			result: provider.CommandResult{
				ExitCode: 1,
				Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n"),
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			input: provider.CodexFailureResolutionInput{
				FlushReason: provider.CodexFlushReasonCanceled,
			},
			want: provider.ProviderFailureResult{
				Reason:  workerexecution.WorkFailureTypeUnknown,
				Message: "Codex execution was canceled.",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := provider.ResolveCodexProviderFailure(tc.result, tc.input)
			if !ok {
				t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
			}
			if got.Result != tc.want {
				t.Fatalf("ResolveCodexProviderFailure() = %#v, want %#v", got.Result, tc.want)
			}
		})
	}
}

func TestParseCodexProviderFailure_MatchesResolveCodexProviderFailure(t *testing.T) {
	result := provider.CommandResult{
		ExitCode: 1,
		Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 503"}}` + "\n"),
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	}
	parsed := provider.ParseCodexProviderFailure(result)
	resolved, ok := provider.ResolveCodexProviderFailure(result, provider.CodexFailureResolutionInput{})
	if !ok {
		t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
	}
	if parsed != resolved.Result {
		t.Fatalf("ParseCodexProviderFailure() = %#v, ResolveCodexProviderFailure() = %#v, want matching outcomes", parsed, resolved.Result)
	}
}

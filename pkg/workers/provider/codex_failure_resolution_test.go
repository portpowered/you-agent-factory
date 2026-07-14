package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestResolveCodexProviderFailure_PrecedenceTable(t *testing.T) {
	testCases := []struct {
		name   string
		result CommandResult
		input  CodexFailureResolutionInput
		want   ProviderFailureResult
	}{
		{
			name: "structured_stream_wins_over_stderr_and_exit",
			result: CommandResult{
				ExitCode: 1,
				Stdout: []byte(strings.Join([]string{
					`{"type":"thread.started","thread_id":"thread-1"}`,
					`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
				}, "\n") + "\n"),
				Stderr: []byte("ERROR: unexpected status 401\n"),
			},
			want: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeThrottled,
				Message: codexThrottleFailureMessage,
			},
		},
		{
			name: "stderr_wins_when_structured_stream_unrecognized",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte(`{"type":"error","message":"cleanup detail that must not win"}` + "\n"),
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			want: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeAuthFailure,
				Message: codexAuthFailureMessage,
			},
		},
		{
			name: "exit_fallback_when_only_noise",
			result: CommandResult{
				ExitCode: 17,
				Stdout:   []byte("ordinary transcript output\n"),
				Stderr:   []byte("cleanup finished\n"),
			},
			want: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeUnknown,
				Message: codexUnknownFailureMessage,
			},
		},
		{
			name: "timeout_wins_over_structured_stderr_and_exit",
			result: CommandResult{
				ExitCode: 124,
				Stdout: []byte(strings.Join([]string{
					`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
				}, "\n") + "\n"),
				Stderr: []byte("ERROR: unexpected status 401\n"),
			},
			input: CodexFailureResolutionInput{
				CommandError: context.DeadlineExceeded,
			},
			want: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeTimeout,
				Message: "Codex execution timed out.",
			},
		},
		{
			name: "cancel_wins_over_structured_stderr_and_exit",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n"),
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			input: CodexFailureResolutionInput{
				FlushReason: CodexFlushReasonCanceled,
			},
			want: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeUnknown,
				Message: "Codex execution was canceled.",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ResolveCodexProviderFailure(tc.result, tc.input)
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
	result := CommandResult{
		ExitCode: 1,
		Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 503"}}` + "\n"),
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	}
	parsed := ParseCodexProviderFailure(result)
	resolved, ok := ResolveCodexProviderFailure(result, CodexFailureResolutionInput{})
	if !ok {
		t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
	}
	if parsed != resolved.Result {
		t.Fatalf("ParseCodexProviderFailure() = %#v, ResolveCodexProviderFailure() = %#v, want matching outcomes", parsed, resolved.Result)
	}
}

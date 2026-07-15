package exitfailure_test

import (
	"context"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/workers/provider/codex/exitfailure"
)

func codexStructuredStreamStdout(message string) []byte {
	return []byte(`{"type":"turn.failed","error":{"message":"` + message + `"}}` + "\n")
}

func TestResolveFailure_PrecedenceTable(t *testing.T) {
	testCases := []struct {
		name   string
		input  exitfailure.ExitFailureInput
		res    exitfailure.ResolutionInput
		want   exitfailure.ExitFailureResult
		wantOK bool
	}{
		{
			name: "structured_stream_wins_over_stderr_and_exit",
			input: exitfailure.ExitFailureInput{
				ExitCode: 1,
				Stdout: []byte(strings.Join([]string{
					`{"type":"thread.started","thread_id":"thread-1"}`,
					`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
				}, "\n") + "\n"),
				Stderr: []byte("ERROR: unexpected status 401\n"),
			},
			want: exitfailure.ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeThrottled,
				Message: codexThrottleFailureMessage,
			},
			wantOK: true,
		},
		{
			name: "stderr_wins_when_structured_stream_unrecognized",
			input: exitfailure.ExitFailureInput{
				ExitCode: 1,
				Stdout:   []byte(`{"type":"error","message":"cleanup detail that must not win"}` + "\n"),
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			want: exitfailure.ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeAuthFailure,
				Message: codexAuthFailureMessage,
			},
			wantOK: true,
		},
		{
			name: "exit_fallback_when_only_noise",
			input: exitfailure.ExitFailureInput{
				ExitCode: 17,
				Stdout:   []byte("ordinary transcript output\n"),
				Stderr:   []byte("cleanup finished\n"),
			},
			want: exitfailure.ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeUnknown,
				Message: exitfailure.UnknownFailureMessage,
			},
			wantOK: true,
		},
		{
			name: "timeout_wins_over_structured_stderr_and_exit",
			input: exitfailure.ExitFailureInput{
				ExitCode: 124,
				Stdout: []byte(strings.Join([]string{
					`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
				}, "\n") + "\n"),
				Stderr: []byte("ERROR: unexpected status 401\n"),
			},
			res: exitfailure.ResolutionInput{CommandError: context.DeadlineExceeded},
			want: exitfailure.ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeTimeout,
				Message: "Codex execution timed out.",
			},
			wantOK: true,
		},
		{
			name: "cancel_wins_over_structured_stderr_and_exit",
			input: exitfailure.ExitFailureInput{
				ExitCode: 1,
				Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n"),
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			res: exitfailure.ResolutionInput{FlushReason: exitfailure.FlushReasonCanceled},
			want: exitfailure.ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeUnknown,
				Message: "Codex execution was canceled.",
			},
			wantOK: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := exitfailure.ResolveFailure(tc.input, tc.res)
			if ok != tc.wantOK {
				t.Fatalf("ResolveFailure() ok = %v, want %v", ok, tc.wantOK)
			}
			if got.Result != tc.want {
				t.Fatalf("ResolveFailure() = %#v, want %#v", got.Result, tc.want)
			}
		})
	}
}

func TestResolveFailure_PreservesBoundedInternalCause(t *testing.T) {
	input := exitfailure.ExitFailureInput{
		ExitCode: 1,
		Stdout:   codexStructuredStreamStdout("unexpected status 401"),
		Stderr:   []byte("ERROR: unexpected status 429\n"),
	}
	got, ok := exitfailure.ResolveFailure(input, exitfailure.ResolutionInput{})
	if !ok {
		t.Fatal("ResolveFailure() ok = false, want true")
	}
	if got.Result.Reason != workerexecution.WorkFailureTypeAuthFailure {
		t.Fatalf("Reason = %q, want auth failure", got.Result.Reason)
	}
	if got.InternalCause == "" {
		t.Fatal("expected bounded internal cause on structured stream failure")
	}
}

func TestStructuredStreamReportingOutcome_ClassifiesTerminalJSONL(t *testing.T) {
	got, ok := exitfailure.StructuredStreamReportingOutcome(codexStructuredStreamStdout("unexpected status 429"))
	if !ok {
		t.Fatal("StructuredStreamReportingOutcome() ok = false, want true")
	}
	if got.Reason != workerexecution.WorkFailureTypeThrottled || got.Message != codexThrottleFailureMessage {
		t.Fatalf("StructuredStreamReportingOutcome() = %#v, want throttle failure", got)
	}
}

func TestProcessExitReportingOutcome_ClassifiesStderrWithoutStdout(t *testing.T) {
	got := exitfailure.ProcessExitReportingOutcome(exitfailure.ExitFailureInput{
		ExitCode: 1,
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	})
	if got.Reason != workerexecution.WorkFailureTypeAuthFailure || got.Message != codexAuthFailureMessage {
		t.Fatalf("ProcessExitReportingOutcome() = %#v, want auth failure", got)
	}
}

func TestExitInternalCause_ReturnsBoundedExitLabel(t *testing.T) {
	if got := exitfailure.ExitInternalCause(17); got != "exit code 17" {
		t.Fatalf("ExitInternalCause() = %q, want exit code label", got)
	}
}

func TestResolveFailure_ContextCanceledSetsInternalCause(t *testing.T) {
	got, ok := exitfailure.ResolveFailure(exitfailure.ExitFailureInput{ExitCode: 1}, exitfailure.ResolutionInput{
		CommandError: context.Canceled,
	})
	if !ok {
		t.Fatal("ResolveFailure() ok = false, want true")
	}
	if got.InternalCause != "execution canceled" {
		t.Fatalf("InternalCause = %q, want execution canceled", got.InternalCause)
	}
}

func TestStructuredStreamReportingOutcome_ClassifiesErrorRecordType(t *testing.T) {
	stdout := []byte(`{"type":"error","message":"unexpected status 503"}` + "\n")
	got, ok := exitfailure.StructuredStreamReportingOutcome(stdout)
	if !ok {
		t.Fatal("StructuredStreamReportingOutcome() ok = false, want true")
	}
	if got.Reason != workerexecution.WorkFailureTypeInternalServerError || got.Message != codexServerFailureMessage {
		t.Fatalf("StructuredStreamReportingOutcome() = %#v, want server failure", got)
	}
}

func TestResolveFailure_ExitCode124WithoutContextDeadlineUsesExitCause(t *testing.T) {
	got, ok := exitfailure.ResolveFailure(exitfailure.ExitFailureInput{
		ExitCode: 124,
		Stderr:   []byte("ERROR: command timed out\n"),
	}, exitfailure.ResolutionInput{})
	if !ok {
		t.Fatal("ResolveFailure() ok = false, want true")
	}
	if got.Result.Reason != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("Reason = %q, want timeout", got.Result.Reason)
	}
	if got.InternalCause != "exit code 124" {
		t.Fatalf("InternalCause = %q, want exit code 124", got.InternalCause)
	}
}

func TestResolveFailure_StructuredJSONInternalCauseFromProcessExit(t *testing.T) {
	got, ok := exitfailure.ResolveFailure(exitfailure.ExitFailureInput{
		ExitCode: 1,
		Stderr:   []byte(`ERROR: {"type":"error","status":401,"error":{"type":"authentication_error","message":"sign in again"}}`),
	}, exitfailure.ResolutionInput{})
	if !ok {
		t.Fatal("ResolveFailure() ok = false, want true")
	}
	if got.Result.Reason != workerexecution.WorkFailureTypeAuthFailure {
		t.Fatalf("Reason = %q, want auth failure", got.Result.Reason)
	}
	if got.InternalCause == "" {
		t.Fatal("expected structured JSON internal cause from process exit layer")
	}
}

package providers

import (
	"context"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
)

type codexFailureReportingFacts struct {
	Type          workerexecution.WorkFailureType
	Family        workerexecution.WorkFailureFamily
	Message       string
	Retryable     bool
	Terminal      bool
	ThrottlePause bool
}

func codexFailureReportingFactsFromResult(result provider.ProviderFailureResult) codexFailureReportingFacts {
	providerErr := provider.NewProviderErrorFromResult(result, nil)
	decision := provider.WorkFailureDecisionFromProviderError(providerErr)
	return codexFailureReportingFacts{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		ThrottlePause: decision.TriggersThrottlePause,
	}
}

func assertCodexFailureReportingFactsEqual(t *testing.T, path string, got, want codexFailureReportingFacts) {
	t.Helper()
	if got != want {
		t.Fatalf("%s facts = %#v, want %#v", path, got, want)
	}
}

type codexFailureReportingAgreementCase struct {
	name          string
	streamMessage string
	exitStderr    string
	exitCode      int
	want          codexFailureReportingFacts
}

func codexFailureReportingAgreementCases() []codexFailureReportingAgreementCase {
	return []codexFailureReportingAgreementCase{
		{
			name: "auth", streamMessage: "unexpected status 401", exitStderr: "ERROR: unexpected status 401\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeAuthFailure, Family: workerexecution.WorkFailureFamilyTerminal,
				Message: codexAuthFailureMessage, Terminal: true,
			},
		},
		{
			name: "invalid_request", streamMessage: "unexpected status 400", exitStderr: "ERROR: unexpected status 400\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypePermanentBadRequest, Family: workerexecution.WorkFailureFamilyTerminal,
				Message: codexBadRequestFailureMessage, Terminal: true,
			},
		},
		{
			name: "throttle", streamMessage: "unexpected status 429", exitStderr: "ERROR: unexpected status 429\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeThrottled, Family: workerexecution.WorkFailureFamilyThrottle,
				Message: codexThrottleFailureMessage, Retryable: true, ThrottlePause: true,
			},
		},
		{
			name: "capacity", streamMessage: "selected model is at capacity", exitStderr: "ERROR: selected model is at capacity\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeThrottled, Family: workerexecution.WorkFailureFamilyThrottle,
				Message: codexThrottleFailureMessage, Retryable: true, ThrottlePause: true,
			},
		},
		{
			name: "usage_limit", streamMessage: "you've hit your usage limit", exitStderr: "ERROR: you've hit your usage limit\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeThrottled, Family: workerexecution.WorkFailureFamilyThrottle,
				Message: codexThrottleFailureMessage, Retryable: true, ThrottlePause: true,
			},
		},
		{
			name: "timeout", streamMessage: "command timed out", exitStderr: "ERROR: command timed out\n", exitCode: 124,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeTimeout, Family: workerexecution.WorkFailureFamilyRetryable,
				Message: codexTimeoutFailureMessage, Retryable: true,
			},
		},
		{
			name: "disconnect", streamMessage: "unexpected status 502", exitStderr: "ERROR: unexpected status 502\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeInternalServerError, Family: workerexecution.WorkFailureFamilyRetryable,
				Message: codexServerFailureMessage, Retryable: true,
			},
		},
		{
			name: "server", streamMessage: "unexpected status 503", exitStderr: "ERROR: unexpected status 503\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeInternalServerError, Family: workerexecution.WorkFailureFamilyRetryable,
				Message: codexServerFailureMessage, Retryable: true,
			},
		},
		{
			name: "malformed", streamMessage: "operation failed with private transcript details",
			exitStderr: `ERROR: {"type":"error","error":{"message":"private transcript"}}` + "\n", exitCode: 1,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeUnknown, Family: workerexecution.WorkFailureFamilyTerminal,
				Message: codexUnknownFailureMessage, Terminal: true,
			},
		},
		{
			name: "unknown", streamMessage: "cleanup detail that is not a recognized provider failure", exitStderr: "", exitCode: 17,
			want: codexFailureReportingFacts{
				Type: workerexecution.WorkFailureTypeUnknown, Family: workerexecution.WorkFailureFamilyTerminal,
				Message: codexUnknownFailureMessage, Terminal: true,
			},
		},
	}
}

func runCodexFailureReportingAgreementCase(t *testing.T, tc codexFailureReportingAgreementCase) {
	t.Helper()
	streamResult, ok := provider.CodexStructuredStreamReportingOutcome(codexStructuredStreamStdout(tc.streamMessage))
	if !ok {
		t.Fatal("CodexStructuredStreamReportingOutcome() ok = false, want true")
	}
	exitResult := provider.CodexProcessExitReportingOutcome(codexProcessExitResult(tc.exitStderr, tc.exitCode))

	streamFacts := codexFailureReportingFactsFromResult(streamResult)
	exitFacts := codexFailureReportingFactsFromResult(exitResult)
	assertCodexFailureReportingFactsEqual(t, "structured-stream", streamFacts, tc.want)
	assertCodexFailureReportingFactsEqual(t, "process-exit", exitFacts, tc.want)
	if streamFacts != exitFacts {
		t.Fatalf("structured-stream facts = %#v, process-exit facts = %#v, want matching outcomes", streamFacts, exitFacts)
	}
	if strings.Contains(streamFacts.Message, tc.streamMessage) {
		t.Fatalf("structured-stream message leaked native text: %q", streamFacts.Message)
	}
}

func TestCodexFailureReportingPaths_AgreeOnListedFailureClasses(t *testing.T) {
	for _, tc := range codexFailureReportingAgreementCases() {
		t.Run(tc.name, func(t *testing.T) {
			runCodexFailureReportingAgreementCase(t, tc)
		})
	}
}

func TestCodexFailureReportingPaths_CanceledAgreesAcrossCompetingSignals(t *testing.T) {
	competing := []struct {
		name   string
		result provider.CommandResult
	}{
		{
			name: "structured_stream_competition",
			result: provider.CommandResult{
				ExitCode: 1,
				Stdout:   codexStructuredStreamStdout("unexpected status 429"),
			},
		},
		{
			name: "process_exit_competition",
			result: provider.CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
		},
	}

	want := codexFailureReportingFacts{
		Type: workerexecution.WorkFailureTypeUnknown, Family: workerexecution.WorkFailureFamilyTerminal,
		Message: "Codex execution was canceled.", Terminal: true,
	}
	input := provider.CodexFailureResolutionInput{FlushReason: provider.CodexFlushReasonCanceled}

	for _, tc := range competing {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := provider.ResolveCodexProviderFailure(tc.result, input)
			if !ok {
				t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
			}
			got := codexFailureReportingFactsFromResult(resolved.Result)
			if got != want {
				t.Fatalf("ResolveCodexProviderFailure() facts = %#v, want %#v", got, want)
			}
		})
	}
}

func TestCodexFailureReportingPaths_TimeoutAgreesAcrossCompetingSignals(t *testing.T) {
	competing := []struct {
		name   string
		result provider.CommandResult
		input  provider.CodexFailureResolutionInput
	}{
		{
			name: "structured_stream_competition",
			result: provider.CommandResult{
				ExitCode: 1,
				Stdout:   codexStructuredStreamStdout("unexpected status 429"),
			},
			input: provider.CodexFailureResolutionInput{CommandError: context.DeadlineExceeded},
		},
		{
			name: "process_exit_competition",
			result: provider.CommandResult{
				ExitCode: 124,
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			input: provider.CodexFailureResolutionInput{CommandError: context.DeadlineExceeded},
		},
	}

	want := codexFailureReportingFacts{
		Type: workerexecution.WorkFailureTypeTimeout, Family: workerexecution.WorkFailureFamilyRetryable,
		Message: "Codex execution timed out.", Retryable: true,
	}

	for _, tc := range competing {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := provider.ResolveCodexProviderFailure(tc.result, tc.input)
			if !ok {
				t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
			}
			got := codexFailureReportingFactsFromResult(resolved.Result)
			if got != want {
				t.Fatalf("ResolveCodexProviderFailure() facts = %#v, want %#v", got, want)
			}
		})
	}
}

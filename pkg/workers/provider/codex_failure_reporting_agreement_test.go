package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type codexFailureReportingFacts struct {
	Type            interfaces.WorkFailureType
	Family          interfaces.WorkFailureFamily
	Message         string
	Retryable       bool
	Terminal        bool
	ThrottlePause   bool
}

func codexFailureReportingFactsFromResult(result ProviderFailureResult) codexFailureReportingFacts {
	providerErr := NewProviderErrorFromResult(result, nil)
	decision := WorkFailureDecisionFromProviderError(providerErr)
	return codexFailureReportingFacts{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		ThrottlePause: decision.TriggersThrottlePause,
	}
}

func assertCodexFailureReportingFactsEqual(t *testing.T, structuredPath, processExitPath string, got, want codexFailureReportingFacts) {
	t.Helper()
	if got != want {
		t.Fatalf("%s facts = %#v, %s facts = %#v, want %#v", structuredPath, got, processExitPath, got, want)
	}
}

func codexStructuredStreamStdout(message string) []byte {
	record, err := json.Marshal(map[string]any{
		"type":  "turn.failed",
		"error": map[string]string{"message": message},
	})
	if err != nil {
		panic(err)
	}
	return append(record, '\n')
}

func codexProcessExitResult(stderr string, exitCode int) CommandResult {
	return CommandResult{ExitCode: exitCode, Stderr: []byte(stderr)}
}

func TestCodexFailureReportingPaths_AgreeOnListedFailureClasses(t *testing.T) {
	testCases := []struct {
		name            string
		streamMessage   string
		exitStderr      string
		exitCode        int
		want            codexFailureReportingFacts
		resolutionInput CodexFailureResolutionInput
	}{
		{
			name:          "auth",
			streamMessage: "unexpected status 401",
			exitStderr:    "ERROR: unexpected status 401\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeAuthFailure, Family: interfaces.WorkFailureFamilyTerminal,
				Message: codexAuthFailureMessage, Terminal: true,
			},
		},
		{
			name:          "invalid_request",
			streamMessage: "unexpected status 400",
			exitStderr:    "ERROR: unexpected status 400\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypePermanentBadRequest, Family: interfaces.WorkFailureFamilyTerminal,
				Message: codexBadRequestFailureMessage, Terminal: true,
			},
		},
		{
			name:          "throttle",
			streamMessage: "unexpected status 429",
			exitStderr:    "ERROR: unexpected status 429\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeThrottled, Family: interfaces.WorkFailureFamilyThrottle,
				Message: codexThrottleFailureMessage, Retryable: true, ThrottlePause: true,
			},
		},
		{
			name:          "capacity",
			streamMessage: "selected model is at capacity",
			exitStderr:    "ERROR: selected model is at capacity\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeThrottled, Family: interfaces.WorkFailureFamilyThrottle,
				Message: codexThrottleFailureMessage, Retryable: true, ThrottlePause: true,
			},
		},
		{
			name:          "usage_limit",
			streamMessage: "you've hit your usage limit",
			exitStderr:    "ERROR: you've hit your usage limit\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeThrottled, Family: interfaces.WorkFailureFamilyThrottle,
				Message: codexThrottleFailureMessage, Retryable: true, ThrottlePause: true,
			},
		},
		{
			name:          "timeout",
			streamMessage: "command timed out",
			exitStderr:    "ERROR: command timed out\n",
			exitCode:      124,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeTimeout, Family: interfaces.WorkFailureFamilyRetryable,
				Message: codexTimeoutFailureMessage, Retryable: true,
			},
		},
		{
			name:          "disconnect",
			streamMessage: "unexpected status 502",
			exitStderr:    "ERROR: unexpected status 502\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeInternalServerError, Family: interfaces.WorkFailureFamilyRetryable,
				Message: codexServerFailureMessage, Retryable: true,
			},
		},
		{
			name:          "server",
			streamMessage: "unexpected status 503",
			exitStderr:    "ERROR: unexpected status 503\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeInternalServerError, Family: interfaces.WorkFailureFamilyRetryable,
				Message: codexServerFailureMessage, Retryable: true,
			},
		},
		{
			name:          "malformed",
			streamMessage: "operation failed with private transcript details",
			exitStderr:    `ERROR: {"type":"error","error":{"message":"private transcript"}}` + "\n",
			exitCode:      1,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeUnknown, Family: interfaces.WorkFailureFamilyTerminal,
				Message: codexUnknownFailureMessage, Terminal: true,
			},
		},
		{
			name:          "unknown",
			streamMessage: "cleanup detail that is not a recognized provider failure",
			exitStderr:    "",
			exitCode:      17,
			want: codexFailureReportingFacts{
				Type: interfaces.WorkFailureTypeUnknown, Family: interfaces.WorkFailureFamilyTerminal,
				Message: codexUnknownFailureMessage, Terminal: true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			streamResult, ok := CodexStructuredStreamReportingOutcome(codexStructuredStreamStdout(tc.streamMessage))
			if !ok {
				t.Fatal("CodexStructuredStreamReportingOutcome() ok = false, want true")
			}
			exitResult := CodexProcessExitReportingOutcome(codexProcessExitResult(tc.exitStderr, tc.exitCode))

			streamFacts := codexFailureReportingFactsFromResult(streamResult)
			exitFacts := codexFailureReportingFactsFromResult(exitResult)
			assertCodexFailureReportingFactsEqual(t, "structured-stream", "process-exit", streamFacts, tc.want)
			assertCodexFailureReportingFactsEqual(t, "process-exit", "structured-stream", exitFacts, tc.want)
			if streamFacts != exitFacts {
				t.Fatalf("structured-stream facts = %#v, process-exit facts = %#v, want matching outcomes", streamFacts, exitFacts)
			}
			if strings.Contains(streamFacts.Message, tc.streamMessage) {
				t.Fatalf("structured-stream message leaked native text: %q", streamFacts.Message)
			}
		})
	}
}

func TestCodexFailureReportingPaths_CanceledAgreesAcrossCompetingSignals(t *testing.T) {
	competing := []struct {
		name   string
		result CommandResult
	}{
		{
			name: "structured_stream_competition",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   codexStructuredStreamStdout("unexpected status 429"),
			},
		},
		{
			name: "process_exit_competition",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
		},
	}

	want := codexFailureReportingFacts{
		Type: interfaces.WorkFailureTypeUnknown, Family: interfaces.WorkFailureFamilyTerminal,
		Message: "Codex execution was canceled.", Terminal: true,
	}
	input := CodexFailureResolutionInput{FlushReason: CodexFlushReasonCanceled}

	for _, tc := range competing {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := ResolveCodexProviderFailure(tc.result, input)
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
		result CommandResult
		input  CodexFailureResolutionInput
	}{
		{
			name: "structured_stream_competition",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   codexStructuredStreamStdout("unexpected status 429"),
			},
			input: CodexFailureResolutionInput{CommandError: context.DeadlineExceeded},
		},
		{
			name: "process_exit_competition",
			result: CommandResult{
				ExitCode: 124,
				Stderr:   []byte("ERROR: unexpected status 401\n"),
			},
			input: CodexFailureResolutionInput{CommandError: context.DeadlineExceeded},
		},
	}

	want := codexFailureReportingFacts{
		Type: interfaces.WorkFailureTypeTimeout, Family: interfaces.WorkFailureFamilyRetryable,
		Message: "Codex execution timed out.", Retryable: true,
	}

	for _, tc := range competing {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := ResolveCodexProviderFailure(tc.result, tc.input)
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

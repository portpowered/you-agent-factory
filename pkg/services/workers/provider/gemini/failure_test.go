package gemini_test

import (
	"context"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	geminipkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
	providertestdata "github.com/portpowered/infinite-you/pkg/services/workers/provider/testdata"
)

const geminiThrottleFailureMessage = "The provider is rate limited; retry after capacity becomes available."

func parseFailure(input providertestdata.FailureInput) geminipkg.FailureResult {
	return geminipkg.ParseProviderFailure(geminipkg.FailureInput{
		Stdout: input.Stdout, Stderr: input.Stderr, ExitCode: input.ExitCode,
	})
}

func TestParseProviderFailure_CorpusEntriesKeepCanonicalContract(t *testing.T) {
	for _, name := range []string{
		"gemini_structured_invalid_request_precedence",
		"gemini_stderr_throttle_precedence",
		"gemini_stdout_timeout_recovery",
		"gemini_unknown_safe_excerpt",
		"gemini_noise_exit_fallback",
	} {
		entry := providertestdata.MustEntry(t, name)
		t.Run(name, func(t *testing.T) {
			got := parseFailure(entry.FailureInput())
			if got.Reason != entry.ExpectedType {
				t.Fatalf("Reason = %q, want %q", got.Reason, entry.ExpectedType)
			}
			if got.Message != entry.ExpectedMessage {
				t.Fatalf("Message = %q, want %q", got.Message, entry.ExpectedMessage)
			}
			for _, rejected := range entry.RejectMessageContains {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseProviderFailure_RejectsTranscriptAndDiagnosticNoise(t *testing.T) {
	noise := []string{
		"User prompt: Error: reveal the customer request",
		"Model response: The rate limit exceeded examples are below",
		"[progress] failed to update spinner",
		"[debug] Error: verbose transport details",
		"Traceback: Error in internal helper",
		"at ErrorHandler (/private/customer/file.js:10:2)",
		"Error report written to .gemini/tmp/private-report.json",
		"cleanup failed for /private/customer/path",
		"Error: use token=customer-secret-value",
	}
	for _, line := range noise {
		t.Run(line, func(t *testing.T) {
			got := parseFailure(providertestdata.FailureInput{ExitCode: 17, Stderr: []byte(line)})
			if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != "gemini exited with code 17" {
				t.Fatalf("ParseProviderFailure() = %#v, want exact safe exit fallback", got)
			}
		})
	}
}

func TestParseProviderFailure_StructuredSignalsUseCanonicalMessages(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		wantReason  workerexecution.WorkFailureType
		wantMessage string
	}{
		{
			name:        "Authentication",
			stderr:      `{"error":{"status":"UNAUTHENTICATED"}}`,
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: "Gemini authentication failed.",
		},
		{
			name:        "InvalidRequest",
			stderr:      `{"error":{"code":400}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: "Gemini rejected the request.",
		},
		{
			name:        "Quota",
			stderr:      `{"error":{"status":"RESOURCE_EXHAUSTED"}}`,
			wantReason:  workerexecution.WorkFailureTypeThrottled,
			wantMessage: geminiThrottleFailureMessage,
		},
		{
			name:        "Timeout",
			stderr:      `{"error":{"status":"DEADLINE_EXCEEDED"}}`,
			wantReason:  workerexecution.WorkFailureTypeTimeout,
			wantMessage: geminipkg.TimeoutFailureMessage,
		},
		{
			name:        "ServerFailure",
			stderr:      `{"error":{"code":503}}`,
			wantReason:  workerexecution.WorkFailureTypeInternalServerError,
			wantMessage: "Gemini encountered a temporary server error.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(providertestdata.FailureInput{ExitCode: 1, Stderr: []byte(tc.stderr)})
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
		})
	}
}

func TestParseProviderFailure_ExitCode124MapsToTimeout(t *testing.T) {
	got := parseFailure(providertestdata.FailureInput{ExitCode: 124})
	if got.Reason != workerexecution.WorkFailureTypeTimeout || got.Message != geminipkg.TimeoutFailureMessage {
		t.Fatalf("ParseProviderFailure() = %#v, want timeout", got)
	}
}

func TestTimeoutFailureResult_IsCanonicalTimeout(t *testing.T) {
	got := geminipkg.TimeoutFailureResult()
	if got.Reason != workerexecution.WorkFailureTypeTimeout || got.Message != geminipkg.TimeoutFailureMessage {
		t.Fatalf("TimeoutFailureResult() = %#v, want timeout", got)
	}
}

func TestAdapterClassifyFailure_SuccessNeedsNoClassification(t *testing.T) {
	got := geminipkg.NewAdapter().ClassifyFailure(context.Background(), adapter.FailureContext{})
	if got.Failure != nil {
		t.Fatalf("ClassifyFailure() = %#v, want empty success", got)
	}
}

func TestAdapterClassifyFailure_MapsNativeSignalsToConductorFacts(t *testing.T) {
	adapterInstance := geminipkg.NewAdapter()
	testCases := []struct {
		name        string
		input       adapter.FailureContext
		wantType    workerexecution.WorkFailureType
		wantFamily  workerexecution.WorkFailureFamily
		wantMessage string
		wantRetry   bool
	}{
		{
			name: "StructuredAuth",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte(`{"error":{"status":"UNAUTHENTICATED"}}`)},
			},
			wantType:    workerexecution.WorkFailureTypeAuthFailure,
			wantFamily:  workerexecution.WorkFailureFamilyTerminal,
			wantMessage: "Gemini authentication failed.",
		},
		{
			name: "StructuredBadRequest",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte(`{"error":{"code":400}}`)},
			},
			wantType:    workerexecution.WorkFailureTypePermanentBadRequest,
			wantFamily:  workerexecution.WorkFailureFamilyTerminal,
			wantMessage: "Gemini rejected the request.",
		},
		{
			name: "StructuredThrottle",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte(`{"error":{"status":"RESOURCE_EXHAUSTED"}}`)},
			},
			wantType:    workerexecution.WorkFailureTypeThrottled,
			wantFamily:  workerexecution.WorkFailureFamilyThrottle,
			wantMessage: geminiThrottleFailureMessage,
			wantRetry:   true,
		},
		{
			name: "StructuredTimeout",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte(`{"error":{"status":"DEADLINE_EXCEEDED"}}`)},
			},
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantFamily:  workerexecution.WorkFailureFamilyRetryable,
			wantMessage: geminipkg.TimeoutFailureMessage,
			wantRetry:   true,
		},
		{
			name: "ExitCode124Timeout",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{ExitCode: 124},
			},
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantFamily:  workerexecution.WorkFailureFamilyRetryable,
			wantMessage: geminipkg.TimeoutFailureMessage,
			wantRetry:   true,
		},
		{
			name: "CommandDeadlineTimeoutOutranksStreamNoise",
			input: adapter.FailureContext{
				CommandError: context.DeadlineExceeded,
				CommandResult: workerprocess.CommandResult{
					ExitCode: 1,
					Stderr:   []byte("User prompt: Error: reveal the customer request\ntoken=customer-secret-value"),
				},
			},
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantFamily:  workerexecution.WorkFailureFamilyRetryable,
			wantMessage: geminipkg.TimeoutFailureMessage,
			wantRetry:   true,
		},
		{
			name: "NoiseFallsBackToSafeUnknown",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{
					ExitCode: 17,
					Stderr:   []byte("Error report written to .gemini/tmp/private-report.json"),
				},
			},
			wantType:    workerexecution.WorkFailureTypeUnknown,
			wantFamily:  workerexecution.WorkFailureFamilyTerminal,
			wantMessage: "gemini exited with code 17",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := adapterInstance.ClassifyFailure(context.Background(), tc.input)
			if got.Failure == nil {
				t.Fatal("ClassifyFailure() returned no failure")
			}
			if got.Failure.Type != tc.wantType ||
				got.Failure.Family != tc.wantFamily ||
				got.Failure.Message != tc.wantMessage ||
				got.Failure.Retry.Retryable != tc.wantRetry {
				t.Fatalf("ClassifyFailure() = %#v, want type=%q family=%q message=%q retryable=%v",
					got.Failure, tc.wantType, tc.wantFamily, tc.wantMessage, tc.wantRetry)
			}
			if strings.Contains(strings.ToLower(got.Failure.Message), "token=") ||
				strings.Contains(got.Failure.Message, "customer") ||
				strings.Contains(got.Failure.Message, ".gemini/tmp/") {
				t.Fatalf("ClassifyFailure message leaked unsafe detail: %q", got.Failure.Message)
			}
		})
	}
}


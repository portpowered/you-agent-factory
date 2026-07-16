package gemini_test

import (
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	geminipkg "github.com/portpowered/infinite-you/pkg/workers/provider/gemini"
	providertestdata "github.com/portpowered/infinite-you/pkg/workers/provider/testdata"
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

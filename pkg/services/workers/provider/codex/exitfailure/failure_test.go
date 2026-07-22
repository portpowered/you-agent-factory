package exitfailure_test

import (
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/codex/exitfailure"
	providertestdata "github.com/portpowered/infinite-you/pkg/services/workers/provider/testdata"
)

const (
	codexAuthFailureMessage       = "Codex authentication failed."
	codexBadRequestFailureMessage = "Codex rejected the request as invalid."
	codexThrottleFailureMessage   = "Codex is temporarily unavailable due to usage or capacity limits."
	codexServerFailureMessage     = "Codex encountered a temporary server error."
	codexFailureMessageBytes      = 1024
	codexErrorLineScanBytes       = 64 * 1024
	codexGPT56SolUpgradeMessage   = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
)

func parseFailure(input providertestdata.FailureInput) exitfailure.ExitFailureResult {
	return exitfailure.ParseExitFailure(exitfailure.ExitFailureInput{
		Stdout: input.Stdout, Stderr: input.Stderr, ExitCode: input.ExitCode,
	})
}

func TestParseExitFailure_GPT56SolReturnsActionableNestedMessage(t *testing.T) {
	entry := providertestdata.MustEntry(t, "codex_gpt_5_6_sol_requires_newer_cli")
	got := parseFailure(entry.FailureInput())
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("Reason = %q, want %q", got.Reason, workerexecution.WorkFailureTypePermanentBadRequest)
	}
	if got.Message != codexGPT56SolUpgradeMessage {
		t.Fatalf("Message = %q, want %q", got.Message, codexGPT56SolUpgradeMessage)
	}
	for _, rejected := range entry.RejectMessageContains {
		if strings.Contains(got.Message, rejected) {
			t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
		}
	}
}

func TestParseExitFailure_StructuredRecordsUseLastValidCrossStreamRecord(t *testing.T) {
	input := providertestdata.FailureInput{
		ExitCode: 1,
		Stderr: []byte(strings.Join([]string{
			`ERROR: {"type":"error","status":400,"error":`,
			`ERROR: {"type":"error","status":401,"error":{"type":"authentication_error","message":"sign in again"}}`,
		}, "\n")),
		Stdout: []byte(strings.Join([]string{
			`ERROR: {"type":"error","status":429,"error":{"type":"rate_limit_error","message":"earlier stdout limit"}}`,
			`ERROR: {"type":"error","status":500,"error":{"type":"server_error","message":"final stdout server failure"}}`,
			`ERROR: {"type":"error","status":400,"error":`,
		}, "\n")),
	}

	got := parseFailure(input)
	if got.Reason != workerexecution.WorkFailureTypeInternalServerError || got.Message != codexServerFailureMessage {
		t.Fatalf("ParseExitFailure() = %#v, want final valid stdout server failure", got)
	}
}

func TestParseExitFailure_StructuredFieldsPrecedeSubstringFallback(t *testing.T) {
	input := providertestdata.FailureInput{
		ExitCode: 1,
		Stderr: []byte(strings.Join([]string{
			`ERROR: transcript said 429 too many requests`,
			`ERROR: {"type":"error","status":400,"error":{"type":"invalid_request_error","message":"choose a supported model"}}`,
			`ERROR: cleanup failed after request`,
		}, "\n")),
	}

	got := parseFailure(input)
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest || got.Message != codexBadRequestFailureMessage {
		t.Fatalf("ParseExitFailure() = %#v, want structured bad request", got)
	}
}

func TestParseExitFailure_UsesOuterStructuredTypeAndMessage(t *testing.T) {
	got := parseFailure(providertestdata.FailureInput{
		ExitCode: 1,
		Stderr:   []byte(`ERROR: {"type":"rate_limit_error","message":"request capacity reached"}`),
	})
	if got.Reason != workerexecution.WorkFailureTypeThrottled || got.Message != codexThrottleFailureMessage {
		t.Fatalf("ParseExitFailure() = %#v, want outer structured throttle failure", got)
	}
}

func TestParseExitFailure_KnownCorpusShapesKeepCanonicalReasons(t *testing.T) {
	for _, name := range []string{
		"codex_status_429_too_many_requests",
		"codex_internal_server_status_500",
		"codex_invalid_request_error",
		"codex_timeout_waiting_for_provider",
		"codex_authentication_unauthorized",
		"codex_windows_exit_code_4294967295",
	} {
		entry := providertestdata.MustEntry(t, name)
		t.Run(name, func(t *testing.T) {
			got := parseFailure(entry.FailureInput())
			if got.Reason != entry.ExpectedType {
				t.Fatalf("Reason = %q, want %q", got.Reason, entry.ExpectedType)
			}
		})
	}
}

func TestParseExitFailure_BoundsAndSanitizesFallbackMessages(t *testing.T) {
	testCases := []struct {
		name        string
		input       providertestdata.FailureInput
		wantReason  workerexecution.WorkFailureType
		wantMessage string
		reject      []string
	}{
		{
			name: "TranscriptAndHeadersUseExitFallback",
			input: providertestdata.FailureInput{ExitCode: 7, Stdout: []byte(strings.Join([]string{
				"OpenAI Codex v0.143.0",
				"model: gpt-5.6-sol",
				"customer prompt must stay private",
				"cleanup complete",
			}, "\n"))},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 7",
			reject:      []string{"customer prompt", "cleanup", "gpt-5.6-sol"},
		},
		{
			name:        "MalformedJSONUsesExitFallback",
			input:       providertestdata.FailureInput{ExitCode: 1, Stderr: []byte(`ERROR: {"type":"error","error":{"message":"private transcript"}`)},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"private transcript", "{"},
		},
		{
			name:        "CredentialsUseExitFallback",
			input:       providertestdata.FailureInput{ExitCode: 1, Stderr: []byte("ERROR: request failed with Authorization: Bearer secret-token")},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"secret-token", "Bearer"},
		},
		{
			name: "OutputBeforeScanTailIsIgnored",
			input: providertestdata.FailureInput{ExitCode: 2, Stderr: []byte(
				"ERROR: should not survive the bounded scan\n" + strings.Repeat("cleanup-padding\n", codexErrorLineScanBytes),
			)},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 2",
			reject:      []string{"should not survive"},
		},
		{
			name:        "UnknownErrorExcerptUsesExitFallback",
			input:       providertestdata.FailureInput{ExitCode: 3, Stderr: []byte("ERROR: operation failed " + strings.Repeat("x", codexFailureMessageBytes+200))},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 3",
			reject:      []string{"operation failed", "xxx"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(tc.input)
			if got.Reason != tc.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if tc.wantMessage != "" && got.Message != tc.wantMessage {
				t.Fatalf("Message = %q, want %q", got.Message, tc.wantMessage)
			}
			for _, rejected := range tc.reject {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseExitFailure_UnstructuredFallbackIsSafeByConstruction(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		wantReason  workerexecution.WorkFailureType
		wantMessage string
		reject      []string
	}{
		{
			name: "ErrorPrefixedCleanupCannotDisplaceCapacityFailure",
			stderr: strings.Join([]string{
				"ERROR: Selected model is at capacity. Please try a different model.",
				"ERROR: cleanup failed for /private/customer/path",
			}, "\n"),
			wantReason:  workerexecution.WorkFailureTypeThrottled,
			wantMessage: codexThrottleFailureMessage,
			reject:      []string{"cleanup", "/private/customer/path"},
		},
		{
			name:        "PromptBearingErrorUsesExitFallback",
			stderr:      "ERROR: customer prompt: explain unexpected status 429 with private details",
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"customer prompt", "private details"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(providertestdata.FailureInput{ExitCode: 1, Stderr: []byte(tc.stderr)})
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseExitFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
			for _, rejected := range tc.reject {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseExitFailure_StructuredFallbackIsSafeByConstruction(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		wantReason  workerexecution.WorkFailureType
		wantMessage string
		reject      []string
	}{
		{
			name: "StructuredCleanupCannotDisplaceDecisiveFailure",
			stderr: strings.Join([]string{
				`ERROR: {"type":"error","status":429,"error":{"type":"rate_limit_error","message":"provider capacity reached"}}`,
				`ERROR: {"type":"cleanup_error","status":500,"message":"cleanup failed for /private/customer/path"}`,
			}, "\n"),
			wantReason:  workerexecution.WorkFailureTypeThrottled,
			wantMessage: codexThrottleFailureMessage,
			reject:      []string{"cleanup", "/private/customer/path", "provider capacity reached"},
		},
		{
			name:        "StructuredPromptUsesFixedMessage",
			stderr:      `ERROR: {"status":400,"error":{"type":"invalid_request_error","message":"customer prompt: explain private account details"}}`,
			wantReason:  workerexecution.WorkFailureTypePermanentBadRequest,
			wantMessage: codexBadRequestFailureMessage,
			reject:      []string{"customer prompt", "private account"},
		},
		{
			name:        "StructuredCredentialUsesFixedMessage",
			stderr:      `ERROR: {"status":401,"error":{"type":"authentication_error","message":"secret_token=customer-private-value"}}`,
			wantReason:  workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: codexAuthFailureMessage,
			reject:      []string{"secret_token", "customer-private-value"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailure(providertestdata.FailureInput{ExitCode: 1, Stderr: []byte(tc.stderr)})
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseExitFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
			for _, rejected := range tc.reject {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestExtractErrorLine_ReturnsLastErrorPrefixedLine(t *testing.T) {
	line, ok := exitfailure.ExtractErrorLine(exitfailure.ExitFailureInput{
		Stderr: []byte("planning\nERROR: first\nERROR: decisive"),
	})
	if !ok || line != "ERROR: decisive" {
		t.Fatalf("ExtractErrorLine() = (%q, %v), want decisive error line", line, ok)
	}
}

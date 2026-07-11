package provider

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func loadProviderErrorCorpusForTest(t *testing.T) ProviderErrorCorpus {
	t.Helper()

	corpus, err := LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("LoadProviderErrorCorpus() error = %v", err)
	}
	return corpus
}

func providerErrorCorpusEntryForTest(t *testing.T, name string) ProviderErrorCorpusEntry {
	t.Helper()

	entry, ok := loadProviderErrorCorpusForTest(t).Entry(name)
	if !ok {
		t.Fatalf("provider error corpus entry %q not found", name)
	}
	return entry
}

func providerErrorCorpusEntryLabel(entry ProviderErrorCorpusEntry) string {
	if entry.UpstreamSourceCase == "" {
		return entry.Name
	}
	return entry.Name + " [" + entry.UpstreamSourceCase + "]"
}

func providerErrorCorpusLastErrorLine(t *testing.T, entry ProviderErrorCorpusEntry) string {
	t.Helper()

	var lastMatch string
	for _, stream := range []string{entry.Stderr, entry.Stdout} {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "ERROR:") {
				lastMatch = trimmed
			}
		}
	}
	if lastMatch == "" {
		t.Fatalf("provider error corpus entry %q contains no ERROR: line", providerErrorCorpusEntryLabel(entry))
	}
	return lastMatch
}

func TestNewProviderError_AssignsDeterministicFamilyFromType(t *testing.T) {
	testCases := []struct {
		name       string
		errorType  interfaces.WorkFailureType
		wantFamily interfaces.WorkFailureFamily
	}{
		{name: "AuthFailure_IsTerminal", errorType: interfaces.WorkFailureTypeAuthFailure, wantFamily: interfaces.WorkFailureFamilyTerminal},
		{name: "PermanentBadRequest_IsTerminal", errorType: interfaces.WorkFailureTypePermanentBadRequest, wantFamily: interfaces.WorkFailureFamilyTerminal},
		{name: "Throttled_IsThrottle", errorType: interfaces.WorkFailureTypeThrottled, wantFamily: interfaces.WorkFailureFamilyThrottle},
		{name: "InternalServerError_IsRetryable", errorType: interfaces.WorkFailureTypeInternalServerError, wantFamily: interfaces.WorkFailureFamilyRetryable},
		{name: "Timeout_IsRetryable", errorType: interfaces.WorkFailureTypeTimeout, wantFamily: interfaces.WorkFailureFamilyRetryable},
		{name: "Unknown_IsTerminal", errorType: interfaces.WorkFailureTypeUnknown, wantFamily: interfaces.WorkFailureFamilyTerminal},
		{name: "Misconfigured_IsTerminal", errorType: interfaces.WorkFailureTypeMisconfigured, wantFamily: interfaces.WorkFailureFamilyTerminal},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewProviderError(tc.errorType, "normalized failure", nil)
			if err.Type != tc.errorType {
				t.Fatalf("expected Type %q, got %q", tc.errorType, err.Type)
			}
			if err.Family != tc.wantFamily {
				t.Fatalf("expected Family %q, got %q", tc.wantFamily, err.Family)
			}
		})
	}
}

func TestNewProviderErrorFromResult_DerivesPolicyFromCanonicalReason(t *testing.T) {
	result := ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeThrottled,
		Message: "request capacity exceeded",
	}

	providerErr := NewProviderErrorFromResult(result, nil)
	if providerErr.Type != result.Reason || providerErr.Message != result.Message {
		t.Fatalf("NewProviderErrorFromResult() = %#v, want canonical reason and message", providerErr)
	}
	if providerErr.Family != interfaces.WorkFailureFamilyThrottle {
		t.Fatalf("Family = %q, want %q", providerErr.Family, interfaces.WorkFailureFamilyThrottle)
	}
}

func TestParseCodexProviderFailure_GPT56SolReturnsActionableNestedMessage(t *testing.T) {
	entry := providerErrorCorpusEntryForTest(t, "codex_gpt_5_6_sol_requires_newer_cli")
	wantMessage := codexGPT56SolUpgradeMessage

	got := ParseCodexProviderFailure(entry.CommandResult())
	if got.Reason != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("Reason = %q, want %q", got.Reason, interfaces.WorkFailureTypePermanentBadRequest)
	}
	if got.Message != wantMessage {
		t.Fatalf("Message = %q, want %q", got.Message, wantMessage)
	}
	for _, rejected := range entry.RejectMessageContains {
		if strings.Contains(got.Message, rejected) {
			t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
		}
	}
}

func TestParseCodexProviderFailure_StructuredRecordsUseLastValidCrossStreamRecord(t *testing.T) {
	result := CommandResult{
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

	got := ParseCodexProviderFailure(result)
	if got.Reason != interfaces.WorkFailureTypeInternalServerError || got.Message != codexServerFailureMessage {
		t.Fatalf("ParseCodexProviderFailure() = %#v, want final valid stdout server failure", got)
	}
}

func TestParseCodexProviderFailure_StructuredFieldsPrecedeSubstringFallback(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stderr: []byte(strings.Join([]string{
			`ERROR: transcript said 429 too many requests`,
			`ERROR: {"type":"error","status":400,"error":{"type":"invalid_request_error","message":"choose a supported model"}}`,
			`ERROR: cleanup failed after request`,
		}, "\n")),
	}

	got := ParseCodexProviderFailure(result)
	if got.Reason != interfaces.WorkFailureTypePermanentBadRequest || got.Message != codexBadRequestFailureMessage {
		t.Fatalf("ParseCodexProviderFailure() = %#v, want structured bad request", got)
	}
}

func TestParseCodexProviderFailure_UsesOuterStructuredTypeAndMessage(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`ERROR: {"type":"rate_limit_error","message":"request capacity reached"}`),
	}

	got := ParseCodexProviderFailure(result)
	if got.Reason != interfaces.WorkFailureTypeThrottled || got.Message != codexThrottleFailureMessage {
		t.Fatalf("ParseCodexProviderFailure() = %#v, want outer structured throttle failure", got)
	}
}

func TestParseCodexProviderFailure_KnownCodexShapesKeepCanonicalReasons(t *testing.T) {
	testCases := []string{
		"codex_status_429_too_many_requests",
		"codex_internal_server_status_500",
		"codex_invalid_request_error",
		"codex_timeout_waiting_for_provider",
		"codex_authentication_unauthorized",
		"codex_windows_exit_code_4294967295",
	}
	for _, name := range testCases {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(name, func(t *testing.T) {
			got := ParseCodexProviderFailure(entry.CommandResult())
			if got.Reason != entry.ExpectedType {
				t.Fatalf("Reason = %q, want %q", got.Reason, entry.ExpectedType)
			}
		})
	}
}

func TestParseCodexProviderFailure_BoundsAndSanitizesFallbackMessages(t *testing.T) {
	testCases := []struct {
		name        string
		result      CommandResult
		wantReason  interfaces.WorkFailureType
		wantMessage string
		reject      []string
		wantMaxLen  int
	}{
		{
			name: "TranscriptAndHeadersUseExitFallback",
			result: CommandResult{ExitCode: 7, Stdout: []byte(strings.Join([]string{
				"OpenAI Codex v0.143.0",
				"model: gpt-5.6-sol",
				"customer prompt must stay private",
				"cleanup complete",
			}, "\n"))},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 7",
			reject:      []string{"customer prompt", "cleanup", "gpt-5.6-sol"},
		},
		{
			name:        "MalformedJSONUsesExitFallback",
			result:      CommandResult{ExitCode: 1, Stderr: []byte(`ERROR: {"type":"error","error":{"message":"private transcript"}`)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"private transcript", "{"},
		},
		{
			name:        "CredentialsUseExitFallback",
			result:      CommandResult{ExitCode: 1, Stderr: []byte("ERROR: request failed with Authorization: Bearer secret-token")},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"secret-token", "Bearer"},
		},
		{
			name: "OutputBeforeScanTailIsIgnored",
			result: CommandResult{ExitCode: 2, Stderr: []byte(
				"ERROR: should not survive the bounded scan\n" + strings.Repeat("cleanup-padding\n", codexErrorLineScanBytes),
			)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 2",
			reject:      []string{"should not survive"},
		},
		{
			name:        "UnknownErrorExcerptUsesExitFallback",
			result:      CommandResult{ExitCode: 3, Stderr: []byte("ERROR: operation failed " + strings.Repeat("x", codexFailureMessageBytes+200))},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 3",
			reject:      []string{"operation failed", "xxx"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCodexProviderFailure(tc.result)
			if got.Reason != tc.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if tc.wantMessage != "" && got.Message != tc.wantMessage {
				t.Fatalf("Message = %q, want %q", got.Message, tc.wantMessage)
			}
			if tc.wantMaxLen != 0 && len(got.Message) != tc.wantMaxLen {
				t.Fatalf("message length = %d, want %d", len(got.Message), tc.wantMaxLen)
			}
			for _, rejected := range tc.reject {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseCodexProviderFailure_UnstructuredFallbackIsSafeByConstruction(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		wantReason  interfaces.WorkFailureType
		wantMessage string
		reject      []string
	}{
		{
			name: "ErrorPrefixedCleanupCannotDisplaceCapacityFailure",
			stderr: strings.Join([]string{
				"ERROR: Selected model is at capacity. Please try a different model.",
				"ERROR: cleanup failed for /private/customer/path",
			}, "\n"),
			wantReason:  interfaces.WorkFailureTypeThrottled,
			wantMessage: codexThrottleFailureMessage,
			reject:      []string{"cleanup", "/private/customer/path"},
		},
		{
			name:        "PromptBearingErrorUsesExitFallback",
			stderr:      "ERROR: customer prompt: explain unexpected status 429 with private details",
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"customer prompt", "private details"},
		},
		{
			name:        "TranscriptBearingErrorUsesExitFallback",
			stderr:      "ERROR: transcript: selected model is at capacity in the user's example",
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"transcript", "user's example"},
		},
		{
			name:        "CredentialBearingErrorUsesExitFallback",
			stderr:      "ERROR: arbitrary credential secret_token=customer-private-value",
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "codex exited with code 1",
			reject:      []string{"secret_token", "customer-private-value"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCodexProviderFailure(CommandResult{ExitCode: 1, Stderr: []byte(tc.stderr)})
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseCodexProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
			for _, rejected := range tc.reject {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseCodexProviderFailure_StructuredFallbackIsSafeByConstruction(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		wantReason  interfaces.WorkFailureType
		wantMessage string
		reject      []string
	}{
		{
			name: "StructuredCleanupCannotDisplaceDecisiveFailure",
			stderr: strings.Join([]string{
				`ERROR: {"type":"error","status":429,"error":{"type":"rate_limit_error","message":"provider capacity reached"}}`,
				`ERROR: {"type":"cleanup_error","status":500,"message":"cleanup failed for /private/customer/path"}`,
			}, "\n"),
			wantReason:  interfaces.WorkFailureTypeThrottled,
			wantMessage: codexThrottleFailureMessage,
			reject:      []string{"cleanup", "/private/customer/path", "provider capacity reached"},
		},
		{
			name:        "StructuredPromptUsesFixedMessage",
			stderr:      `ERROR: {"status":400,"error":{"type":"invalid_request_error","message":"customer prompt: explain private account details"}}`,
			wantReason:  interfaces.WorkFailureTypePermanentBadRequest,
			wantMessage: codexBadRequestFailureMessage,
			reject:      []string{"customer prompt", "private account"},
		},
		{
			name:        "StructuredTranscriptUsesFixedMessage",
			stderr:      `ERROR: {"status":500,"error":{"type":"server_error","message":"transcript: user's private response draft"}}`,
			wantReason:  interfaces.WorkFailureTypeInternalServerError,
			wantMessage: codexServerFailureMessage,
			reject:      []string{"transcript", "private response"},
		},
		{
			name:        "StructuredCredentialUsesFixedMessage",
			stderr:      `ERROR: {"status":401,"error":{"type":"authentication_error","message":"secret_token=customer-private-value"}}`,
			wantReason:  interfaces.WorkFailureTypeAuthFailure,
			wantMessage: codexAuthFailureMessage,
			reject:      []string{"secret_token", "customer-private-value"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCodexProviderFailure(CommandResult{ExitCode: 1, Stderr: []byte(tc.stderr)})
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseCodexProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
			for _, rejected := range tc.reject {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseGeminiProviderFailure_NormalizesKnownStructuredFailures(t *testing.T) {
	testCases := []struct {
		name        string
		output      string
		wantReason  interfaces.WorkFailureType
		wantMessage string
	}{
		{
			name:        "AuthenticationTypePreservesActionableMessage",
			output:      `{"type":"error","error":{"type":"FatalAuthenticationError","message":"Run gemini auth login to continue."}}`,
			wantReason:  interfaces.WorkFailureTypeAuthFailure,
			wantMessage: "Run gemini auth login to continue.",
		},
		{
			name:        "PermissionStatusUsesAuthenticationFallback",
			output:      `{"type":"error","error":{"status":"PERMISSION_DENIED"}}`,
			wantReason:  interfaces.WorkFailureTypeAuthFailure,
			wantMessage: geminiAuthFailureMessage,
		},
		{
			name:        "InvalidArgumentPreservesActionableMessage",
			output:      `{"error":{"status":"INVALID_ARGUMENT","message":"The selected model name is invalid."}}`,
			wantReason:  interfaces.WorkFailureTypePermanentBadRequest,
			wantMessage: "The selected model name is invalid.",
		},
		{
			name:        "NumericQuotaCodeUsesFixedMessage",
			output:      `{"error":{"code":429,"message":"quota exhausted for project private-project"}}`,
			wantReason:  interfaces.WorkFailureTypeThrottled,
			wantMessage: geminiThrottleFailureMessage,
		},
		{
			name:        "DeadlineStatusUsesFixedMessage",
			output:      `{"error":{"status":"DEADLINE_EXCEEDED","message":"request exceeded 60 seconds"}}`,
			wantReason:  interfaces.WorkFailureTypeTimeout,
			wantMessage: geminiTimeoutFailureMessage,
		},
		{
			name:        "UnavailableStatusUsesFixedMessage",
			output:      `{"error":{"status":"UNAVAILABLE","message":"backend unavailable"}}`,
			wantReason:  interfaces.WorkFailureTypeInternalServerError,
			wantMessage: geminiServerFailureMessage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseGeminiProviderFailure(CommandResult{ExitCode: 1, Stderr: []byte(tc.output)})
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseGeminiProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
		})
	}
}

func TestParseGeminiProviderFailure_KnownTextAndStructuredPrecedenceAreSafe(t *testing.T) {
	testCases := []struct {
		name        string
		result      CommandResult
		wantReason  interfaces.WorkFailureType
		wantMessage string
	}{
		{
			name: "StructuredErrorOutranksConflictingText",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("HTTP 429 too many requests"),
				Stdout:   []byte(`{"error":{"status":"INVALID_ARGUMENT","message":"Unsupported generation config."}}`),
			},
			wantReason:  interfaces.WorkFailureTypePermanentBadRequest,
			wantMessage: "Unsupported generation config.",
		},
		{
			name:        "AuthenticationTextUsesFixedMessage",
			result:      CommandResult{ExitCode: 1, Stderr: []byte("FatalAuthenticationError: login required")},
			wantReason:  interfaces.WorkFailureTypeAuthFailure,
			wantMessage: geminiAuthFailureMessage,
		},
		{
			name:        "ExitCode124IsTimeout",
			result:      CommandResult{ExitCode: 124},
			wantReason:  interfaces.WorkFailureTypeTimeout,
			wantMessage: geminiTimeoutFailureMessage,
		},
		{
			name: "CredentialMessageUsesSafeFallback",
			result: CommandResult{ExitCode: 1, Stderr: []byte(
				`{"error":{"status":"UNAUTHENTICATED","message":"token=customer-secret-value"}}`,
			)},
			wantReason:  interfaces.WorkFailureTypeAuthFailure,
			wantMessage: geminiAuthFailureMessage,
		},
		{
			name:        "BasicAuthorizationMessageUsesSafeFallback",
			result:      CommandResult{ExitCode: 1, Stderr: []byte(`{"error":{"status":"UNAUTHENTICATED","message":"Authorization: Basic dXNlcjpwYXNz"}}`)},
			wantReason:  interfaces.WorkFailureTypeAuthFailure,
			wantMessage: geminiAuthFailureMessage,
		},
		{
			name: "OrdinaryJSONMessageIsNotAnErrorRecord",
			result: CommandResult{ExitCode: 7, Stdout: []byte(
				`{"type":"message","message":"Explain how rate limits work."}`,
			)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "gemini exited with code 7",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseGeminiProviderFailure(tc.result)
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseGeminiProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
		})
	}
}

func TestParseGeminiProviderFailure_BoundsAndNormalizesActionableMessage(t *testing.T) {
	upstream := "Invalid generation config:\n\t" + strings.Repeat("é", geminiFailureMessageRunes+20)
	payload, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"status":  "INVALID_ARGUMENT",
			"message": upstream,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got := ParseGeminiProviderFailure(CommandResult{ExitCode: 1, Stderr: payload})
	if got.Reason != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("Reason = %q, want %q", got.Reason, interfaces.WorkFailureTypePermanentBadRequest)
	}
	if strings.ContainsAny(got.Message, "\n\t") {
		t.Fatalf("Message = %q, want normalized controls", got.Message)
	}
	if length := len([]rune(got.Message)); length != geminiFailureMessageRunes {
		t.Fatalf("Message rune length = %d, want %d", length, geminiFailureMessageRunes)
	}
}

func TestParseGeminiProviderFailure_DeterministicFallbackPrecedence(t *testing.T) {
	testCases := []struct {
		name        string
		result      CommandResult
		wantReason  interfaces.WorkFailureType
		wantMessage string
	}{
		{
			name: "FinalStructuredErrorWinsAcrossStreams",
			result: CommandResult{
				ExitCode: 1,
				Stdout: []byte(
					"{malformed\n" +
						`{"error":{"status":"INVALID_ARGUMENT","message":"Earlier structured error."}}`,
				),
				Stderr: []byte(
					`{"error":{"status":"UNAUTHENTICATED","message":"Run gemini auth login."}}`,
				),
			},
			wantReason:  interfaces.WorkFailureTypeAuthFailure,
			wantMessage: "Run gemini auth login.",
		},
		{
			name: "FinalUnknownStructuredErrorUsesKnownFieldOnly",
			result: CommandResult{ExitCode: 9, Stderr: []byte(
				"{\"error\":{\"status\":\"UNAVAILABLE\"}}\n" +
					`{"error":{"status":"NEW_PROVIDER_STATUS","message":"Error: selected region is unsupported."}}`,
			)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "Error: selected region is unsupported.",
		},
		{
			name: "UnknownStructuredCredentialUsesExitFallback",
			result: CommandResult{ExitCode: 9, Stderr: []byte(
				`{"error":{"status":"NEW_PROVIDER_STATUS","message":"token=customer-secret"}}`,
			)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "gemini exited with code 9",
		},
		{
			name:        "UnknownTextBasicAuthorizationUsesExitFallback",
			result:      CommandResult{ExitCode: 9, Stderr: []byte("Error: request failed with Authorization: Basic dXNlcjpwYXNz")},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "gemini exited with code 9",
		},
		{
			name: "MalformedStructuredRecordAllowsTextFallback",
			result: CommandResult{ExitCode: 1, Stderr: []byte(
				"{\"error\":\nError: unsupported response mode",
			)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "Error: unsupported response mode",
		},
		{
			name: "MeaningfulStderrOutranksConflictingStdout",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte("Error: internal server error"),
				Stderr:   []byte("Error: invalid request payload"),
			},
			wantReason:  interfaces.WorkFailureTypePermanentBadRequest,
			wantMessage: geminiBadRequestMessage,
		},
		{
			name: "StdoutErrorRecoversFromNoisyStderr",
			result: CommandResult{
				ExitCode: 6,
				Stdout:   []byte("Error: unsupported response mode"),
				Stderr:   []byte("[debug] Error report written to /tmp/gemini-report.txt\ncleanup complete"),
			},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "Error: unsupported response mode",
		},
		{
			name: "FinalSafeErrorCandidateWins",
			result: CommandResult{ExitCode: 6, Stderr: []byte(
				"Error: earlier provider failure\ncleanup failed for /tmp/private\nError: final provider failure",
			)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: "Error: final provider failure",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseGeminiProviderFailure(tc.result)
			if got.Reason != tc.wantReason || got.Message != tc.wantMessage {
				t.Fatalf("ParseGeminiProviderFailure() = %#v, want reason=%q message=%q", got, tc.wantReason, tc.wantMessage)
			}
		})
	}
}

func TestParseGeminiProviderFailure_BoundsInspectedOutputAndUnknownMessage(t *testing.T) {
	outsideScan := "Error: must not survive bounded scan\n" + strings.Repeat("padding without a signal\n", geminiFailureScanBytes)
	got := ParseGeminiProviderFailure(CommandResult{ExitCode: 5, Stderr: []byte(outsideScan)})
	if got.Message != "gemini exited with code 5" {
		t.Fatalf("Message = %q, want bounded-scan exit fallback", got.Message)
	}
	unknown := "Error:\x00\t" + strings.Repeat("é", geminiFailureMessageRunes+20)
	got = ParseGeminiProviderFailure(CommandResult{ExitCode: 5, Stderr: []byte(unknown)})
	if strings.ContainsAny(got.Message, "\x00\t\n") {
		t.Fatalf("Message = %q, want normalized controls", got.Message)
	}
	if length := len([]rune(got.Message)); length != geminiFailureMessageRunes {
		t.Fatalf("Message rune length = %d, want %d", length, geminiFailureMessageRunes)
	}
}
func TestProviderError_Error_PrefersMessageThenCauseThenType(t *testing.T) {

	if got := NewProviderError(interfaces.WorkFailureTypeUnknown, "", nil).Error(); got != "provider error: unknown" {
		t.Fatalf("expected fallback type-based message, got %q", got)
	}
}

func TestNewProviderErrorWithSession_ClonesProviderSessionMetadata(t *testing.T) {
	session := &interfaces.ProviderSessionMetadata{
		Provider: "codex",
		Kind:     "session_id",
		ID:       "sess_codex_123",
	}

	providerErr := NewProviderErrorWithSession(interfaces.WorkFailureTypeAuthFailure, "auth failed", nil, session)
	session.ID = "mutated-session"

	if providerErr.ProviderSession == nil {
		t.Fatal("expected provider session metadata on provider error")
	}
	if providerErr.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session id = %q, want detached original", providerErr.ProviderSession.ID)
	}
}

func TestClassifyProviderFailure_ReturnsDeterministicBehavior(t *testing.T) {
	testCases := []struct {
		name              string
		err               *ProviderError
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name:         "AuthFailure_Terminates",
			err:          NewProviderError(interfaces.WorkFailureTypeAuthFailure, "", nil),
			wantTerminal: true,
		},
		{
			name:         "PermanentBadRequest_Terminates",
			err:          NewProviderError(interfaces.WorkFailureTypePermanentBadRequest, "", nil),
			wantTerminal: true,
		},
		{
			name:              "Throttled_RetriesAndPauses",
			err:               NewProviderError(interfaces.WorkFailureTypeThrottled, "", nil),
			wantRetryable:     true,
			wantThrottlePause: true,
		},
		{
			name:          "InternalServerError_Retries",
			err:           NewProviderError(interfaces.WorkFailureTypeInternalServerError, "", nil),
			wantRetryable: true,
		},
		{
			name:          "Timeout_Retries",
			err:           NewProviderError(interfaces.WorkFailureTypeTimeout, "", nil),
			wantRetryable: true,
		},
		{
			name:         "Unknown_Terminates",
			err:          NewProviderError(interfaces.WorkFailureTypeUnknown, "", nil),
			wantTerminal: true,
		},
		{
			name:         "Misconfigured_Terminates",
			err:          NewProviderError(interfaces.WorkFailureTypeMisconfigured, "", nil),
			wantTerminal: true,
		},
		{
			name:         "EmptyReason_Terminates",
			err:          NewProviderError("", "", nil),
			wantTerminal: true,
		},
		{
			name:         "UnsupportedReason_Terminates",
			err:          NewProviderError(interfaces.WorkFailureType("unsupported"), "", nil),
			wantTerminal: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyProviderFailure(tc.err)
			if got.Retryable != tc.wantRetryable {
				t.Fatalf("expected Retryable=%t, got %t", tc.wantRetryable, got.Retryable)
			}
			if got.Terminal != tc.wantTerminal {
				t.Fatalf("expected Terminal=%t, got %t", tc.wantTerminal, got.Terminal)
			}
			if got.TriggersThrottlePause != tc.wantThrottlePause {
				t.Fatalf("expected TriggersThrottlePause=%t, got %t", tc.wantThrottlePause, got.TriggersThrottlePause)
			}
		})
	}
}

func TestClassifyProviderFailure_CanonicalReasonOverridesConflictingFamily(t *testing.T) {
	testCases := []struct {
		name   string
		reason interfaces.WorkFailureType
		stale  interfaces.WorkFailureFamily
		want   interfaces.WorkFailureDecision
	}{
		{
			name:   "RetryableReasonOverridesTerminalFamily",
			reason: interfaces.WorkFailureTypeInternalServerError,
			stale:  interfaces.WorkFailureFamilyTerminal,
			want:   interfaces.WorkFailureDecision{Retryable: true},
		},
		{
			name:   "TerminalReasonOverridesThrottleFamily",
			reason: interfaces.WorkFailureTypePermanentBadRequest,
			stale:  interfaces.WorkFailureFamilyThrottle,
			want:   interfaces.WorkFailureDecision{Terminal: true},
		},
		{
			name:   "ThrottleReasonOverridesTerminalFamily",
			reason: interfaces.WorkFailureTypeThrottled,
			stale:  interfaces.WorkFailureFamilyTerminal,
			want: interfaces.WorkFailureDecision{
				Retryable:             true,
				TriggersThrottlePause: true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			providerErr := NewProviderError(tc.reason, "failure", nil)
			providerErr.Family = tc.stale
			if got := ClassifyProviderFailure(providerErr); got != tc.want {
				t.Fatalf("ClassifyProviderFailure() = %#v, want %#v", got, tc.want)
			}
			metadata := WorkFailureMetadataFromError(providerErr)
			if metadata.Family != providerErrorFamilyForType(tc.reason) {
				t.Fatalf("WorkFailureMetadataFromError().Family = %q, want reason-derived family", metadata.Family)
			}
		})
	}
}

func TestWorkFailureDecisionFromMetadata_UsesNormalizedTypeAsCanonicalRetryClass(t *testing.T) {
	testCases := []struct {
		name              string
		metadata          *interfaces.WorkFailureMetadata
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name: "InternalServerErrorWithoutFamily_Retries",
			metadata: &interfaces.WorkFailureMetadata{
				Type: interfaces.WorkFailureTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "InternalServerErrorWithStaleTerminalFamily_StillRetries",
			metadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyTerminal,
				Type:   interfaces.WorkFailureTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "CodexWindowsExitCode4294967295WithStaleTerminalFamily_StillRetriesWithoutThrottlePause",
			metadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyTerminal,
				Type:   interfaces.WorkFailureTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "AuthFailureWithStaleRetryableFamily_StillTerminates",
			metadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyRetryable,
				Type:   interfaces.WorkFailureTypeAuthFailure,
			},
			wantTerminal: true,
		},
		{
			name: "ThrottleFamilyWithoutType_UsesFamilyFallback",
			metadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyThrottle,
			},
			wantRetryable:     true,
			wantThrottlePause: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := WorkFailureDecisionFromMetadata(tc.metadata)
			if got.Retryable != tc.wantRetryable {
				t.Fatalf("expected Retryable=%t, got %t", tc.wantRetryable, got.Retryable)
			}
			if got.Terminal != tc.wantTerminal {
				t.Fatalf("expected Terminal=%t, got %t", tc.wantTerminal, got.Terminal)
			}
			if got.TriggersThrottlePause != tc.wantThrottlePause {
				t.Fatalf("expected TriggersThrottlePause=%t, got %t", tc.wantThrottlePause, got.TriggersThrottlePause)
			}
		})
	}
}

func TestWorkFailureMetadataFromError_ProducesGeneralizedFailureMetadata(t *testing.T) {
	providerErr := NewProviderError(interfaces.WorkFailureTypeTimeout, "execution timeout", nil)

	metadata := WorkFailureMetadataFromError(providerErr)
	if metadata == nil {
		t.Fatal("WorkFailureMetadataFromError() = nil, want timeout metadata")
	}
	if metadata.Type != interfaces.WorkFailureTypeTimeout {
		t.Fatalf("Type = %q, want %q", metadata.Type, interfaces.WorkFailureTypeTimeout)
	}
	if metadata.Family != interfaces.WorkFailureFamilyRetryable {
		t.Fatalf("Family = %q, want %q", metadata.Family, interfaces.WorkFailureFamilyRetryable)
	}
}

func TestWorkFailureDecisionFromProviderError_UsesFailureMetadataProjection(t *testing.T) {
	providerErr := NewProviderError(interfaces.WorkFailureTypeInternalServerError, "high demand", nil)
	providerErr.Family = interfaces.WorkFailureFamilyTerminal

	decision := WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("WorkFailureDecisionFromProviderError() = %#v, want retryable non-terminal non-throttle", decision)
	}
}

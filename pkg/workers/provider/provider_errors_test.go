package provider

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
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

func TestParseOpenCodeProviderFailure_KnownCorpusShapesUseCanonicalContract(t *testing.T) {
	testCases := []struct {
		name        string
		wantMessage string
	}{
		{name: "opencode_provider_auth_error", wantMessage: "Authentication required for openai. Run opencode auth login."},
		{name: "opencode_invalid_request_api_error", wantMessage: "The selected model does not support this request."},
		{name: "opencode_rate_limit_text", wantMessage: opencodeThrottleFailureMessage},
		{name: "opencode_timeout_error", wantMessage: opencodeTimeoutFailureMessage},
		{name: "opencode_server_api_error", wantMessage: opencodeServerFailureMessage},
	}

	for _, tc := range testCases {
		entry := providerErrorCorpusEntryForTest(t, tc.name)
		t.Run(tc.name, func(t *testing.T) {
			got := ParseOpenCodeProviderFailure(entry.CommandResult())
			if got.Reason != entry.ExpectedType || got.Message != tc.wantMessage {
				t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want reason=%q message=%q", got, entry.ExpectedType, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}

func TestParseOpenCodeProviderFailure_StructuredFailurePrecedesText(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stderr:   []byte("Error: rate limit exceeded"),
		Stdout:   []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"Choose a supported model."}}}`),
	}

	got := ParseOpenCodeProviderFailure(result)
	if got.Reason != interfaces.WorkFailureTypePermanentBadRequest || got.Message != "Choose a supported model." {
		t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want structured bad request", got)
	}
}

func TestParseOpenCodeProviderFailure_SanitizesKnownActionableDetails(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stdout: []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"prompt: ` +
			strings.Repeat("private ", 100) + ` Authorization: Bearer secret-token"}}}`),
	}

	got := ParseOpenCodeProviderFailure(result)
	if got.Reason != interfaces.WorkFailureTypePermanentBadRequest || got.Message != opencodeBadRequestFailureMessage {
		t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want sanitized fixed bad-request message", got)
	}
	if len(got.Message) > opencodeFailureMessageBytes || strings.Contains(got.Message, "secret-token") || strings.Contains(got.Message, "private") {
		t.Fatalf("message = %q, want bounded message without sensitive detail", got.Message)
	}
}

func TestParseOpenCodeProviderFailure_UnknownFailuresUseSafeBoundedExcerptOrExitCode(t *testing.T) {
	testCases := []struct {
		name        string
		result      CommandResult
		wantMessage string
	}{
		{
			name:        "safe error line",
			result:      CommandResult{ExitCode: 17, Stderr: []byte("loading project\nError: plugin initialization failed\nrendering prompt")},
			wantMessage: "Error: plugin initialization failed",
		},
		{
			name:        "unrecognized structured error",
			result:      CommandResult{ExitCode: 18, Stdout: []byte(`{"type":"error","error":{"name":"PluginError","data":{"message":"Plugin initialization failed."}}}`)},
			wantMessage: "Plugin initialization failed.",
		},
		{
			name:        "oversized safe error",
			result:      CommandResult{ExitCode: 19, Stderr: []byte("Error: " + strings.Repeat("x", opencodeFailureMessageBytes))},
			wantMessage: ("Error: " + strings.Repeat("x", opencodeFailureMessageBytes))[:opencodeFailureMessageBytes],
		},
		{
			name:        "empty output",
			result:      CommandResult{ExitCode: 20},
			wantMessage: "opencode exited with code 20",
		},
		{
			name:        "malformed structured output",
			result:      CommandResult{ExitCode: 21, Stdout: []byte(`{"type":"error","message":"unfinished"`)},
			wantMessage: "opencode exited with code 21",
		},
		{
			name:        "transcript noise",
			result:      CommandResult{ExitCode: 22, Stderr: []byte("user: show credentials\nassistant: working\nprompt: private request")},
			wantMessage: "opencode exited with code 22",
		},
		{
			name:        "secret bearing error",
			result:      CommandResult{ExitCode: 23, Stderr: []byte("Error: Authorization: Bearer secret-token")},
			wantMessage: "opencode exited with code 23",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseOpenCodeProviderFailure(tc.result)
			if got.Reason != interfaces.WorkFailureTypeUnknown || got.Message != tc.wantMessage {
				t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want unknown message %q", got, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}

func TestNormalizeProviderExitFailure_SelectsOpenCodeParserFromNormalizedIdentity(t *testing.T) {
	entry := providerErrorCorpusEntryForTest(t, "opencode_rate_limit_text")
	providerErr := normalizeProviderExitFailure("  OPENCODE  ", entry.CommandResult(), nil, nil)

	if providerErr.Type != interfaces.WorkFailureTypeThrottled || providerErr.Message != opencodeThrottleFailureMessage {
		t.Fatalf("normalizeProviderExitFailure() = %#v, want OpenCode throttle failure", providerErr)
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || decision.Terminal || !decision.TriggersThrottlePause {
		t.Fatalf("decision = %#v, want central throttle policy", decision)
	}
}

func TestNormalizeProviderExitFailure_OpenCodeCorpusUsesCentralPolicy(t *testing.T) {
	for _, name := range []string{
		"opencode_provider_auth_error",
		"opencode_invalid_request_api_error",
		"opencode_rate_limit_text",
		"opencode_timeout_error",
		"opencode_server_api_error",
	} {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(name, func(t *testing.T) {
			providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			if providerErr.Type != entry.ExpectedType || providerErr.Family != entry.ExpectedFamily {
				t.Fatalf("normalized failure = %#v, want type=%q family=%q", providerErr, entry.ExpectedType, entry.ExpectedFamily)
			}
			decision := WorkFailureDecisionFromProviderError(providerErr)
			if decision.Retryable != entry.Retryable || decision.Terminal == entry.Retryable || decision.TriggersThrottlePause != entry.TriggersThrottlePause {
				t.Fatalf("decision = %#v, want retryable=%t terminal=%t throttlePause=%t", decision, entry.Retryable, !entry.Retryable, entry.TriggersThrottlePause)
			}
		})
	}
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

func TestParseKiroProviderFailure_KnownCorpusFixtures(t *testing.T) {
	fixtureNames := []string{
		"kiro_structured_authentication_error",
		"kiro_structured_invalid_request_stdout",
		"kiro_text_authentication_stdout",
		"kiro_structured_throttle_precedes_text",
		"kiro_text_capacity_error",
		"kiro_text_timeout_malformed_structured",
		"kiro_structured_service_unavailable",
	}

	for _, name := range fixtureNames {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			got := ParseKiroProviderFailure(entry.CommandResult())
			if got.Reason != entry.ExpectedType {
				t.Fatalf("Reason = %q, want %q", got.Reason, entry.ExpectedType)
			}
			if got.Message != knownKiroFailure(entry.ExpectedType).Message {
				t.Fatalf("Message = %q, want stable Kiro message %q", got.Message, knownKiroFailure(entry.ExpectedType).Message)
			}
			if len(got.Message) > kiroFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d bytes", len(got.Message), kiroFailureMessageBytes)
			}
			for _, rejected := range entry.RejectMessageContains {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseKiroProviderFailure_KnownTextMessagesNormalizeControlCharactersByReplacement(t *testing.T) {
	got := ParseKiroProviderFailure(CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR:\tauthentication\x00 required\r for Kiro"),
	})
	if got.Reason != interfaces.WorkFailureTypeAuthFailure || got.Message != kiroAuthFailureMessage {
		t.Fatalf("ParseKiroProviderFailure() = %#v, want fixed auth failure", got)
	}
	if strings.ContainsAny(got.Message, "\x00\r\n\t") {
		t.Fatalf("Message = %q, want control-character-normalized text", got.Message)
	}
}

func TestParseKiroProviderFailure_IgnoresKnownTextOutsideBoundedTail(t *testing.T) {
	got := ParseKiroProviderFailure(CommandResult{
		ExitCode: 7,
		Stderr: []byte(
			"ERROR: authentication required\n" + strings.Repeat("cleanup padding\n", kiroErrorLineScanBytes),
		),
	})
	if got.Reason != interfaces.WorkFailureTypeUnknown || got.Message != "kiro-cli exited with code 7" {
		t.Fatalf("ParseKiroProviderFailure() = %#v, want bounded-scan exit fallback", got)
	}
}

func TestParseKiroProviderFailure_ExitTimeoutPrecedesOutput(t *testing.T) {
	got := ParseKiroProviderFailure(CommandResult{
		ExitCode: 124,
		Stderr:   []byte(`{"type":"error","error":{"type":"authentication_error"}}`),
	})
	if got.Reason != interfaces.WorkFailureTypeTimeout || got.Message != kiroTimeoutFailureMessage {
		t.Fatalf("ParseKiroProviderFailure() = %#v, want explicit process timeout", got)
	}
}

func TestParseKiroProviderFailure_UnknownCorpusFixturesUseSafeDeterministicMessages(t *testing.T) {
	testCases := map[string]string{
		"kiro_unknown_stderr_excerpt_precedes_stdout":     "Kiro error: model registry handshake failed",
		"kiro_unknown_stdout_excerpt_after_unsafe_stderr": "Kiro error: plugin bridge failed",
		"kiro_unknown_noise_only_exit_fallback":           "kiro-cli exited with code 11",
	}

	for name, wantMessage := range testCases {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			got := ParseKiroProviderFailure(entry.CommandResult())
			if got.Reason != interfaces.WorkFailureTypeUnknown || got.Message != wantMessage {
				t.Fatalf("ParseKiroProviderFailure() = %#v, want unknown message %q", got, wantMessage)
			}
			if len(got.Message) > kiroFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d bytes", len(got.Message), kiroFailureMessageBytes)
			}
			for _, rejected := range entry.RejectMessageContains {
				if strings.Contains(got.Message, rejected) {
					t.Fatalf("Message = %q, must not contain %q", got.Message, rejected)
				}
			}
		})
	}
}

func TestParseKiroProviderFailure_UnknownExcerptBoundsAndFallbacks(t *testing.T) {
	testCases := []struct {
		name        string
		result      CommandResult
		wantMessage string
	}{
		{
			name:        "EmptyOutput",
			result:      CommandResult{ExitCode: 2},
			wantMessage: "kiro-cli exited with code 2",
		},
		{
			name:        "EmptyAfterSanitization",
			result:      CommandResult{ExitCode: 3, Stderr: []byte("ERROR:\x00\t\r")},
			wantMessage: "kiro-cli exited with code 3",
		},
		{
			name:        "StructuredUnknownRecord",
			result:      CommandResult{ExitCode: 4, Stderr: []byte(`ERROR: {"type":"mystery","message":"private value"}`)},
			wantMessage: "kiro-cli exited with code 4",
		},
		{
			name:        "EnvironmentAssignment",
			result:      CommandResult{ExitCode: 5, Stderr: []byte("ERROR: REGION=private-region is unsupported")},
			wantMessage: "kiro-cli exited with code 5",
		},
		{
			name:        "CredentialBearingError",
			result:      CommandResult{ExitCode: 8, Stderr: []byte("ERROR: request failed with Bearer customer-token")},
			wantMessage: "kiro-cli exited with code 8",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseKiroProviderFailure(tc.result)
			if got.Reason != interfaces.WorkFailureTypeUnknown || got.Message != tc.wantMessage {
				t.Fatalf("ParseKiroProviderFailure() = %#v, want unknown message %q", got, tc.wantMessage)
			}
		})
	}
}

func TestParseKiroProviderFailure_UnknownExcerptIsUTF8SafeAndBounded(t *testing.T) {
	detail := strings.Repeat("é", kiroFailureMessageBytes)
	got := ParseKiroProviderFailure(CommandResult{ExitCode: 6, Stderr: []byte("ERROR: " + detail)})

	if got.Reason != interfaces.WorkFailureTypeUnknown {
		t.Fatalf("Reason = %q, want unknown", got.Reason)
	}
	if len(got.Message) > kiroFailureMessageBytes {
		t.Fatalf("message length = %d, want at most %d bytes", len(got.Message), kiroFailureMessageBytes)
	}
	if !utf8.ValidString(got.Message) {
		t.Fatalf("Message is not valid UTF-8: %q", got.Message)
	}
	if !strings.HasPrefix(got.Message, "Kiro error: é") {
		t.Fatalf("Message = %q, want normalized Kiro excerpt", got.Message)
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

func TestClassifyProviderFailure_SharedCorpusEntriesFollowExpectedRuntimeDecisions(t *testing.T) {
	testCases := []ProviderErrorCorpusEntry{
		providerErrorCorpusEntryForTest(t, "codex_status_429_too_many_requests"),
		providerErrorCorpusEntryForTest(t, "codex_usage_limit_reached"),
		providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model"),
		providerErrorCorpusEntryForTest(t, "codex_internal_server_status_500"),
		providerErrorCorpusEntryForTest(t, "codex_high_demand_temporary_errors"),
		providerErrorCorpusEntryForTest(t, "codex_windows_exit_code_4294967295"),
		providerErrorCorpusEntryForTest(t, "codex_invalid_request_error"),
		providerErrorCorpusEntryForTest(t, "codex_timeout_waiting_for_provider"),
		providerErrorCorpusEntryForTest(t, "codex_authentication_unauthorized"),
		providerErrorCorpusEntryForTest(t, "cursor_usage_limit_reached"),
		providerErrorCorpusEntryForTest(t, "cursor_high_demand_temporary_errors"),
		providerErrorCorpusEntryForTest(t, "kiro_structured_authentication_error"),
		providerErrorCorpusEntryForTest(t, "kiro_structured_invalid_request_stdout"),
		providerErrorCorpusEntryForTest(t, "kiro_text_authentication_stdout"),
		providerErrorCorpusEntryForTest(t, "kiro_structured_throttle_precedes_text"),
		providerErrorCorpusEntryForTest(t, "kiro_text_timeout_malformed_structured"),
		providerErrorCorpusEntryForTest(t, "kiro_structured_service_unavailable"),
		providerErrorCorpusEntryForTest(t, "kiro_unknown_stderr_excerpt_precedes_stdout"),
		providerErrorCorpusEntryForTest(t, "kiro_unknown_stdout_excerpt_after_unsafe_stderr"),
		providerErrorCorpusEntryForTest(t, "kiro_unknown_noise_only_exit_fallback"),
	}

	for _, entry := range testCases {
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			if providerErr.Type != entry.ExpectedType {
				t.Fatalf("%s normalized type = %q, want %q", providerErrorCorpusEntryLabel(entry), providerErr.Type, entry.ExpectedType)
			}
			if providerErr.Family != entry.ExpectedFamily {
				t.Fatalf("%s normalized family = %q, want %q", providerErrorCorpusEntryLabel(entry), providerErr.Family, entry.ExpectedFamily)
			}

			decision := WorkFailureDecisionFromProviderError(providerErr)
			wantTerminal := !entry.Retryable
			if decision.Retryable != entry.Retryable || decision.Terminal != wantTerminal || decision.TriggersThrottlePause != entry.TriggersThrottlePause {
				t.Fatalf(
					"%s decision = %#v, want retryable=%t terminal=%t throttlePause=%t",
					providerErrorCorpusEntryLabel(entry),
					decision,
					entry.Retryable,
					wantTerminal,
					entry.TriggersThrottlePause,
				)
			}
		})
	}
}

func TestNormalizeProviderExitFailure_CleanupHeavyCodexCorpusEntriesKeepTheDecisiveFailure(t *testing.T) {
	testCases := []ProviderErrorCorpusEntry{
		providerErrorCorpusEntryForTest(t, "codex_model_capacity_cleanup_noise"),
		providerErrorCorpusEntryForTest(t, "codex_timeout_cleanup_noise"),
	}

	for _, entry := range testCases {
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			wantMessage := codexTextFailureMessage(entry.ExpectedType)
			if providerErr.Message != wantMessage {
				t.Fatalf("%s normalized message = %q, want %q", providerErrorCorpusEntryLabel(entry), providerErr.Message, wantMessage)
			}
			for _, reject := range entry.RejectMessageContains {
				if strings.Contains(providerErr.Message, reject) {
					t.Fatalf("%s normalized message = %q, want decisive error line without %q", providerErrorCorpusEntryLabel(entry), providerErr.Message, reject)
				}
			}
			if providerErr.Type != entry.ExpectedType {
				t.Fatalf("%s normalized type = %q, want %q", providerErrorCorpusEntryLabel(entry), providerErr.Type, entry.ExpectedType)
			}
			if providerErr.Family != entry.ExpectedFamily {
				t.Fatalf("%s normalized family = %q, want %q", providerErrorCorpusEntryLabel(entry), providerErr.Family, entry.ExpectedFamily)
			}

			decision := WorkFailureDecisionFromProviderError(providerErr)
			wantTerminal := !entry.Retryable
			if decision.Retryable != entry.Retryable || decision.Terminal != wantTerminal || decision.TriggersThrottlePause != entry.TriggersThrottlePause {
				t.Fatalf(
					"%s decision = %#v, want retryable=%t terminal=%t throttlePause=%t",
					providerErrorCorpusEntryLabel(entry),
					decision,
					entry.Retryable,
					wantTerminal,
					entry.TriggersThrottlePause,
				)
			}
		})
	}
}

func TestProviderErrorCorpus_ContainsSupportedCoverageForEachFailureCategory(t *testing.T) {
	corpus := loadProviderErrorCorpusForTest(t)

	for _, category := range []string{
		"throttled",
		"internal_server_error",
		"auth_failure",
		"permanent_bad_request",
		"timeout",
	} {
		if got := len(corpus.SupportedEntriesForCategory(category)); got == 0 {
			t.Fatalf("supported corpus entries for category %q = %d, want at least 1", category, got)
		}
	}
}

func TestCodexProviderBehavior_ClassifiesUsageLimitAsThrottled(t *testing.T) {
	result := providerErrorCorpusEntryForTest(t, "codex_usage_limit_reached").CommandResult()

	providerErr := normalizeProviderExitFailure(string(interfaces.ModelProviderCodex), result, nil, nil)
	if providerErr.Type != interfaces.WorkFailureTypeThrottled {
		t.Fatalf("expected usage limit to classify as %q, got %q", interfaces.WorkFailureTypeThrottled, providerErr.Type)
	}
	if providerErr.Family != interfaces.WorkFailureFamilyThrottle {
		t.Fatalf("expected usage limit to be in family %q, got %q", interfaces.WorkFailureFamilyThrottle, providerErr.Family)
	}
	if providerErr.Message != codexThrottleFailureMessage {
		t.Fatalf("expected normalized error to use the safe throttle message, got %q", providerErr.Message)
	}
}

func TestCodexProviderBehavior_StreamsUserMessageOnStdin(t *testing.T) {
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	req := interfaces.ProviderInferenceRequest{
		ModelProvider:    string(interfaces.ModelProviderCodex),
		Model:            "gpt-5.3-codex-spark",
		UserMessage:      "line one\nline two",
		WorkingDirectory: "workspace",
	}

	args, err := behavior.BuildArgs(context.Background(), req, false, nil)
	if err != nil {
		t.Fatalf("BuildArgs returned error: %v", err)
	}
	commandReq := behavior.BuildCommandRequest(req, args)

	if len(args) == 0 || args[len(args)-1] != "-" {
		t.Fatalf("expected codex args to end with stdin marker, got %#v", args)
	}
	if string(commandReq.Stdin) != req.UserMessage {
		t.Fatalf("expected codex request to stream prompt on stdin, got %q", string(commandReq.Stdin))
	}
}

func TestClaudeProviderBehavior_PassesUserMessageAsArgument(t *testing.T) {
	behavior := claudeProviderBehavior{logger: logging.NoopLogger{}}
	req := interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet",
		UserMessage:   "line one\nline two",
	}

	args, err := behavior.BuildArgs(context.Background(), req, false, nil)
	if err != nil {
		t.Fatalf("BuildArgs returned error: %v", err)
	}
	commandReq := behavior.BuildCommandRequest(req, args)

	if len(args) == 0 || args[len(args)-1] != req.UserMessage {
		t.Fatalf("expected claude args to end with user message, got %#v", args)
	}
	if len(commandReq.Stdin) != 0 {
		t.Fatalf("expected claude request not to use stdin, got %q", string(commandReq.Stdin))
	}
}

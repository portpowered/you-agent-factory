package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
	claudeexitfailure "github.com/portpowered/infinite-you/pkg/workers/provider/claude/exitfailure"
	codexexitfailure "github.com/portpowered/infinite-you/pkg/workers/provider/codex/exitfailure"
	geminipkg "github.com/portpowered/infinite-you/pkg/workers/provider/gemini"
	kiropkg "github.com/portpowered/infinite-you/pkg/workers/provider/kiro"
)

const (
	claudeThrottleFailureMessage         = claudeexitfailure.ThrottleFailureMessage
	claudeTimeoutFailureMessage          = claudeexitfailure.TimeoutFailureMessage
	claudeAuthFailureMessage             = "Claude authentication failed."
	claudeBadRequestFailureMessage       = "Claude rejected the request as invalid."
	claudeConfigFailureMessage           = "Claude is not configured correctly."
	claudeFailureScanBytes               = 64 * 1024
	geminiThrottleFailureMessage         = "The provider is rate limited; retry after capacity becomes available."
	geminiTimeoutFailureMessage          = geminipkg.TimeoutFailureMessage
	codexGPT56SolUpgradeMessage          = codexexitfailure.GPT56SolUpgradeMessage
	codexUnknownFailureMessage           = codexexitfailure.UnknownFailureMessage
	codexAuthFailureMessage              = codexexitfailure.AuthFailureMessage
	codexThrottleFailureMessage          = "Codex is temporarily unavailable due to usage or capacity limits."
	codexServerFailureMessage            = "Codex encountered a temporary server error."
	codexTimeoutFailureMessage           = "Codex request timed out."
	codexWindowsProcessFailureExitCode   = 4294967295
	opencodeThrottleFailureMessage       = opencodeadapter.ThrottleFailureMessage
	opencodeTimeoutFailureMessage        = opencodeadapter.TimeoutFailureMessage
	opencodeServerFailureMessage         = "OpenCode encountered a temporary server error."
	opencodeBadRequestFailureMessage     = opencodeadapter.BadRequestFailureMessage
	opencodeFailureMessageBytes          = 512
	codexBadRequestFailureMessage        = "Codex rejected the request as invalid."
	codexErrorLineScanBytes              = 64 * 1024
	codexFailureMessageBytes             = 1024
	codexHighDemandTemporaryErrorsNeedle = codexexitfailure.HighDemandTemporaryErrorsNeedle
)

func codexTextFailureMessage(reason interfaces.WorkFailureType) string {
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return codexAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return codexBadRequestFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return codexThrottleFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return codexServerFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return codexTimeoutFailureMessage
	default:
		return ""
	}
}

func knownKiroFailure(reason interfaces.WorkFailureType) ProviderFailureResult {
	message := ""
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		message = "Kiro authentication failed. Sign in again and retry."
	case interfaces.WorkFailureTypePermanentBadRequest:
		message = "Kiro rejected the request as invalid."
	case interfaces.WorkFailureTypeThrottled:
		message = "Kiro is temporarily unavailable due to usage or capacity limits."
	case interfaces.WorkFailureTypeTimeout:
		message = kiropkg.TimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		message = "Kiro encountered a temporary service error."
	}
	return ProviderFailureResult{Reason: reason, Message: message}
}

func ParseClaudeProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := claudeexitfailure.ParseProviderFailure(claudeexitfailure.FailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	})
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func ParseGeminiProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := geminipkg.ParseProviderFailure(geminipkg.FailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	})
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func ParseOpenCodeProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := opencodeadapter.ParseProviderFailure(opencodeadapter.FailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	})
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func loadProviderErrorCorpusForTest(t *testing.T) ProviderErrorCorpus {
	t.Helper()

	corpus, err := LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("LoadProviderErrorCorpus() error = %v", err)
	}
	return corpus
}

func TestProviderErrorCorpusReturnsStableCopiesAndRepeatedResults(t *testing.T) {
	t.Parallel()

	corpus := loadProviderErrorCorpusForTest(t)
	entries := corpus.Entries()
	if len(entries) == 0 {
		t.Fatal("provider error corpus is empty")
	}
	repeated := entries[0].RepeatedCommandResults(2)
	if len(repeated) != 2 || repeated[0].ExitCode != entries[0].ExitCode || repeated[1].ExitCode != entries[0].ExitCode {
		t.Fatalf("repeated command results = %#v, want two copies of first corpus entry", repeated)
	}
	entries[0].Name = "mutated"
	if corpus.Entries()[0].Name == "mutated" {
		t.Fatal("Entries() exposed the corpus backing slice")
	}
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

func TestNormalizeProviderExitFailure_ClaudeUsesOneParsedReasonAndMessageForPolicy(t *testing.T) {
	entry := providerErrorCorpusEntryForTest(t, "claude_rate_limit_error")
	providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)

	if providerErr.Type != interfaces.WorkFailureTypeThrottled || providerErr.Message != claudeThrottleFailureMessage {
		t.Fatalf("normalized Claude failure = %#v, want parser reason and message", providerErr)
	}
	if providerErr.Family != interfaces.WorkFailureFamilyThrottle {
		t.Fatalf("Family = %q, want %q", providerErr.Family, interfaces.WorkFailureFamilyThrottle)
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || !decision.TriggersThrottlePause || decision.Terminal {
		t.Fatalf("decision = %#v, want retryable throttle pause", decision)
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
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"customer prompt", "cleanup", "gpt-5.6-sol"},
		},
		{
			name:        "MalformedJSONUsesExitFallback",
			result:      CommandResult{ExitCode: 1, Stderr: []byte(`ERROR: {"type":"error","error":{"message":"private transcript"}`)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"private transcript", "{"},
		},
		{
			name:        "CredentialsUseExitFallback",
			result:      CommandResult{ExitCode: 1, Stderr: []byte("ERROR: request failed with Authorization: Bearer secret-token")},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"secret-token", "Bearer"},
		},
		{
			name: "OutputBeforeScanTailIsIgnored",
			result: CommandResult{ExitCode: 2, Stderr: []byte(
				"ERROR: should not survive the bounded scan\n" + strings.Repeat("cleanup-padding\n", codexErrorLineScanBytes),
			)},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"should not survive"},
		},
		{
			name:        "UnknownErrorExcerptUsesExitFallback",
			result:      CommandResult{ExitCode: 3, Stderr: []byte("ERROR: operation failed " + strings.Repeat("x", codexFailureMessageBytes+200))},
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
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
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"customer prompt", "private details"},
		},
		{
			name:        "TranscriptBearingErrorUsesExitFallback",
			stderr:      "ERROR: transcript: selected model is at capacity in the user's example",
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"transcript", "user's example"},
		},
		{
			name:        "CredentialBearingErrorUsesExitFallback",
			stderr:      "ERROR: arbitrary credential secret_token=customer-private-value",
			wantReason:  interfaces.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
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

func TestClassifyProviderFailure_SharedCodexAndCursorCorpusEntriesFollowExpectedRuntimeDecisions(t *testing.T) {
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

func TestResolveCodexProviderFailure_MapsOwnedResolution(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 503"}}` + "\n"),
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	}
	got, ok := ResolveCodexProviderFailure(result, CodexFailureResolutionInput{})
	if !ok {
		t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
	}
	if got.Result.Reason != interfaces.WorkFailureTypeInternalServerError {
		t.Fatalf("Result = %#v, want server failure", got.Result)
	}
	if got.InternalCause == "" {
		t.Fatal("expected bounded internal cause on resolution")
	}
}

func TestParseCodexProviderFailureLayers_SkipsPrecedence(t *testing.T) {
	result := CommandResult{
		ExitCode: 1,
		Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n"),
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	}
	got := ParseCodexProviderFailureLayers(result)
	if got.Reason != interfaces.WorkFailureTypeAuthFailure {
		t.Fatalf("ParseCodexProviderFailureLayers() = %#v, want stderr auth failure without stream precedence", got)
	}
}

func TestCodexReportingDispatchWrappers_MapNeutralOutcomes(t *testing.T) {
	stream, ok := CodexStructuredStreamReportingOutcome([]byte(
		`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n",
	))
	if !ok || stream.Reason != interfaces.WorkFailureTypeThrottled {
		t.Fatalf("CodexStructuredStreamReportingOutcome() = (%#v, %v), want throttle", stream, ok)
	}

	exit := CodexProcessExitReportingOutcome(CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	})
	if exit.Reason != interfaces.WorkFailureTypeAuthFailure {
		t.Fatalf("CodexProcessExitReportingOutcome() = %#v, want auth failure", exit)
	}
}

func TestCodexSanitizedFailureFixtureFromResolution_ProjectsPolicy(t *testing.T) {
	resolution := ProviderFailureResolution{
		Result: ProviderFailureResult{
			Reason:  interfaces.WorkFailureTypeThrottled,
			Message: "Codex is temporarily unavailable due to usage or capacity limits.",
		},
		InternalCause: "unexpected status 429",
	}
	fixture := CodexSanitizedFailureFixtureFromResolution(resolution)
	if !fixture.Retryable || fixture.InternalCause != resolution.InternalCause {
		t.Fatalf("CodexSanitizedFailureFixtureFromResolution() = %#v, want retryable throttle projection", fixture)
	}
}

func TestCodexSanitizedFailureFixtureFromProviderError_IncludesCause(t *testing.T) {
	providerErr := NewProviderErrorFromResult(ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeTimeout,
		Message: "Codex execution timed out.",
	}, ProviderFailureInternalCauseError("context deadline exceeded"))
	fixture := CodexSanitizedFailureFixtureFromProviderError(providerErr)
	if fixture.Type != interfaces.WorkFailureTypeTimeout || fixture.InternalCause != "context deadline exceeded" {
		t.Fatalf("CodexSanitizedFailureFixtureFromProviderError() = %#v, want timeout with cause", fixture)
	}
}

func TestCodexSanitizedFailureFixtureFromProviderError_NilReturnsZeroValue(t *testing.T) {
	if got := CodexSanitizedFailureFixtureFromProviderError(nil); got != (CodexSanitizedFailureFixture{}) {
		t.Fatalf("CodexSanitizedFailureFixtureFromProviderError(nil) = %#v, want zero fixture", got)
	}
}

func TestParseProviderExitFailure_RoutesOwnedProviderPackages(t *testing.T) {
	testCases := []struct {
		provider string
		result   CommandResult
		want     interfaces.WorkFailureType
	}{
		{
			provider: string(interfaces.ModelProviderClaude),
			result:   CommandResult{ExitCode: 1, Stderr: []byte(`API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"sign in"}}`)},
			want:     interfaces.WorkFailureTypeAuthFailure,
		},
		{
			provider: string(interfaces.ModelProviderGemini),
			result:   CommandResult{ExitCode: 1, Stderr: []byte("ERROR: 429 RESOURCE_EXHAUSTED")},
			want:     interfaces.WorkFailureTypeThrottled,
		},
		{
			provider: string(interfaces.ModelProviderKiro),
			result:   CommandResult{ExitCode: 1, Stderr: []byte("ERROR: Unauthorized")},
			want:     interfaces.WorkFailureTypeAuthFailure,
		},
		{
			provider: string(interfaces.ModelProviderOpenCode),
			result:   CommandResult{ExitCode: 1, Stderr: []byte(`{"error":{"type":"invalid_request_error","message":"bad model"}}`)},
			want:     interfaces.WorkFailureTypePermanentBadRequest,
		},
		{
			provider: string(interfaces.ModelProviderCodex),
			result:   CommandResult{ExitCode: 1, Stderr: []byte("ERROR: unexpected status 429\n")},
			want:     interfaces.WorkFailureTypeThrottled,
		},
		{
			provider: "unknown-provider",
			result:   CommandResult{ExitCode: 9, Stderr: []byte("cleanup noise")},
			want:     interfaces.WorkFailureTypeUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.provider, func(t *testing.T) {
			got := parseProviderExitFailure(tc.provider, tc.result)
			if got.failure.Reason != tc.want {
				t.Fatalf("parseProviderExitFailure() = %#v, want reason %q", got.failure, tc.want)
			}
		})
	}
}

func TestExtractCodexErrorLine_ReturnsLastErrorPrefixedLine(t *testing.T) {
	line, ok := extractCodexErrorLine(CommandResult{Stderr: []byte("planning\nERROR: first\nERROR: decisive")})
	if !ok || line != "ERROR: decisive" {
		t.Fatalf("extractCodexErrorLine() = (%q, %v), want decisive error line", line, ok)
	}
}

func TestSelectFailureByPrecedence_StructuredWinsOverStderr(t *testing.T) {
	throttle := ProviderFailureResult{Reason: interfaces.WorkFailureTypeThrottled, Message: "throttle"}
	auth := ProviderFailureResult{Reason: interfaces.WorkFailureTypeAuthFailure, Message: "auth"}
	exit := ProviderFailureResult{Reason: interfaces.WorkFailureTypeUnknown, Message: "exit fallback"}
	got, ok := SelectFailureByPrecedence([]CompetingFailureSignal{
		{Tier: FailureSignalTierStructured, Recognized: true, Result: throttle},
		{Tier: FailureSignalTierStderr, Recognized: true, Result: auth},
		{Tier: FailureSignalTierExit, Result: exit},
	})
	if !ok || got.Result != throttle {
		t.Fatalf("SelectFailureByPrecedence() = (%#v, %v), want structured throttle", got, ok)
	}
}

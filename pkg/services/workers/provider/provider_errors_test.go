// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/opencode"
	claudeexitfailure "github.com/portpowered/infinite-you/pkg/services/workers/provider/claude/exitfailure"
	codexexitfailure "github.com/portpowered/infinite-you/pkg/services/workers/provider/codex/exitfailure"
)

const (
	claudeThrottleFailureMessage         = claudeexitfailure.ThrottleFailureMessage
	claudeTimeoutFailureMessage          = claudeexitfailure.TimeoutFailureMessage
	claudeAuthFailureMessage             = "Claude authentication failed."
	claudeBadRequestFailureMessage       = "Claude rejected the request as invalid."
	claudeConfigFailureMessage           = "Claude is not configured correctly."
	claudeFailureScanBytes               = 64 * 1024
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

func codexTextFailureMessage(reason workerexecution.WorkFailureType) string {
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		return codexAuthFailureMessage
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return codexBadRequestFailureMessage
	case workerexecution.WorkFailureTypeThrottled:
		return codexThrottleFailureMessage
	case workerexecution.WorkFailureTypeInternalServerError:
		return codexServerFailureMessage
	case workerexecution.WorkFailureTypeTimeout:
		return codexTimeoutFailureMessage
	default:
		return ""
	}
}

func ParseClaudeProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := claudeexitfailure.ParseProviderFailure(claudeexitfailure.FailureInput{
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
	t.Parallel()
	entry := providerErrorCorpusEntryForTest(t, "claude_rate_limit_error")
	providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)

	if providerErr.Type != workerexecution.WorkFailureTypeThrottled || providerErr.Message != claudeThrottleFailureMessage {
		t.Fatalf("normalized Claude failure = %#v, want parser reason and message", providerErr)
	}
	if providerErr.Family != workerexecution.WorkFailureFamilyThrottle {
		t.Fatalf("Family = %q, want %q", providerErr.Family, workerexecution.WorkFailureFamilyThrottle)
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || !decision.TriggersThrottlePause || decision.Terminal {
		t.Fatalf("decision = %#v, want retryable throttle pause", decision)
	}
}

func TestParseCodexProviderFailure_GPT56SolReturnsActionableNestedMessage(t *testing.T) {
	t.Parallel()
	entry := providerErrorCorpusEntryForTest(t, "codex_gpt_5_6_sol_requires_newer_cli")
	wantMessage := codexGPT56SolUpgradeMessage

	got := ParseCodexProviderFailure(entry.CommandResult())
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("Reason = %q, want %q", got.Reason, workerexecution.WorkFailureTypePermanentBadRequest)
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
	t.Parallel()
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
	if got.Reason != workerexecution.WorkFailureTypeInternalServerError || got.Message != codexServerFailureMessage {
		t.Fatalf("ParseCodexProviderFailure() = %#v, want final valid stdout server failure", got)
	}
}

func TestParseCodexProviderFailure_StructuredFieldsPrecedeSubstringFallback(t *testing.T) {
	t.Parallel()
	result := CommandResult{
		ExitCode: 1,
		Stderr: []byte(strings.Join([]string{
			`ERROR: transcript said 429 too many requests`,
			`ERROR: {"type":"error","status":400,"error":{"type":"invalid_request_error","message":"choose a supported model"}}`,
			`ERROR: cleanup failed after request`,
		}, "\n")),
	}

	got := ParseCodexProviderFailure(result)
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest || got.Message != codexBadRequestFailureMessage {
		t.Fatalf("ParseCodexProviderFailure() = %#v, want structured bad request", got)
	}
}

func TestParseCodexProviderFailure_UsesOuterStructuredTypeAndMessage(t *testing.T) {
	t.Parallel()
	result := CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`ERROR: {"type":"rate_limit_error","message":"request capacity reached"}`),
	}

	got := ParseCodexProviderFailure(result)
	if got.Reason != workerexecution.WorkFailureTypeThrottled || got.Message != codexThrottleFailureMessage {
		t.Fatalf("ParseCodexProviderFailure() = %#v, want outer structured throttle failure", got)
	}
}

func TestParseCodexProviderFailure_KnownCodexShapesKeepCanonicalReasons(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	testCases := []struct {
		name        string
		result      CommandResult
		wantReason  workerexecution.WorkFailureType
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
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"customer prompt", "cleanup", "gpt-5.6-sol"},
		},
		{
			name:        "MalformedJSONUsesExitFallback",
			result:      CommandResult{ExitCode: 1, Stderr: []byte(`ERROR: {"type":"error","error":{"message":"private transcript"}`)},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"private transcript", "{"},
		},
		{
			name:        "CredentialsUseExitFallback",
			result:      CommandResult{ExitCode: 1, Stderr: []byte("ERROR: request failed with Authorization: Bearer secret-token")},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"secret-token", "Bearer"},
		},
		{
			name: "OutputBeforeScanTailIsIgnored",
			result: CommandResult{ExitCode: 2, Stderr: []byte(
				"ERROR: should not survive the bounded scan\n" + strings.Repeat("cleanup-padding\n", codexErrorLineScanBytes),
			)},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"should not survive"},
		},
		{
			name:        "UnknownErrorExcerptUsesExitFallback",
			result:      CommandResult{ExitCode: 3, Stderr: []byte("ERROR: operation failed " + strings.Repeat("x", codexFailureMessageBytes+200))},
			wantReason:  workerexecution.WorkFailureTypeUnknown,
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
	t.Parallel()
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
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"customer prompt", "private details"},
		},
		{
			name:        "TranscriptBearingErrorUsesExitFallback",
			stderr:      "ERROR: transcript: selected model is at capacity in the user's example",
			wantReason:  workerexecution.WorkFailureTypeUnknown,
			wantMessage: codexUnknownFailureMessage,
			reject:      []string{"transcript", "user's example"},
		},
		{
			name:        "CredentialBearingErrorUsesExitFallback",
			stderr:      "ERROR: arbitrary credential secret_token=customer-private-value",
			wantReason:  workerexecution.WorkFailureTypeUnknown,
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
	t.Parallel()
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
			name:        "StructuredTranscriptUsesFixedMessage",
			stderr:      `ERROR: {"status":500,"error":{"type":"server_error","message":"transcript: user's private response draft"}}`,
			wantReason:  workerexecution.WorkFailureTypeInternalServerError,
			wantMessage: codexServerFailureMessage,
			reject:      []string{"transcript", "private response"},
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
	t.Parallel()

	if got := NewProviderError(workerexecution.WorkFailureTypeUnknown, "", nil).Error(); got != "provider error: unknown" {
		t.Fatalf("expected fallback type-based message, got %q", got)
	}
}

func TestNewProviderErrorWithSession_ClonesProviderSessionMetadata(t *testing.T) {
	t.Parallel()
	session := &workerexecution.ProviderSessionMetadata{
		Provider: "codex",
		Kind:     "session_id",
		ID:       "sess_codex_123",
	}

	providerErr := NewProviderErrorWithSession(workerexecution.WorkFailureTypeAuthFailure, "auth failed", nil, session)
	session.ID = "mutated-session"

	if providerErr.ProviderSession == nil {
		t.Fatal("expected provider session metadata on provider error")
	}
	if providerErr.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session id = %q, want detached original", providerErr.ProviderSession.ID)
	}
}

func TestClassifyProviderFailure_ReturnsDeterministicBehavior(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name              string
		err               *ProviderError
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name:         "AuthFailure_Terminates",
			err:          NewProviderError(workerexecution.WorkFailureTypeAuthFailure, "", nil),
			wantTerminal: true,
		},
		{
			name:         "PermanentBadRequest_Terminates",
			err:          NewProviderError(workerexecution.WorkFailureTypePermanentBadRequest, "", nil),
			wantTerminal: true,
		},
		{
			name:              "Throttled_RetriesAndPauses",
			err:               NewProviderError(workerexecution.WorkFailureTypeThrottled, "", nil),
			wantRetryable:     true,
			wantThrottlePause: true,
		},
		{
			name:          "InternalServerError_Retries",
			err:           NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "", nil),
			wantRetryable: true,
		},
		{
			name:          "Timeout_Retries",
			err:           NewProviderError(workerexecution.WorkFailureTypeTimeout, "", nil),
			wantRetryable: true,
		},
		{
			name:         "Unknown_Terminates",
			err:          NewProviderError(workerexecution.WorkFailureTypeUnknown, "", nil),
			wantTerminal: true,
		},
		{
			name:         "Misconfigured_Terminates",
			err:          NewProviderError(workerexecution.WorkFailureTypeMisconfigured, "", nil),
			wantTerminal: true,
		},
		{
			name:         "EmptyReason_Terminates",
			err:          NewProviderError("", "", nil),
			wantTerminal: true,
		},
		{
			name:         "UnsupportedReason_Terminates",
			err:          NewProviderError(workerexecution.WorkFailureType("unsupported"), "", nil),
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
	t.Parallel()
	testCases := []struct {
		name   string
		reason workerexecution.WorkFailureType
		stale  workerexecution.WorkFailureFamily
		want   workerexecution.WorkFailureDecision
	}{
		{
			name:   "RetryableReasonOverridesTerminalFamily",
			reason: workerexecution.WorkFailureTypeInternalServerError,
			stale:  workerexecution.WorkFailureFamilyTerminal,
			want:   workerexecution.WorkFailureDecision{Retryable: true},
		},
		{
			name:   "TerminalReasonOverridesThrottleFamily",
			reason: workerexecution.WorkFailureTypePermanentBadRequest,
			stale:  workerexecution.WorkFailureFamilyThrottle,
			want:   workerexecution.WorkFailureDecision{Terminal: true},
		},
		{
			name:   "ThrottleReasonOverridesTerminalFamily",
			reason: workerexecution.WorkFailureTypeThrottled,
			stale:  workerexecution.WorkFailureFamilyTerminal,
			want: workerexecution.WorkFailureDecision{
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
	t.Parallel()
	testCases := []struct {
		name              string
		metadata          *workerexecution.WorkFailureMetadata
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name: "InternalServerErrorWithoutFamily_Retries",
			metadata: &workerexecution.WorkFailureMetadata{
				Type: workerexecution.WorkFailureTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "InternalServerErrorWithStaleTerminalFamily_StillRetries",
			metadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyTerminal,
				Type:   workerexecution.WorkFailureTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "CodexWindowsExitCode4294967295WithStaleTerminalFamily_StillRetriesWithoutThrottlePause",
			metadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyTerminal,
				Type:   workerexecution.WorkFailureTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "AuthFailureWithStaleRetryableFamily_StillTerminates",
			metadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyRetryable,
				Type:   workerexecution.WorkFailureTypeAuthFailure,
			},
			wantTerminal: true,
		},
		{
			name: "ThrottleFamilyWithoutType_UsesFamilyFallback",
			metadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyThrottle,
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
	t.Parallel()
	providerErr := NewProviderError(workerexecution.WorkFailureTypeTimeout, "execution timeout", nil)

	metadata := WorkFailureMetadataFromError(providerErr)
	if metadata == nil {
		t.Fatal("WorkFailureMetadataFromError() = nil, want timeout metadata")
	}
	if metadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("Type = %q, want %q", metadata.Type, workerexecution.WorkFailureTypeTimeout)
	}
	if metadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("Family = %q, want %q", metadata.Family, workerexecution.WorkFailureFamilyRetryable)
	}
}

func TestProviderFailureBoundaryHelpersPreserveSafeObservableBehavior(t *testing.T) {
	t.Parallel()

	if got := NormalizeProviderExecutionError(nil); got != nil {
		t.Fatalf("NormalizeProviderExecutionError(nil) = %#v, want nil", got)
	}
	if got := NormalizeProviderExecutionError(errors.New("unclassified failure")); got != nil {
		t.Fatalf("NormalizeProviderExecutionError(unclassified) = %#v, want nil", got)
	}
	deadlineErr := NormalizeProviderExecutionError(context.DeadlineExceeded)
	if deadlineErr == nil || deadlineErr.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("NormalizeProviderExecutionError(deadline) = %#v, want timeout", deadlineErr)
	}

	if got := SafeProviderFailureDetail(nil); got != nil {
		t.Fatalf("SafeProviderFailureDetail(nil) = %#v, want nil", got)
	}
	providerErr := NewProviderError(
		workerexecution.WorkFailureTypeAuthFailure,
		"Authentication failed.",
		errors.New("secret native output"),
	)
	detail := SafeProviderFailureDetail(providerErr)
	if detail == nil {
		t.Fatal("SafeProviderFailureDetail(error) = nil, want public detail")
	}
	if detail.Reason != workerexecution.WorkFailureTypeAuthFailure || detail.Message != "Provider authentication failed." {
		t.Fatalf("SafeProviderFailureDetail(error) = %#v, want stable auth detail", detail)
	}
	for _, unsafe := range []string{"Authentication failed.", "secret native output"} {
		if strings.Contains(detail.Message, unsafe) {
			t.Fatalf("SafeProviderFailureDetail(error) leaked %q: %#v", unsafe, detail)
		}
	}

	if got := ClassifyProviderFailure(nil); got != (workerexecution.WorkFailureDecision{}) {
		t.Fatalf("ClassifyProviderFailure(nil) = %#v, want zero decision", got)
	}
	if got := WorkFailureMetadataFromError(nil); got != nil {
		t.Fatalf("WorkFailureMetadataFromError(nil) = %#v, want nil", got)
	}
}

func TestProviderCompatibilityHelpersReportCapabilitiesAndStopTokens(t *testing.T) {
	t.Parallel()

	runner := NewInferenceProgressPublishingCommandRunner(nil, logging.NoopLogger{})
	streaming, ok := runner.(interface{ SupportsResponseStreaming() bool })
	if !ok || !streaming.SupportsResponseStreaming() {
		t.Fatalf("progress runner = %T, want streaming-capable runner", runner)
	}

	if !ContainsStopToken("work <promise>COMPLETE</promise>", "<promise>COMPLETE</promise>") {
		t.Fatal("ContainsStopToken() = false, want exact token match")
	}
	if ContainsStopToken("work COMPLETE", "") {
		t.Fatal("ContainsStopToken() = true for empty token")
	}
}

func TestWorkFailureDecisionFromProviderError_UsesFailureMetadataProjection(t *testing.T) {
	t.Parallel()
	providerErr := NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "high demand", nil)
	providerErr.Family = workerexecution.WorkFailureFamilyTerminal

	decision := WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("WorkFailureDecisionFromProviderError() = %#v, want retryable non-terminal non-throttle", decision)
	}
}

func TestClassifyProviderFailure_SharedCodexAndCursorCorpusEntriesFollowExpectedRuntimeDecisions(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	result := providerErrorCorpusEntryForTest(t, "codex_usage_limit_reached").CommandResult()

	providerErr := normalizeProviderExitFailure(string(modelprovider.ProviderCodex), result, nil, nil)
	if providerErr.Type != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("expected usage limit to classify as %q, got %q", workerexecution.WorkFailureTypeThrottled, providerErr.Type)
	}
	if providerErr.Family != workerexecution.WorkFailureFamilyThrottle {
		t.Fatalf("expected usage limit to be in family %q, got %q", workerexecution.WorkFailureFamilyThrottle, providerErr.Family)
	}
	if providerErr.Message != codexThrottleFailureMessage {
		t.Fatalf("expected normalized error to use the safe throttle message, got %q", providerErr.Message)
	}
}

func TestCodexProviderBehavior_StreamsUserMessageOnStdin(t *testing.T) {
	t.Parallel()
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	req := workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCodex),
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
	t.Parallel()
	behavior := claudeProviderBehavior{logger: logging.NoopLogger{}}
	req := workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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
	t.Parallel()
	result := CommandResult{
		ExitCode: 1,
		Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 503"}}` + "\n"),
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	}
	got, ok := ResolveCodexProviderFailure(result, CodexFailureResolutionInput{})
	if !ok {
		t.Fatal("ResolveCodexProviderFailure() ok = false, want true")
	}
	if got.Result.Reason != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("Result = %#v, want server failure", got.Result)
	}
	if got.InternalCause == "" {
		t.Fatal("expected bounded internal cause on resolution")
	}
}

func TestParseCodexProviderFailureLayers_SkipsPrecedence(t *testing.T) {
	t.Parallel()
	result := CommandResult{
		ExitCode: 1,
		Stdout:   []byte(`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n"),
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	}
	got := ParseCodexProviderFailureLayers(result)
	if got.Reason != workerexecution.WorkFailureTypeAuthFailure {
		t.Fatalf("ParseCodexProviderFailureLayers() = %#v, want stderr auth failure without stream precedence", got)
	}
}

func TestCodexReportingDispatchWrappers_MapNeutralOutcomes(t *testing.T) {
	t.Parallel()
	stream, ok := CodexStructuredStreamReportingOutcome([]byte(
		`{"type":"turn.failed","error":{"message":"unexpected status 429"}}` + "\n",
	))
	if !ok || stream.Reason != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("CodexStructuredStreamReportingOutcome() = (%#v, %v), want throttle", stream, ok)
	}

	exit := CodexProcessExitReportingOutcome(CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR: unexpected status 401\n"),
	})
	if exit.Reason != workerexecution.WorkFailureTypeAuthFailure {
		t.Fatalf("CodexProcessExitReportingOutcome() = %#v, want auth failure", exit)
	}
}

func TestCodexSanitizedFailureFixtureFromResolution_ProjectsPolicy(t *testing.T) {
	t.Parallel()
	resolution := ProviderFailureResolution{
		Result: ProviderFailureResult{
			Reason:  workerexecution.WorkFailureTypeThrottled,
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
	t.Parallel()
	providerErr := NewProviderErrorFromResult(ProviderFailureResult{
		Reason:  workerexecution.WorkFailureTypeTimeout,
		Message: "Codex execution timed out.",
	}, ProviderFailureInternalCauseError("context deadline exceeded"))
	fixture := CodexSanitizedFailureFixtureFromProviderError(providerErr)
	if fixture.Type != workerexecution.WorkFailureTypeTimeout || fixture.InternalCause != "context deadline exceeded" {
		t.Fatalf("CodexSanitizedFailureFixtureFromProviderError() = %#v, want timeout with cause", fixture)
	}
}

func TestCodexSanitizedFailureFixtureFromProviderError_NilReturnsZeroValue(t *testing.T) {
	t.Parallel()
	if got := CodexSanitizedFailureFixtureFromProviderError(nil); got != (CodexSanitizedFailureFixture{}) {
		t.Fatalf("CodexSanitizedFailureFixtureFromProviderError(nil) = %#v, want zero fixture", got)
	}
}

func TestParseProviderExitFailure_RoutesOwnedProviderPackages(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		provider string
		result   CommandResult
		want     workerexecution.WorkFailureType
	}{
		{
			provider: string(modelprovider.ProviderClaude),
			result:   CommandResult{ExitCode: 1, Stderr: []byte(`API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"sign in"}}`)},
			want:     workerexecution.WorkFailureTypeAuthFailure,
		},
		{
			provider: string(modelprovider.ProviderOpenCode),
			result:   CommandResult{ExitCode: 1, Stderr: []byte(`{"error":{"type":"invalid_request_error","message":"bad model"}}`)},
			want:     workerexecution.WorkFailureTypePermanentBadRequest,
		},
		{
			provider: string(modelprovider.ProviderCodex),
			result:   CommandResult{ExitCode: 1, Stderr: []byte("ERROR: unexpected status 429\n")},
			want:     workerexecution.WorkFailureTypeThrottled,
		},
		{
			provider: string(modelprovider.ProviderKiro),
			result:   CommandResult{ExitCode: 124},
			want:     workerexecution.WorkFailureTypeTimeout,
		},
		{
			provider: "unknown-provider",
			result:   CommandResult{ExitCode: 9, Stderr: []byte("cleanup noise")},
			want:     workerexecution.WorkFailureTypeUnknown,
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

func TestNormalizeProviderExecutionError_UsesExitFailureParserForNonZeroExitCode(t *testing.T) {
	t.Parallel()

	providerErr := normalizeProviderExecutionError(
		string(modelprovider.ProviderKiro),
		CommandResult{ExitCode: 124},
		errors.New("command failed"),
		nil,
		nil,
	)
	if providerErr.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("normalizeProviderExecutionError() = %#v, want timeout", providerErr)
	}
}

func TestExtractCodexErrorLine_ReturnsLastErrorPrefixedLine(t *testing.T) {
	t.Parallel()
	line, ok := extractCodexErrorLine(CommandResult{Stderr: []byte("planning\nERROR: first\nERROR: decisive")})
	if !ok || line != "ERROR: decisive" {
		t.Fatalf("extractCodexErrorLine() = (%q, %v), want decisive error line", line, ok)
	}
}

func TestSelectFailureByPrecedence_StructuredWinsOverStderr(t *testing.T) {
	t.Parallel()
	throttle := ProviderFailureResult{Reason: workerexecution.WorkFailureTypeThrottled, Message: "throttle"}
	auth := ProviderFailureResult{Reason: workerexecution.WorkFailureTypeAuthFailure, Message: "auth"}
	exit := ProviderFailureResult{Reason: workerexecution.WorkFailureTypeUnknown, Message: "exit fallback"}
	got, ok := SelectFailureByPrecedence([]CompetingFailureSignal{
		{Tier: FailureSignalTierStructured, Recognized: true, Result: throttle},
		{Tier: FailureSignalTierStderr, Recognized: true, Result: auth},
		{Tier: FailureSignalTierExit, Result: exit},
	})
	if !ok || got.Result != throttle {
		t.Fatalf("SelectFailureByPrecedence() = (%#v, %v), want structured throttle", got, ok)
	}
}

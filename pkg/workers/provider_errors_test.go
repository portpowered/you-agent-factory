package workers

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestNewProviderError_AssignsDeterministicFamilyFromType(t *testing.T) {
	testCases := []struct {
		name       string
		errorType  interfaces.ProviderErrorType
		wantFamily interfaces.ProviderErrorFamily
	}{
		{name: "AuthFailure_IsTerminal", errorType: interfaces.ProviderErrorTypeAuthFailure, wantFamily: interfaces.ProviderErrorFamilyTerminal},
		{name: "PermanentBadRequest_IsTerminal", errorType: interfaces.ProviderErrorTypePermanentBadRequest, wantFamily: interfaces.ProviderErrorFamilyTerminal},
		{name: "Throttled_IsThrottle", errorType: interfaces.ProviderErrorTypeThrottled, wantFamily: interfaces.ProviderErrorFamilyThrottle},
		{name: "InternalServerError_IsRetryable", errorType: interfaces.ProviderErrorTypeInternalServerError, wantFamily: interfaces.ProviderErrorFamilyRetryable},
		{name: "Timeout_IsRetryable", errorType: interfaces.ProviderErrorTypeTimeout, wantFamily: interfaces.ProviderErrorFamilyRetryable},
		{name: "Unknown_IsTerminal", errorType: interfaces.ProviderErrorTypeUnknown, wantFamily: interfaces.ProviderErrorFamilyTerminal},
		{name: "Misconfigured_IsTerminal", errorType: interfaces.ProviderErrorTypeMisconfigured, wantFamily: interfaces.ProviderErrorFamilyTerminal},
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

func TestProviderError_Error_PrefersMessageThenCauseThenType(t *testing.T) {

	if got := NewProviderError(interfaces.ProviderErrorTypeUnknown, "", nil).Error(); got != "provider error: unknown" {
		t.Fatalf("expected fallback type-based message, got %q", got)
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
			err:          NewProviderError(interfaces.ProviderErrorTypeAuthFailure, "", nil),
			wantTerminal: true,
		},
		{
			name:         "PermanentBadRequest_Terminates",
			err:          NewProviderError(interfaces.ProviderErrorTypePermanentBadRequest, "", nil),
			wantTerminal: true,
		},
		{
			name:              "Throttled_RetriesAndPauses",
			err:               NewProviderError(interfaces.ProviderErrorTypeThrottled, "", nil),
			wantRetryable:     true,
			wantThrottlePause: true,
		},
		{
			name:          "InternalServerError_Retries",
			err:           NewProviderError(interfaces.ProviderErrorTypeInternalServerError, "", nil),
			wantRetryable: true,
		},
		{
			name:          "Timeout_Retries",
			err:           NewProviderError(interfaces.ProviderErrorTypeTimeout, "", nil),
			wantRetryable: true,
		},
		{
			name:         "Unknown_Terminates",
			err:          NewProviderError(interfaces.ProviderErrorTypeUnknown, "", nil),
			wantTerminal: true,
		},
		{
			name:         "Misconfigured_Terminates",
			err:          NewProviderError(interfaces.ProviderErrorTypeMisconfigured, "", nil),
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

func TestProviderFailureDecisionFromMetadata_UsesNormalizedTypeAsCanonicalRetryClass(t *testing.T) {
	testCases := []struct {
		name              string
		metadata          *interfaces.ProviderFailureMetadata
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name: "InternalServerErrorWithoutFamily_Retries",
			metadata: &interfaces.ProviderFailureMetadata{
				Type: interfaces.ProviderErrorTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "InternalServerErrorWithStaleTerminalFamily_StillRetries",
			metadata: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyTerminal,
				Type:   interfaces.ProviderErrorTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "CodexWindowsExitCode4294967295WithStaleTerminalFamily_StillRetriesWithoutThrottlePause",
			metadata: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyTerminal,
				Type:   interfaces.ProviderErrorTypeInternalServerError,
			},
			wantRetryable: true,
		},
		{
			name: "AuthFailureWithStaleRetryableFamily_StillTerminates",
			metadata: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyRetryable,
				Type:   interfaces.ProviderErrorTypeAuthFailure,
			},
			wantTerminal: true,
		},
		{
			name: "ThrottleFamilyWithoutType_UsesFamilyFallback",
			metadata: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
			},
			wantRetryable:     true,
			wantThrottlePause: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProviderFailureDecisionFromMetadata(tc.metadata)
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

			decision := ClassifyProviderFailure(providerErr)
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

	providerErr := normalizeProviderExitFailure(string(ModelProviderCodex), result, nil, nil)
	if providerErr.Type != interfaces.ProviderErrorTypeThrottled {
		t.Fatalf("expected usage limit to classify as %q, got %q", interfaces.ProviderErrorTypeThrottled, providerErr.Type)
	}
	if providerErr.Family != interfaces.ProviderErrorFamilyThrottle {
		t.Fatalf("expected usage limit to be in family %q, got %q", interfaces.ProviderErrorFamilyThrottle, providerErr.Family)
	}
	if !strings.Contains(providerErr.Message, "usage limit") {
		t.Fatalf("expected normalized error to preserve usage limit message, got %q", providerErr.Message)
	}
}

func TestCodexProviderBehavior_StreamsUserMessageOnStdin(t *testing.T) {
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	req := interfaces.ProviderInferenceRequest{
		ModelProvider:    string(ModelProviderCodex),
		Model:            "gpt-5.3-codex-spark",
		UserMessage:      "line one\nline two",
		WorkingDirectory: "workspace",
	}

	args := behavior.BuildArgs(req, false)
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
		ModelProvider: string(ModelProviderClaude),
		Model:         "claude-sonnet",
		UserMessage:   "line one\nline two",
	}

	args := behavior.BuildArgs(req, false)
	commandReq := behavior.BuildCommandRequest(req, args)

	if len(args) == 0 || args[len(args)-1] != req.UserMessage {
		t.Fatalf("expected claude args to end with user message, got %#v", args)
	}
	if len(commandReq.Stdin) != 0 {
		t.Fatalf("expected claude request not to use stdin, got %q", string(commandReq.Stdin))
	}
}

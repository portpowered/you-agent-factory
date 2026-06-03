package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

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

func TestNormalizeProviderExitFailure_CleanupHeavyCodexCorpusEntriesKeepTheDecisiveErrorLine(t *testing.T) {
	testCases := []ProviderErrorCorpusEntry{
		providerErrorCorpusEntryForTest(t, "codex_model_capacity_cleanup_noise"),
		providerErrorCorpusEntryForTest(t, "codex_timeout_cleanup_noise"),
	}

	for _, entry := range testCases {
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			wantMessage := providerErrorCorpusLastErrorLine(t, entry)
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
	if !strings.Contains(providerErr.Message, "usage limit") {
		t.Fatalf("expected normalized error to preserve usage limit message, got %q", providerErr.Message)
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

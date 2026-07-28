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
	opencodepkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/opencode"
)

const (
	opencodeThrottleFailureMessage   = opencodepkg.ThrottleFailureMessage
	opencodeTimeoutFailureMessage    = opencodepkg.TimeoutFailureMessage
	opencodeServerFailureMessage     = "OpenCode encountered a temporary server error."
	opencodeBadRequestFailureMessage = opencodepkg.BadRequestFailureMessage
	opencodeFailureMessageBytes      = 512
)

func ParseOpenCodeProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := opencodepkg.ParseProviderFailure(opencodepkg.FailureInput{
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

func TestClassifyProviderFailure_CursorCorpusEntriesFollowExpectedRuntimeDecisions(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []ProviderErrorCorpusEntry{
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

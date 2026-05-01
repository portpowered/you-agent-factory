//go:build functionallong

package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

type providerLongCase struct {
	name              string
	corpusEntry       string
	provider          workers.ModelProvider
	model             string
	wantCalls         int
	wantPlace         string
	wantThrottlePause bool
}

func TestProviderErrorLong_ScriptWrapScenariosStayNormalizedAcrossProviders(t *testing.T) {
	for _, tc := range providerErrorLongCases() {
		tc := tc
		subtestName := tc.name
		if tc.corpusEntry != "" {
			subtestName += "_" + tc.corpusEntry
		}
		t.Run(subtestName, func(t *testing.T) {
			runProviderErrorLongCase(t, tc)
		})
	}
}

func providerErrorLongCases() []providerLongCase {
	return []providerLongCase{
		{name: "Claude_Throttled_RequeuesAfterBoundedRetries", corpusEntry: "claude_rate_limit_error", provider: workers.ModelProviderClaude, model: "claude-sonnet-4-5-20250514", wantCalls: 3, wantPlace: "task:init", wantThrottlePause: true},
		{name: "Claude_TransientServerError_RequeuesAfterBoundedRetries", corpusEntry: "claude_internal_server_api_error", provider: workers.ModelProviderClaude, model: "claude-sonnet-4-5-20250514", wantCalls: 3, wantPlace: "task:init"},
		{name: "Claude_Timeout_RequeuesAfterBoundedRetries", corpusEntry: "claude_timeout_waiting_for_provider", provider: workers.ModelProviderClaude, model: "claude-sonnet-4-5-20250514", wantCalls: 3, wantPlace: "task:init"},
		{name: "Claude_PermanentBadRequest_FailsWithoutRetry", corpusEntry: "claude_invalid_request_error", provider: workers.ModelProviderClaude, model: "claude-sonnet-4-5-20250514", wantCalls: 1, wantPlace: "task:failed"},
		{name: "Claude_Unknown_FailsWithoutRetry", provider: workers.ModelProviderClaude, model: "claude-sonnet-4-5-20250514", wantCalls: 1, wantPlace: "task:failed"},
		{name: "Codex_Throttled_RequeuesAfterBoundedRetries", corpusEntry: "codex_status_429_too_many_requests", provider: workers.ModelProviderCodex, model: "gpt-5-codex", wantCalls: 3, wantPlace: "task:init", wantThrottlePause: true},
		{name: "Codex_TransientServerError_RequeuesAfterBoundedRetries", corpusEntry: "codex_internal_server_status_500", provider: workers.ModelProviderCodex, model: "gpt-5-codex", wantCalls: 3, wantPlace: "task:init"},
		{name: "Codex_HighDemandTemporaryServerError_RequeuesWithoutThrottlePause", corpusEntry: "codex_high_demand_temporary_errors", provider: workers.ModelProviderCodex, model: "gpt-5-codex", wantCalls: 3, wantPlace: "task:init"},
		{name: "Codex_Timeout_RequeuesAfterBoundedRetries", corpusEntry: "codex_timeout_waiting_for_provider", provider: workers.ModelProviderCodex, model: "gpt-5-codex", wantCalls: 3, wantPlace: "task:init"},
		{name: "Codex_PermanentBadRequest_FailsWithoutRetry", corpusEntry: "codex_invalid_request_error", provider: workers.ModelProviderCodex, model: "gpt-5-codex", wantCalls: 1, wantPlace: "task:failed"},
		{name: "Codex_AuthFailure_FailsWithoutRetry", corpusEntry: "codex_authentication_unauthorized", provider: workers.ModelProviderCodex, model: "gpt-5-codex", wantCalls: 1, wantPlace: "task:failed"},
		{name: "Codex_Unknown_FailsWithoutRetry", provider: workers.ModelProviderCodex, model: "gpt-5-codex", wantCalls: 1, wantPlace: "task:failed"},
	}
}

// portos:func-length-exception owner=agent-factory reason=provider-long-lane-normalization-sweep review=2026-07-19 removal=split-provider-long-case-runner-before-next-provider-lane-expansion
func runProviderErrorLongCase(t *testing.T, tc providerLongCase) {
	t.Helper()

	expectedType := interfaces.ProviderErrorTypeUnknown
	expectedFamily := interfaces.ProviderErrorFamilyTerminal
	if tc.corpusEntry != "" {
		entry := providerErrorCorpusEntryForTest(t, tc.corpusEntry)
		expectedType = entry.ExpectedType
		expectedFamily = entry.ExpectedFamily
	}

	smokeHarness := testutil.NewProviderErrorSmokeHarness(
		t,
		support.LegacyFixtureDir(t, "worktree_passthrough"),
		tc.provider,
		tc.model,
		testutil.WithProviderErrorSmokeServiceOptions(
			testutil.WithExtraOptions(factory.WithProviderThrottlePauseDuration(3*time.Second)),
			testutil.WithFullWorkerPoolAndScriptWrap(),
		),
	)
	queueProviderErrorResults(t, smokeHarness, tc)

	work := testutil.ProviderErrorSmokeWork{
		Name:       strings.ToLower(strings.ReplaceAll(tc.name, "_", "-")),
		WorkID:     "work-" + strings.ToLower(strings.ReplaceAll(tc.name, "_", "-")),
		WorkTypeID: "task",
		TraceID:    "trace-" + strings.ToLower(strings.ReplaceAll(tc.name, "_", "-")),
		Payload:    []byte("provider smoke payload"),
	}
	smokeHarness.SeedWork(t, work)

	runner := smokeHarness.ProviderRunner()
	h, outcome := runProviderErrorHarness(t, smokeHarness, work, tc)
	assertProviderRunnerCalls(t, runner, tc)
	assertProviderCommandMatchesLane(t, runner.LastRequest(), tc.provider, work.Name, tc.model)
	assertProviderOutcome(t, h, outcome, work, tc, expectedType, expectedFamily)
}

func queueProviderErrorResults(t *testing.T, smokeHarness *testutil.ProviderErrorSmokeHarness, tc providerLongCase) {
	t.Helper()

	if tc.corpusEntry != "" {
		smokeHarness.QueueProviderResults(providerErrorCorpusEntryForTest(t, tc.corpusEntry).RepeatedCommandResults(tc.wantCalls)...)
		return
	}

	smokeHarness.QueueProviderResults(workers.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("some brand new " + string(tc.provider) + " failure"),
	})
}

func runProviderErrorHarness(t *testing.T, smokeHarness *testutil.ProviderErrorSmokeHarness, work testutil.ProviderErrorSmokeWork, tc providerLongCase) (*testutil.ServiceTestHarness, testutil.ProviderErrorSmokeOutcome) {
	t.Helper()

	if tc.wantPlace == "task:init" {
		h := smokeHarness.BuildRunningServiceHarness(t, 5*time.Second)
		if tc.wantThrottlePause {
			return h, smokeHarness.WaitForThrottleRequeue(t, h, work, 5*time.Second)
		}
		return h, smokeHarness.WaitForRetryableRequeue(t, h, work, 5*time.Second)
	}

	h := smokeHarness.BuildServiceHarness(t)
	h.RunUntilComplete(t, 5*time.Second)
	return h, smokeHarness.WaitForFailedAfterBoundedRetries(t, h, work, time.Second)
}

func assertProviderRunnerCalls(t *testing.T, runner *testutil.ProviderCommandRunner, tc providerLongCase) {
	t.Helper()

	if tc.wantPlace == "task:init" && !tc.wantThrottlePause {
		if runner.CallCount() < tc.wantCalls {
			t.Fatalf("provider runner called %d times, want at least %d", runner.CallCount(), tc.wantCalls)
		}
		return
	}

	if runner.CallCount() != tc.wantCalls {
		t.Fatalf("provider runner called %d times, want %d", runner.CallCount(), tc.wantCalls)
	}
}

func assertProviderOutcome(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
	tc providerLongCase,
	wantType interfaces.ProviderErrorType,
	wantFamily interfaces.ProviderErrorFamily,
) {
	t.Helper()

	switch tc.wantPlace {
	case "task:init":
		assertProviderRequeueOutcome(t, h, outcome, work, tc, wantType, wantFamily)
	case "task:failed":
		h.Assert().
			PlaceTokenCount("task:failed", 1).
			HasNoTokenInPlace("task:init").
			HasNoTokenInPlace("task:complete")
		dispatch := outcome.Dispatches[len(outcome.Dispatches)-1]
		assertDispatchHistoryMatchesWork(t, dispatch, work)
		assertDispatchProviderFailureMatchesExpected(t, dispatch, wantType, wantFamily)
	default:
		t.Fatalf("unsupported wantPlace %q", tc.wantPlace)
	}
}

func assertProviderRequeueOutcome(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
	tc providerLongCase,
	wantType interfaces.ProviderErrorType,
	wantFamily interfaces.ProviderErrorFamily,
) {
	t.Helper()

	if tc.wantThrottlePause {
		h.Assert().
			PlaceTokenCount("task:init", 1).
			HasNoTokenInPlace("task:complete").
			HasNoTokenInPlace("task:failed")
	}
	if got := outcome.Token.History.TotalVisits["process"]; got != 1 {
		t.Fatalf("TotalVisits[process] = %d, want 1", got)
	}
	if got := outcome.Token.History.ConsecutiveFailures["process"]; got != 1 {
		t.Fatalf("ConsecutiveFailures[process] = %d, want 1", got)
	}
	if got := len(outcome.Token.History.FailureLog); got != 1 {
		t.Fatalf("FailureLog length = %d, want 1", got)
	}
	if len(outcome.Dispatches) != 1 {
		t.Fatalf("DispatchHistory length = %d, want 1", len(outcome.Dispatches))
	}
	dispatch := outcome.Dispatches[0]
	if dispatch.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("DispatchHistory outcome = %s, want %s", dispatch.Outcome, interfaces.OutcomeFailed)
	}
	assertDispatchHistoryMatchesWork(t, dispatch, work)
	assertDispatchProviderFailureMatchesExpected(t, dispatch, wantType, wantFamily)
	if tc.wantThrottlePause {
		assertActiveThrottlePause(t, outcome.EngineState, tc.provider, tc.model)
		return
	}
	if len(outcome.EngineState.ActiveThrottlePauses) != 0 {
		t.Fatalf("active throttle pauses = %d, want 0", len(outcome.EngineState.ActiveThrottlePauses))
	}
}

func providerErrorCorpusEntryForTest(t *testing.T, name string) workers.ProviderErrorCorpusEntry {
	t.Helper()

	corpus, err := workers.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("workers.LoadProviderErrorCorpus() error = %v", err)
	}
	entry, ok := corpus.Entry(name)
	if !ok {
		t.Fatalf("provider error corpus entry %q not found", name)
	}
	return entry
}

func assertProviderCommandMatchesLane(t *testing.T, req workers.CommandRequest, provider workers.ModelProvider, workName, model string) {
	t.Helper()

	if req.Command != string(provider) {
		t.Fatalf("provider command = %q, want %q", req.Command, provider)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"--model", model})

	switch provider {
	case workers.ModelProviderClaude:
		support.AssertArgsContainSequence(t, req.Args, []string{"--worktree", workName})
		if len(req.Stdin) != 0 {
			t.Fatalf("expected claude prompt in args, got stdin %q", string(req.Stdin))
		}
	case workers.ModelProviderCodex:
		if got := req.Args[len(req.Args)-1]; got != "-" {
			t.Fatalf("expected codex stdin placeholder '-', got %q", got)
		}
		if len(req.Stdin) == 0 {
			t.Fatal("expected codex prompt over stdin")
		}
	default:
		t.Fatalf("unsupported provider %q", provider)
	}
}

func assertDispatchHistoryMatchesWork(t *testing.T, dispatch interfaces.CompletedDispatch, work testutil.ProviderErrorSmokeWork) {
	t.Helper()

	if len(dispatch.ConsumedTokens) == 0 {
		t.Fatal("dispatch consumed no tokens")
	}

	consumed := dispatch.ConsumedTokens[0]
	if consumed.Color.WorkID != work.WorkID {
		t.Fatalf("dispatch consumed WorkID = %q, want %q", consumed.Color.WorkID, work.WorkID)
	}
	if consumed.Color.TraceID != work.TraceID {
		t.Fatalf("dispatch consumed TraceID = %q, want %q", consumed.Color.TraceID, work.TraceID)
	}
	if consumed.Color.Name != work.Name {
		t.Fatalf("dispatch consumed Name = %q, want %q", consumed.Color.Name, work.Name)
	}
}

func assertDispatchProviderFailureMatchesExpected(
	t *testing.T,
	dispatch interfaces.CompletedDispatch,
	wantType interfaces.ProviderErrorType,
	wantFamily interfaces.ProviderErrorFamily,
) {
	t.Helper()

	if dispatch.ProviderFailure == nil {
		t.Fatal("dispatch ProviderFailure is nil")
	}
	if dispatch.ProviderFailure.Type != wantType {
		t.Fatalf("dispatch ProviderFailure.Type = %s, want %s", dispatch.ProviderFailure.Type, wantType)
	}
	if dispatch.ProviderFailure.Family != wantFamily {
		t.Fatalf("dispatch ProviderFailure.Family = %s, want %s", dispatch.ProviderFailure.Family, wantFamily)
	}
}

func assertActiveThrottlePause(
	t *testing.T,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	provider workers.ModelProvider,
	model string,
) {
	t.Helper()

	if engineState == nil {
		t.Fatal("engine state is nil")
	}
	if len(engineState.ActiveThrottlePauses) != 1 {
		t.Fatalf("active throttle pauses = %d, want 1", len(engineState.ActiveThrottlePauses))
	}
	pause := engineState.ActiveThrottlePauses[0]
	if pause.Provider != string(provider) || pause.Model != model {
		t.Fatalf("active throttle pause lane = %s/%s, want %s/%s", pause.Provider, pause.Model, provider, model)
	}
	if pause.LaneID != string(provider)+"/"+model {
		t.Fatalf("active throttle pause LaneID = %q, want %q", pause.LaneID, string(provider)+"/"+model)
	}
	if pause.PausedAt.IsZero() {
		t.Fatal("active throttle pause PausedAt is zero")
	}
	if !pause.PausedUntil.After(pause.PausedAt) {
		t.Fatalf("active throttle pause PausedUntil = %s, want after PausedAt %s", pause.PausedUntil, pause.PausedAt)
	}
}

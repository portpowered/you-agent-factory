package functional_test

import (
	"context"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type providerSmokeCase struct {
	name              string
	corpusEntry       string
	provider          workers.ModelProvider
	model             string
	wantCalls         int
	wantPlace         string
	wantThrottlePause bool
}

// portos:func-length-exception owner=agent-factory reason=legacy-provider-error-scenario-table review=2026-07-19 removal=split-provider-scenario-cases-and-normalization-assertions-before-next-provider-error-smoke-change
func TestProviderErrorSmoke_ScriptWrapScenariosStayNormalizedAcrossProviders(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow provider normalization smoke")
	testCases := []providerSmokeCase{
		{
			name:              "Claude_Throttled_RequeuesAfterBoundedRetries",
			corpusEntry:       "claude_rate_limit_error",
			provider:          workers.ModelProviderClaude,
			model:             "claude-sonnet-4-5-20250514",
			wantCalls:         3,
			wantPlace:         "task:init",
			wantThrottlePause: true,
		},
		{
			name:        "Claude_TransientServerError_RequeuesAfterBoundedRetries",
			corpusEntry: "claude_internal_server_api_error",
			provider:    workers.ModelProviderClaude,
			model:       "claude-sonnet-4-5-20250514",
			wantCalls:   3,
			wantPlace:   "task:init",
		},
		{
			name:        "Claude_Timeout_RequeuesAfterBoundedRetries",
			corpusEntry: "claude_timeout_waiting_for_provider",
			provider:    workers.ModelProviderClaude,
			model:       "claude-sonnet-4-5-20250514",
			wantCalls:   3,
			wantPlace:   "task:init",
		},
		{
			name:        "Claude_PermanentBadRequest_FailsWithoutRetry",
			corpusEntry: "claude_invalid_request_error",
			provider:    workers.ModelProviderClaude,
			model:       "claude-sonnet-4-5-20250514",
			wantCalls:   1,
			wantPlace:   "task:failed",
		},
		{
			name:      "Claude_Unknown_FailsWithoutRetry",
			provider:  workers.ModelProviderClaude,
			model:     "claude-sonnet-4-5-20250514",
			wantCalls: 1,
			wantPlace: "task:failed",
		},
		{
			name:              "Codex_Throttled_RequeuesAfterBoundedRetries",
			corpusEntry:       "codex_status_429_too_many_requests",
			provider:          workers.ModelProviderCodex,
			model:             "gpt-5-codex",
			wantCalls:         3,
			wantPlace:         "task:init",
			wantThrottlePause: true,
		},
		{
			name:        "Codex_TransientServerError_RequeuesAfterBoundedRetries",
			corpusEntry: "codex_internal_server_status_500",
			provider:    workers.ModelProviderCodex,
			model:       "gpt-5-codex",
			wantCalls:   3,
			wantPlace:   "task:init",
		},
		{
			name:        "Codex_HighDemandTemporaryServerError_RequeuesWithoutThrottlePause",
			corpusEntry: "codex_high_demand_temporary_errors",
			provider:    workers.ModelProviderCodex,
			model:       "gpt-5-codex",
			wantCalls:   3,
			wantPlace:   "task:init",
		},
		{
			name:        "Codex_Timeout_RequeuesAfterBoundedRetries",
			corpusEntry: "codex_timeout_waiting_for_provider",
			provider:    workers.ModelProviderCodex,
			model:       "gpt-5-codex",
			wantCalls:   3,
			wantPlace:   "task:init",
		},
		{
			name:        "Codex_PermanentBadRequest_FailsWithoutRetry",
			corpusEntry: "codex_invalid_request_error",
			provider:    workers.ModelProviderCodex,
			model:       "gpt-5-codex",
			wantCalls:   1,
			wantPlace:   "task:failed",
		},
		{
			name:        "Codex_AuthFailure_FailsWithoutRetry",
			corpusEntry: "codex_authentication_unauthorized",
			provider:    workers.ModelProviderCodex,
			model:       "gpt-5-codex",
			wantCalls:   1,
			wantPlace:   "task:failed",
		},
		{
			name:      "Codex_Unknown_FailsWithoutRetry",
			provider:  workers.ModelProviderCodex,
			model:     "gpt-5-codex",
			wantCalls: 1,
			wantPlace: "task:failed",
		},
	}

	for _, tc := range testCases {
		subtestName := tc.name
		if tc.corpusEntry != "" {
			subtestName += "_" + tc.corpusEntry
		}
		t.Run(subtestName, func(t *testing.T) {
			runProviderErrorSmokeCase(t, tc)
		})
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-provider-throttle-isolation-smoke review=2026-07-19 removal=split-pause-harness-setup-submissions-and-lane-assertions-before-next-provider-error-smoke-change
func TestProviderErrorSmoke_ThrottlePauseOnlyBlocksTheAffectedProviderModelLane(t *testing.T) {
	pauseHarness := testutil.NewProviderErrorSmokePauseIsolationHarness(
		t,
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "claude-task",
			WorkerName:      "claude-worker",
			WorkstationName: "process-claude",
			Provider:        workers.ModelProviderClaude,
			Model:           "claude-sonnet-4-5-20250514",
			PromptBody:      "Process the Claude lane task.\n",
		},
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "codex-task",
			WorkerName:      "codex-worker",
			WorkstationName: "process-codex",
			Provider:        workers.ModelProviderCodex,
			Model:           "gpt-5-codex",
			PromptBody:      "Process the Codex lane task.\n",
		},
		testutil.WithProviderErrorSmokePauseIsolationServiceOptions(
			testutil.WithExtraOptions(factory.WithProviderThrottlePauseDuration(3*time.Second)),
			testutil.WithFullWorkerPoolAndScriptWrap(),
		),
	)
	runner := pauseHarness.ProviderRunner()
	pauseHarness.QueueProviderResults(
		providerErrorCorpusEntryForTest(t, "claude_rate_limit_error").RepeatedCommandResults(3)...,
	)
	pauseHarness.QueueProviderResults(
		workers.CommandResult{Stdout: []byte("codex lane recovered. COMPLETE")},
	)

	throttledWork := testutil.ProviderErrorSmokeWork{
		Name:       "claude-throttle-lane",
		WorkID:     "work-claude-throttle-lane",
		WorkTypeID: "claude-task",
		TraceID:    "trace-claude-throttle-lane",
		Payload:    []byte("claude throttle payload"),
	}
	unaffectedWork := testutil.ProviderErrorSmokeWork{
		Name:       "codex-healthy-lane",
		WorkID:     "work-codex-healthy-lane",
		WorkTypeID: "codex-task",
		TraceID:    "trace-codex-healthy-lane",
		Payload:    []byte("codex healthy payload"),
	}
	pauseHarness.SeedWork(t, throttledWork)

	h := pauseHarness.BuildRunningServiceHarness(t, 5*time.Second)

	pauseHarness.WaitForThrottleRequeue(t, h, throttledWork, 5*time.Second)
	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-" + unaffectedWork.Name,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       unaffectedWork.Name,
			WorkID:     unaffectedWork.WorkID,
			WorkTypeID: unaffectedWork.WorkTypeID,
			TraceID:    unaffectedWork.TraceID,
			Payload:    unaffectedWork.Payload,
		}},
	})

	outcome := pauseHarness.WaitForPauseIsolation(t, h, throttledWork, unaffectedWork, 5*time.Second)

	requests := runner.Requests()
	if len(requests) != 4 {
		t.Fatalf("provider command count = %d, want 4", len(requests))
	}
	for i := 0; i < 3; i++ {
		if requests[i].Command != string(workers.ModelProviderClaude) {
			t.Fatalf("request %d command = %q, want %q", i, requests[i].Command, workers.ModelProviderClaude)
		}
	}
	if requests[3].Command != string(workers.ModelProviderCodex) {
		t.Fatalf("request 3 command = %q, want %q", requests[3].Command, workers.ModelProviderCodex)
	}
	assertDispatchHistoryMatchesWork(t, outcome.ThrottledLane.Dispatches[0], throttledWork)
	assertDispatchHistoryMatchesWork(
		t,
		outcome.UnaffectedLane.Dispatches[len(outcome.UnaffectedLane.Dispatches)-1],
		unaffectedWork,
	)
}

// portos:func-length-exception owner=agent-factory reason=legacy-worker-pool-provider-error-smoke review=2026-07-19 removal=split-codex-timeout-and-worker-pool-assertions-before-next-provider-error-smoke-change
func TestProviderErrorSmoke_CodexAndTimeoutFailuresNormalizeThroughWorkerPool(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow provider worker-pool smoke")
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	t.Run(providerErrorCorpusEntryLabel(capacityEntry)+"_BoundedErrorLine_RequeuesWithConciseFailureReason", func(t *testing.T) {
		conciseError := providerErrorCorpusLastErrorLine(t, capacityEntry)
		const transcriptMarker = "full inference transcript should not be retained"
		rawOutput := strings.Join([]string{
			"OpenAI Codex v0.118.0 (research preview)",
			transcriptMarker,
			strings.Repeat("reasoning transcript ", 200),
			conciseError,
			"cleanup complete",
		}, "\n")

		smokeHarness := testutil.NewProviderErrorSmokeHarness(
			t,
			fixtureDir(t, "worktree_passthrough"),
			workers.ModelProviderCodex,
			"gpt-5-codex",
			testutil.WithProviderErrorSmokeServiceOptions(
				testutil.WithExtraOptions(factory.WithProviderThrottlePauseDuration(3*time.Second)),
				testutil.WithFullWorkerPoolAndScriptWrap(),
			),
		)
		smokeHarness.QueueProviderResults(
			workers.CommandResult{ExitCode: capacityEntry.ExitCode, Stdout: []byte(rawOutput)},
			workers.CommandResult{ExitCode: capacityEntry.ExitCode, Stdout: []byte(rawOutput)},
			workers.CommandResult{ExitCode: capacityEntry.ExitCode, Stdout: []byte(rawOutput)},
		)
		work := testutil.ProviderErrorSmokeWork{
			Name:       "codex-late-error-line",
			WorkID:     "work-codex-late-error-line",
			WorkTypeID: "task",
			TraceID:    "trace-codex-late-error-line",
			Payload:    []byte("codex late error smoke payload"),
		}
		smokeHarness.SeedWork(t, work)

		h := smokeHarness.BuildRunningServiceHarness(t, 5*time.Second)
		outcome := smokeHarness.WaitForThrottleRequeue(t, h, work, 5*time.Second)

		if smokeHarness.ProviderRunner().CallCount() < 3 {
			t.Fatalf("provider command count = %d, want at least 3", smokeHarness.ProviderRunner().CallCount())
		}
		if len(outcome.Dispatches) != 1 {
			t.Fatalf("DispatchHistory length = %d, want 1", len(outcome.Dispatches))
		}
		dispatch := outcome.Dispatches[0]
		if dispatch.ProviderFailure == nil {
			t.Fatal("ProviderFailure is nil, want throttled metadata")
		}
		if dispatch.ProviderFailure.Type != capacityEntry.ExpectedType {
			t.Fatalf("provider failure type = %s, want %s", dispatch.ProviderFailure.Type, capacityEntry.ExpectedType)
		}
		assertContainsAll(t, dispatch.Reason, []string{"provider error: " + string(capacityEntry.ExpectedType), conciseError})
		if strings.Contains(dispatch.Reason, transcriptMarker) {
			t.Fatalf("dispatch reason retained raw transcript marker: %q", dispatch.Reason)
		}
	})

	t.Run("CodexTimeoutLine_RetriesAndRecordsRetryableTimeout", func(t *testing.T) {
		const conciseError = "ERROR: command timed out while waiting for codex"
		const transcriptMarker = "timeout transcript should not be retained"
		rawOutput := strings.Join([]string{
			"OpenAI Codex v0.118.0 (research preview)",
			transcriptMarker,
			conciseError,
			"post-error diagnostics",
		}, "\n")

		smokeHarness := testutil.NewProviderErrorSmokeHarness(
			t,
			fixtureDir(t, "worktree_passthrough"),
			workers.ModelProviderCodex,
			"gpt-5-codex",
			testutil.WithProviderErrorSmokeServiceOptions(testutil.WithFullWorkerPoolAndScriptWrap()),
		)
		smokeHarness.QueueProviderResults(
			workers.CommandResult{ExitCode: 1, Stderr: []byte(rawOutput)},
			workers.CommandResult{ExitCode: 1, Stderr: []byte(rawOutput)},
			workers.CommandResult{ExitCode: 1, Stderr: []byte(rawOutput)},
		)
		work := testutil.ProviderErrorSmokeWork{
			Name:       "codex-timeout-line",
			WorkID:     "work-codex-timeout-line",
			WorkTypeID: "task",
			TraceID:    "trace-codex-timeout-line",
			Payload:    []byte("codex timeout smoke payload"),
		}
		smokeHarness.SeedWork(t, work)

		h := smokeHarness.BuildRunningServiceHarness(t, 5*time.Second)
		outcome := smokeHarness.WaitForRetryableRequeue(t, h, work, 5*time.Second)

		if smokeHarness.ProviderRunner().CallCount() < 3 {
			t.Fatalf("provider command count = %d, want at least 3", smokeHarness.ProviderRunner().CallCount())
		}
		if len(outcome.Dispatches) != 1 {
			t.Fatalf("DispatchHistory length = %d, want 1", len(outcome.Dispatches))
		}
		dispatch := outcome.Dispatches[0]
		if dispatch.ProviderFailure == nil {
			t.Fatal("ProviderFailure is nil, want timeout metadata")
		}
		if dispatch.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
			t.Fatalf("provider failure type = %s, want %s", dispatch.ProviderFailure.Type, interfaces.ProviderErrorTypeTimeout)
		}
		if dispatch.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable {
			t.Fatalf("provider failure family = %s, want %s", dispatch.ProviderFailure.Family, interfaces.ProviderErrorFamilyRetryable)
		}
		assertContainsAll(t, dispatch.Reason, []string{"provider error: timeout", conciseError})
		if strings.Contains(dispatch.Reason, transcriptMarker) {
			t.Fatalf("dispatch reason retained raw transcript marker: %q", dispatch.Reason)
		}
	})
}

func TestProviderErrorSmoke_CodexTemporaryServerErrorsRequeueWithoutThrottlePause(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow codex retry smoke")
	testCases := []struct {
		name     string
		workName string
		stderr   string
	}{
		{
			name:     "HighDemandMessage_RequeuesThroughRetryablePath",
			workName: "codex-high-demand-requeue",
			stderr:   `ERROR: We're currently experiencing high demand, which may cause temporary errors.`,
		},
		{
			name:     "UnexpectedStatus500_RequeuesThroughRetryablePath",
			workName: "codex-unexpected-status-500-requeue",
			stderr:   `ERROR: unexpected status 500 Internal Server Error`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			smokeHarness := testutil.NewProviderErrorSmokeHarness(
				t,
				fixtureDir(t, "worktree_passthrough"),
				workers.ModelProviderCodex,
				"gpt-5-codex",
				testutil.WithProviderErrorSmokeServiceOptions(testutil.WithFullWorkerPoolAndScriptWrap()),
			)
			smokeHarness.QueueProviderResults(
				providerErrorCommandFailure(tc.stderr),
				providerErrorCommandFailure(tc.stderr),
				providerErrorCommandFailure(tc.stderr),
			)
			work := testutil.ProviderErrorSmokeWork{
				Name:       tc.workName,
				WorkID:     "work-" + tc.workName,
				WorkTypeID: "task",
				TraceID:    "trace-" + tc.workName,
				Payload:    []byte("codex temporary server error payload"),
			}
			smokeHarness.SeedWork(t, work)

			h := smokeHarness.BuildRunningServiceHarness(t, 5*time.Second)
			outcome := smokeHarness.WaitForRetryableRequeue(t, h, work, 5*time.Second)

			assertRetryableInternalServerRequeueOutcome(t, smokeHarness.ProviderRunner(), outcome, work)
		})
	}
}

func TestProviderErrorSmoke_CodexWindowsExitCode4294967295RequeuesAndSurfacesRetryableProviderFailureMetadata(t *testing.T) {
	entry := providerErrorCorpusEntryForTest(t, "codex_windows_exit_code_4294967295")
	smokeHarness := testutil.NewProviderErrorSmokeHarness(
		t,
		fixtureDir(t, "worktree_passthrough"),
		workers.ModelProviderCodex,
		"gpt-5-codex",
		testutil.WithProviderErrorSmokeServiceOptions(testutil.WithFullWorkerPoolAndScriptWrap()),
	)
	smokeHarness.QueueProviderResults(entry.RepeatedCommandResults(3)...)
	work := providerErrorSmokeWork(
		"codex-windows-exit-code-4294967295",
		"codex windows retryable provider failure payload",
	)
	smokeHarness.SeedWork(t, work)

	h := smokeHarness.BuildRunningServiceHarness(t, 5*time.Second)
	outcome := smokeHarness.WaitForRetryableRequeue(t, h, work, 5*time.Second)

	assertRetryableInternalServerRequeueOutcome(t, smokeHarness.ProviderRunner(), outcome, work)

	dispatch := outcome.Dispatches[0]
	assertContainsAll(t, dispatch.Reason, []string{"provider error: internal_server_error", "4294967295"})
	assertNoAuthRemediationText(t, dispatch.Reason)

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}

	completion := requireProviderErrorDispatchCompletedEventForWork(t, events, work.WorkID)
	if completion.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("DISPATCH_COMPLETED outcome = %s, want %s", completion.Outcome, factoryapi.WorkOutcomeFailed)
	}
	if got := stringPointerValue(completion.FailureReason); got != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("DISPATCH_COMPLETED failureReason = %q, want %q", got, interfaces.ProviderErrorTypeInternalServerError)
	}
	if completion.ProviderFailure == nil {
		t.Fatal("DISPATCH_COMPLETED providerFailure is nil, want canonical metadata")
	}
	if got := stringPointerValue(completion.ProviderFailure.Type); got != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("DISPATCH_COMPLETED providerFailure.type = %q, want %q", got, interfaces.ProviderErrorTypeInternalServerError)
	}
	if got := stringPointerValue(completion.ProviderFailure.Family); got != string(interfaces.ProviderErrorFamilyRetryable) {
		t.Fatalf("DISPATCH_COMPLETED providerFailure.family = %q, want %q", got, interfaces.ProviderErrorFamilyRetryable)
	}
	assertContainsAll(
		t,
		stringPointerValue(completion.FailureMessage),
		[]string{"internal_server_error", "4294967295"},
	)
	assertNoAuthRemediationText(t, stringPointerValue(completion.FailureMessage))
}

func TestProviderErrorSmoke_CodexHighDemandPersistentFailureFailsOnlyAfterGuardedLoopBreakerThreshold(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow guarded loop-breaker provider smoke")
	smokeHarness := newCodexHighDemandLoopBreakerHarness(t)
	work := providerErrorSmokeWork(
		"codex-high-demand-loop-breaker",
		"codex persistent high demand payload",
	)
	smokeHarness.SeedWork(t, work)

	h := smokeHarness.BuildServiceHarness(t)
	h.RunUntilComplete(t, 10*time.Second)

	outcome := smokeHarness.WaitForFailedAfterBoundedRetries(t, h, work, time.Second)
	assertCodexHighDemandLoopBreakerOutcome(t, h, smokeHarness.ProviderRunner(), outcome, work)
}

// portos:func-length-exception owner=agent-factory reason=legacy-provider-throttle-observability-smoke review=2026-07-19 removal=split-runtime-snapshot-dashboard-and-event-assertions-before-next-provider-error-smoke-change
func TestProviderErrorSmoke_ThrottlePauseObservabilityFlowsThroughRuntimeSnapshotAndDashboard(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow throttle-pause observability smoke")
	pauseDuration := 2 * time.Second
	pauseHarness := testutil.NewProviderErrorSmokePauseIsolationHarness(
		t,
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "claude-task",
			WorkerName:      "claude-worker",
			WorkstationName: "process-claude",
			Provider:        workers.ModelProviderClaude,
			Model:           "claude-sonnet-4-5-20250514",
			PromptBody:      "Process the Claude lane task.\n",
		},
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "codex-task",
			WorkerName:      "codex-worker",
			WorkstationName: "process-codex",
			Provider:        workers.ModelProviderCodex,
			Model:           "gpt-5-codex",
			PromptBody:      "Process the Codex lane task.\n",
		},
	)
	runner := pauseHarness.ProviderRunner()
	pauseHarness.QueueProviderResults(
		providerErrorCorpusEntryForTest(t, "claude_rate_limit_error").RepeatedCommandResults(3)...,
	)
	pauseHarness.QueueProviderResults(
		workers.CommandResult{Stdout: []byte("codex lane completed while claude was paused. COMPLETE")},
		workers.CommandResult{Stdout: []byte("claude lane recovered after pause expiry. COMPLETE")},
		workers.CommandResult{Stdout: []byte("codex reconciliation lane completed. COMPLETE")},
	)

	throttledWork := testutil.ProviderErrorSmokeWork{
		Name:       "claude-observable-throttle-lane",
		WorkID:     "work-claude-observable-throttle-lane",
		WorkTypeID: "claude-task",
		TraceID:    "trace-claude-observable-throttle-lane",
		Payload:    []byte("claude observable throttle payload"),
	}
	unaffectedWork := testutil.ProviderErrorSmokeWork{
		Name:       "codex-observable-healthy-lane",
		WorkID:     "work-codex-observable-healthy-lane",
		WorkTypeID: "codex-task",
		TraceID:    "trace-codex-observable-healthy-lane",
		Payload:    []byte("codex observable healthy payload"),
	}
	reconcileWork := testutil.ProviderErrorSmokeWork{
		Name:       "codex-reconcile-after-pause-expiry",
		WorkID:     "work-codex-reconcile-after-pause-expiry",
		WorkTypeID: "codex-task",
		TraceID:    "trace-codex-reconcile-after-pause-expiry",
		Payload:    []byte("codex reconciliation payload"),
	}
	pauseHarness.SeedWork(t, throttledWork)

	server := StartFunctionalServerWithConfig(
		t,
		pauseHarness.Dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.ProviderCommandRunnerOverride = runner
		},
		factory.WithServiceMode(),
		factory.WithProviderThrottlePauseDuration(pauseDuration),
	)

	activeEngineState := waitForEngineStateSnapshot(
		t,
		server,
		10*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return len(snapshot.ActiveThrottlePauses) == 1 &&
				hasWorkTokenInPlace(snapshot.Marking, throttledWork.WorkTypeID+":init", throttledWork.WorkID)
		},
	)
	assertActiveThrottlePause(t, activeEngineState, workers.ModelProviderClaude, "claude-sonnet-4-5-20250514")

	activeDashboard := server.GetDashboard(t)
	assertDashboardThrottlePausesMatchEngineState(t, "active pause dashboard", activeEngineState, activeDashboard)

	server.SubmitRuntimeWork(t, interfaces.SubmitRequest{
		Name:       unaffectedWork.Name,
		WorkID:     unaffectedWork.WorkID,
		WorkTypeID: unaffectedWork.WorkTypeID,
		TraceID:    unaffectedWork.TraceID,
		Payload:    unaffectedWork.Payload,
	})

	isolatedEngineState := waitForEngineStateSnapshot(
		t,
		server,
		5*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return len(snapshot.ActiveThrottlePauses) == 1 &&
				hasWorkTokenInPlace(snapshot.Marking, throttledWork.WorkTypeID+":init", throttledWork.WorkID) &&
				hasWorkTokenInPlace(snapshot.Marking, unaffectedWork.WorkTypeID+":complete", unaffectedWork.WorkID)
		},
	)
	assertDashboardThrottlePausesMatchEngineState(t, "pause isolation dashboard", isolatedEngineState, server.GetDashboard(t))

	if wait := time.Until(activeEngineState.ActiveThrottlePauses[0].PausedUntil.Add(100 * time.Millisecond)); wait > 0 {
		time.Sleep(wait)
	}
	server.SubmitRuntimeWork(t, interfaces.SubmitRequest{
		Name:       reconcileWork.Name,
		WorkID:     reconcileWork.WorkID,
		WorkTypeID: reconcileWork.WorkTypeID,
		TraceID:    reconcileWork.TraceID,
		Payload:    reconcileWork.Payload,
	})

	recoveredEngineState := waitForEngineStateSnapshot(
		t,
		server,
		10*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return len(snapshot.ActiveThrottlePauses) == 0 &&
				hasWorkTokenInPlace(snapshot.Marking, throttledWork.WorkTypeID+":complete", throttledWork.WorkID) &&
				hasWorkTokenInPlace(snapshot.Marking, unaffectedWork.WorkTypeID+":complete", unaffectedWork.WorkID) &&
				hasWorkTokenInPlace(snapshot.Marking, reconcileWork.WorkTypeID+":complete", reconcileWork.WorkID)
		},
	)
	recoveredDashboard := waitForDashboardSnapshot(
		t,
		5*time.Second,
		func() (DashboardResponse, bool) {
			dashboard := server.GetDashboard(t)
			return dashboard, len(sliceValue(dashboard.Runtime.ActiveThrottlePauses)) == 0 &&
				dashboard.Runtime.InFlightDispatchCount == 0 &&
				dashboard.Runtime.Session.CompletedCount >= 3
		},
	)
	assertDashboardMatchesEngineState(t, "recovered dashboard", recoveredEngineState, recoveredDashboard)

	requests := runner.Requests()
	if len(requests) < 4 {
		t.Fatalf("provider command count = %d, want at least 4", len(requests))
	}
	for i := 0; i < 3; i++ {
		if requests[i].Command != string(workers.ModelProviderClaude) {
			t.Fatalf("request %d command = %q, want %q", i, requests[i].Command, workers.ModelProviderClaude)
		}
	}
	if requests[3].Command != string(workers.ModelProviderCodex) {
		t.Fatalf("request 3 command = %q, want %q", requests[3].Command, workers.ModelProviderCodex)
	}

	throttledDispatches := dispatchesForProviderSmokeWork(recoveredEngineState.DispatchHistory, throttledWork)
	unaffectedDispatches := dispatchesForProviderSmokeWork(recoveredEngineState.DispatchHistory, unaffectedWork)
	if len(throttledDispatches) == 0 {
		t.Fatal("throttled lane dispatch count = 0, want at least one failed dispatch")
	}
	if len(unaffectedDispatches) != 1 {
		t.Fatalf("unaffected lane dispatch count = %d, want 1", len(unaffectedDispatches))
	}
	if throttledDispatches[0].Outcome != interfaces.OutcomeFailed {
		t.Fatalf("first throttled dispatch outcome = %s, want %s", throttledDispatches[0].Outcome, interfaces.OutcomeFailed)
	}
	if len(throttledDispatches) > 1 && throttledDispatches[1].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("second throttled dispatch outcome = %s, want %s", throttledDispatches[1].Outcome, interfaces.OutcomeAccepted)
	}
	if unaffectedDispatches[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("unaffected dispatch outcome = %s, want %s", unaffectedDispatches[0].Outcome, interfaces.OutcomeAccepted)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-provider-error-case-fixture review=2026-07-19 removal=split-smoke-harness-run-provider-assertions-and-event-assertions-before-next-provider-error-smoke-change
func runProviderErrorSmokeCase(t *testing.T, tc providerSmokeCase) {
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
		fixtureDir(t, "worktree_passthrough"),
		tc.provider,
		tc.model,
		testutil.WithProviderErrorSmokeServiceOptions(
			testutil.WithExtraOptions(factory.WithProviderThrottlePauseDuration(3*time.Second)),
			testutil.WithFullWorkerPoolAndScriptWrap(),
		),
	)
	if tc.corpusEntry != "" {
		smokeHarness.QueueProviderResults(providerErrorCorpusEntryForTest(t, tc.corpusEntry).RepeatedCommandResults(tc.wantCalls)...)
	} else {
		smokeHarness.QueueProviderResults(workers.CommandResult{
			ExitCode: 1,
			Stderr:   []byte("some brand new " + string(tc.provider) + " failure"),
		})
	}
	runner := smokeHarness.ProviderRunner()

	workName := strings.ToLower(strings.ReplaceAll(tc.name, "_", "-"))
	work := testutil.ProviderErrorSmokeWork{
		Name:       workName,
		WorkID:     "work-" + workName,
		WorkTypeID: "task",
		TraceID:    "trace-" + workName,
		Payload:    []byte("provider smoke payload"),
	}
	smokeHarness.SeedWork(t, work)

	var h *testutil.ServiceTestHarness
	var outcome testutil.ProviderErrorSmokeOutcome
	if tc.wantPlace == "task:init" {
		h = smokeHarness.BuildRunningServiceHarness(t, 5*time.Second)
		if tc.wantThrottlePause {
			outcome = smokeHarness.WaitForThrottleRequeue(t, h, work, 5*time.Second)
		} else {
			outcome = smokeHarness.WaitForRetryableRequeue(t, h, work, 5*time.Second)
		}
	} else {
		h = smokeHarness.BuildServiceHarness(t)
		h.RunUntilComplete(t, 5*time.Second)
		outcome = smokeHarness.WaitForFailedAfterBoundedRetries(t, h, work, time.Second)
	}

	if tc.wantPlace == "task:init" && !tc.wantThrottlePause {
		if runner.CallCount() < tc.wantCalls {
			t.Fatalf("provider runner called %d times, want at least %d", runner.CallCount(), tc.wantCalls)
		}
	} else if runner.CallCount() != tc.wantCalls {
		t.Fatalf("provider runner called %d times, want %d", runner.CallCount(), tc.wantCalls)
	}

	assertProviderCommandMatchesLane(t, runner.LastRequest(), tc.provider, workName, tc.model)

	switch tc.wantPlace {
	case "task:init":
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
		if len(outcome.Token.History.FailureLog) != 1 {
			t.Fatalf("FailureLog length = %d, want 1", len(outcome.Token.History.FailureLog))
		}
		// assertContainsAll(t, outcome.Token.History.LastError, tc.wantErrorContains)
		if len(outcome.Dispatches) != 1 {
			t.Fatalf("DispatchHistory length = %d, want 1", len(outcome.Dispatches))
		}
		dispatch := outcome.Dispatches[0]
		if dispatch.Outcome != interfaces.OutcomeFailed {
			t.Fatalf("DispatchHistory outcome = %s, want %s", dispatch.Outcome, interfaces.OutcomeFailed)
		}
		assertDispatchHistoryMatchesWork(t, dispatch, work)
		assertDispatchProviderFailureMatchesExpected(t, dispatch, expectedType, expectedFamily)
		if tc.wantThrottlePause {
			assertActiveThrottlePause(t, outcome.EngineState, tc.provider, tc.model)
		} else if len(outcome.EngineState.ActiveThrottlePauses) != 0 {
			t.Fatalf("active throttle pauses = %d, want 0", len(outcome.EngineState.ActiveThrottlePauses))
		}
	case "task:failed":
		h.Assert().
			PlaceTokenCount("task:failed", 1).
			HasNoTokenInPlace("task:init").
			HasNoTokenInPlace("task:complete")

		// assertContainsAll(t, outcome.Token.History.LastError, tc.wantErrorContains)
		if len(outcome.Dispatches) == 0 {
			t.Fatal("DispatchHistory is empty, want at least 1 entry")
		}
		dispatch := outcome.Dispatches[len(outcome.Dispatches)-1]
		assertDispatchHistoryMatchesWork(t, dispatch, work)
		assertDispatchProviderFailureMatchesExpected(t, dispatch, expectedType, expectedFamily)
	default:
		t.Fatalf("unsupported wantPlace %q", tc.wantPlace)
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

func assertProviderCommandMatchesLane(t *testing.T, req workers.CommandRequest, provider workers.ModelProvider, workName, model string) {
	t.Helper()

	if req.Command != string(provider) {
		t.Fatalf("provider command = %q, want %q", req.Command, provider)
	}
	assertArgsContainSequence(t, req.Args, []string{"--model", model})

	switch provider {
	case workers.ModelProviderClaude:
		assertArgsContainSequence(t, req.Args, []string{"--worktree", workName})
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

func dispatchesForProviderSmokeWork(
	history []interfaces.CompletedDispatch,
	work testutil.ProviderErrorSmokeWork,
) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.WorkID == work.WorkID {
				dispatches = append(dispatches, dispatch)
				break
			}
		}
	}
	return dispatches
}

func newCodexHighDemandLoopBreakerHarness(t *testing.T) *testutil.ProviderErrorSmokeHarness {
	t.Helper()

	smokeHarness := testutil.NewProviderErrorSmokeHarness(
		t,
		fixtureDir(t, "worktree_passthrough"),
		workers.ModelProviderCodex,
		"gpt-5-codex",
		testutil.WithProviderErrorSmokeServiceOptions(testutil.WithFullWorkerPoolAndScriptWrap()),
	)
	configureProviderErrorLoopBreaker(t, smokeHarness.Dir)

	const highDemandMessage = `ERROR: We're currently experiencing high demand, which may cause temporary errors.`
	for range 6 {
		smokeHarness.QueueProviderResults(providerErrorCommandFailure(highDemandMessage))
	}

	return smokeHarness
}

func configureProviderErrorLoopBreaker(t *testing.T, dir string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		workstations, ok := cfg["workstations"].([]any)
		if !ok {
			t.Fatalf("factory.json workstations = %T, want []any", cfg["workstations"])
		}
		cfg["workstations"] = append(workstations, map[string]any{
			"name": "provider-error-loop-breaker",
			"type": "LOGICAL_MOVE",
			"inputs": []any{
				map[string]any{
					"workType": "task",
					"state":    "init",
				},
			},
			"outputs": []any{
				map[string]any{
					"workType": "task",
					"state":    "failed",
				},
			},
			"guards": []any{
				map[string]any{
					"type":        "visit_count",
					"workstation": "process",
					"maxVisits":   2,
				},
			},
		})
	})
}

func providerErrorSmokeWork(name, payload string) testutil.ProviderErrorSmokeWork {
	return testutil.ProviderErrorSmokeWork{
		Name:       name,
		WorkID:     "work-" + name,
		WorkTypeID: "task",
		TraceID:    "trace-" + name,
		Payload:    []byte(payload),
	}
}

func assertCodexHighDemandLoopBreakerOutcome(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	runner *testutil.ProviderCommandRunner,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
) {
	t.Helper()

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	if runner.CallCount() != 6 {
		t.Fatalf("provider command count = %d, want 6 across two exhausted dispatches", runner.CallCount())
	}
	if len(outcome.EngineState.ActiveThrottlePauses) != 0 {
		t.Fatalf("active throttle pauses = %d, want 0", len(outcome.EngineState.ActiveThrottlePauses))
	}

	assertProviderErrorLoopBreakerHistory(t, outcome, work)
	assertRetryableInternalServerRequeueDispatch(t, outcome.Dispatches[0], work, "first")
	assertRetryableInternalServerRequeueDispatch(t, outcome.Dispatches[1], work, "second")
}

func assertProviderErrorLoopBreakerHistory(
	t *testing.T,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
) {
	t.Helper()

	if got := outcome.Token.History.TotalVisits["process"]; got != 2 {
		t.Fatalf("TotalVisits[process] = %d, want 2 after bounded retry exhaustion", got)
	}
	if got := outcome.Token.History.ConsecutiveFailures["process"]; got != 2 {
		t.Fatalf("ConsecutiveFailures[process] = %d, want 2 after bounded retry exhaustion", got)
	}
	if got := len(outcome.Token.History.FailureLog); got != 2 {
		t.Fatalf("FailureLog length = %d, want 2 after bounded retry exhaustion", got)
	}
	if len(outcome.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want 2 failed provider dispatches plus 1 guarded loop-breaker dispatch", len(outcome.Dispatches))
	}
	loopBreaker := outcome.Dispatches[len(outcome.Dispatches)-1]
	if loopBreaker.WorkstationName != "provider-error-loop-breaker" {
		t.Fatalf("final workstation = %q, want provider-error-loop-breaker", loopBreaker.WorkstationName)
	}
	if loopBreaker.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("final loop-breaker outcome = %s, want %s", loopBreaker.Outcome, interfaces.OutcomeAccepted)
	}
	if !dispatchHasOutputMutationToPlace(loopBreaker, work.WorkTypeID+":failed", work.WorkID) {
		t.Fatalf("final loop-breaker mutations = %#v, want route to %s:failed", loopBreaker.OutputMutations, work.WorkTypeID)
	}
}

func assertRetryableInternalServerRequeueDispatch(
	t *testing.T,
	dispatch interfaces.CompletedDispatch,
	work testutil.ProviderErrorSmokeWork,
	dispatchName string,
) {
	t.Helper()

	assertDispatchHistoryMatchesWork(t, dispatch, work)
	assertProviderFailureIsRetryableInternalServer(t, dispatch.ProviderFailure)
	if !dispatchHasOutputMutationToPlace(dispatch, work.WorkTypeID+":init", work.WorkID) {
		t.Fatalf(
			"%s dispatch mutations = %#v, want retryable requeue to %s:init",
			dispatchName,
			dispatch.OutputMutations,
			work.WorkTypeID,
		)
	}
}

func assertDashboardThrottlePausesMatchEngineState(
	t *testing.T,
	label string,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	dashboard DashboardResponse,
) {
	t.Helper()

	pauses := sliceValue(dashboard.Runtime.ActiveThrottlePauses)
	if len(pauses) != len(engineState.ActiveThrottlePauses) {
		t.Fatalf(
			"%s active throttle pause count = %d, want engine-state count %d",
			label,
			len(pauses),
			len(engineState.ActiveThrottlePauses),
		)
	}
	for i, pause := range pauses {
		enginePause := engineState.ActiveThrottlePauses[i]
		if pause.LaneId != enginePause.LaneID || pause.Provider != enginePause.Provider || pause.Model != enginePause.Model {
			t.Fatalf("%s pause[%d] identity = %#v, want engine pause %#v", label, i, pause, enginePause)
		}
		if pause.PausedAt == nil || !pause.PausedAt.Equal(enginePause.PausedAt) {
			t.Fatalf("%s pause[%d] PausedAt = %#v, want %s", label, i, pause.PausedAt, enginePause.PausedAt)
		}
		if !pause.PausedUntil.Equal(enginePause.PausedUntil) || !pause.RecoverAt.Equal(enginePause.PausedUntil) {
			t.Fatalf("%s pause[%d] window = %#v, want paused-until/recover-at %s", label, i, pause, enginePause.PausedUntil)
		}
		if pause.AffectedTransitionIds == nil || len(*pause.AffectedTransitionIds) == 0 {
			t.Fatalf("%s pause[%d] affected transitions = %#v, want non-empty projection", label, i, pause.AffectedTransitionIds)
		}
		if pause.AffectedWorkstationNames == nil || len(*pause.AffectedWorkstationNames) == 0 {
			t.Fatalf("%s pause[%d] affected workstation names = %#v, want non-empty projection", label, i, pause.AffectedWorkstationNames)
		}
		if pause.AffectedWorkerTypes == nil || len(*pause.AffectedWorkerTypes) == 0 {
			t.Fatalf("%s pause[%d] affected worker types = %#v, want non-empty projection", label, i, pause.AffectedWorkerTypes)
		}
		if pause.AffectedWorkTypeIds == nil || len(*pause.AffectedWorkTypeIds) == 0 {
			t.Fatalf("%s pause[%d] affected work type IDs = %#v, want non-empty projection", label, i, pause.AffectedWorkTypeIds)
		}
	}
}

func assertContainsAll(t *testing.T, got string, want []string) {
	t.Helper()

	for _, fragment := range want {
		if !strings.Contains(got, fragment) {
			t.Fatalf("expected %q to contain %q", got, fragment)
		}
	}
}

func providerErrorCommandFailure(stderr string) workers.CommandResult {
	return workers.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(stderr),
	}
}

func assertRetryableInternalServerRequeueOutcome(
	t *testing.T,
	runner *testutil.ProviderCommandRunner,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
) {
	t.Helper()

	if runner.CallCount() < 3 {
		t.Fatalf("provider command count = %d, want at least 3", runner.CallCount())
	}
	assertProviderCommandMatchesLane(t, runner.LastRequest(), workers.ModelProviderCodex, work.Name, "gpt-5-codex")

	if len(outcome.Dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want 1 failed dispatch before requeue", len(outcome.Dispatches))
	}
	dispatch := outcome.Dispatches[0]
	if dispatch.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want %s", dispatch.Outcome, interfaces.OutcomeFailed)
	}
	assertDispatchHistoryMatchesWork(t, dispatch, work)
	assertProviderFailureIsRetryableInternalServer(t, dispatch.ProviderFailure)
	if !dispatchHasOutputMutationToPlace(dispatch, work.WorkTypeID+":init", work.WorkID) {
		t.Fatalf("dispatch mutations = %#v, want requeue to %s:init", dispatch.OutputMutations, work.WorkTypeID)
	}
	if len(outcome.EngineState.ActiveThrottlePauses) != 0 {
		t.Fatalf("active throttle pauses = %d, want 0", len(outcome.EngineState.ActiveThrottlePauses))
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
}

func assertProviderFailureIsRetryableInternalServer(t *testing.T, failure *interfaces.ProviderFailureMetadata) {
	t.Helper()

	if failure == nil {
		t.Fatal("ProviderFailure is nil, want normalized internal_server_error metadata")
	}
	if failure.Type != interfaces.ProviderErrorTypeInternalServerError {
		t.Fatalf("provider failure type = %s, want %s", failure.Type, interfaces.ProviderErrorTypeInternalServerError)
	}
	if failure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("provider failure family = %s, want %s", failure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
}

func requireProviderErrorDispatchCompletedEventForWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	workID string,
) factoryapi.DispatchResponseEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE event %q: %v", event.Id, err)
		}
		for _, eventWorkID := range stringSliceValue(event.Context.WorkIds) {
			if eventWorkID == workID {
				return payload
			}
		}
	}

	t.Fatalf("missing DISPATCH_RESPONSE event for work %q", workID)
	return factoryapi.DispatchResponseEventPayload{}
}

func assertNoAuthRemediationText(t *testing.T, body string) {
	t.Helper()

	lowered := strings.ToLower(body)
	for _, forbidden := range []string{"auth_failure", "authentication", "api key", "unauthorized", "forbidden"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("expected operator-facing text to avoid %q, got %q", forbidden, body)
		}
	}
}

func dispatchHasOutputMutationToPlace(dispatch interfaces.CompletedDispatch, placeID, workID string) bool {
	for _, mutation := range dispatch.OutputMutations {
		if mutation.ToPlace != placeID || mutation.Token == nil {
			continue
		}
		if mutation.Token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

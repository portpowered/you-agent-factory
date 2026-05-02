package providers

import (
	"context"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=provider-throttle-isolation-smoke review=2026-07-19 removal=split-pause-harness-setup-submissions-and-lane-assertions-before-next-provider-error-smoke-change
func TestProviderErrorSmoke_ThrottlePauseOnlyBlocksTheAffectedProviderModelLane(t *testing.T) {
	support.SkipLongFunctional(t, "slow provider throttle-pause isolation sweep")

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

// portos:func-length-exception owner=agent-factory reason=provider-worker-pool-error-smoke review=2026-07-19 removal=split-codex-timeout-and-worker-pool-assertions-before-next-provider-error-smoke-change
func TestProviderErrorSmoke_CodexAndTimeoutFailuresNormalizeThroughWorkerPool(t *testing.T) {
	support.SkipLongFunctional(t, "slow provider worker-pool smoke")
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
			support.LegacyFixtureDir(t, "worktree_passthrough"),
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
			support.LegacyFixtureDir(t, "worktree_passthrough"),
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
	support.SkipLongFunctional(t, "slow codex retry smoke")
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
				support.LegacyFixtureDir(t, "worktree_passthrough"),
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
	support.SkipLongFunctional(t, "slow codex windows-exit provider smoke")
	entry := providerErrorCorpusEntryForTest(t, "codex_windows_exit_code_4294967295")
	smokeHarness := testutil.NewProviderErrorSmokeHarness(
		t,
		support.LegacyFixtureDir(t, "worktree_passthrough"),
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
	support.SkipLongFunctional(t, "slow guarded loop-breaker provider smoke")

	smokeHarness := testutil.NewProviderErrorSmokeHarness(
		t,
		support.LegacyFixtureDir(t, "worktree_passthrough"),
		workers.ModelProviderCodex,
		"gpt-5-codex",
		testutil.WithProviderErrorSmokeServiceOptions(testutil.WithFullWorkerPoolAndScriptWrap()),
	)
	configureProviderErrorLoopBreaker(t, smokeHarness.Dir)

	const highDemandMessage = `ERROR: We're currently experiencing high demand, which may cause temporary errors.`
	for range 6 {
		smokeHarness.QueueProviderResults(providerErrorCommandFailure(highDemandMessage))
	}

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

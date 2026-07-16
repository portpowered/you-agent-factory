//go:build functionallong

package workflow

import (
	"fmt"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestAdhocProcessReviewContract_ProcessContinueUsesContinuePath(t *testing.T) {
	support.SkipLongFunctional(t, "slow process/review continue-loop sweep")

	_, provider, harness := newAdhocProcessReviewHarness(t, []workerexecution.InferenceResponse{
		{Content: "<CONTINUE>\n"},
		{Content: "<COMPLETE>\n"},
		{Content: "<COMPLETE>\n"},
	})

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:failed")

	if got := provider.CallCount("processor"); got != 3 {
		t.Fatalf("processor call count = %d, want 3", got)
	}

	calls := provider.Calls("processor")
	assertProviderCallWorkstations(t, calls, []string{"process", "process", "review"})

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	processDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "process")
	if len(processDispatches) != 2 {
		t.Fatalf("process dispatch count = %d, want 2", len(processDispatches))
	}
	if processDispatches[0].Outcome != workerexecution.OutcomeContinue {
		t.Fatalf("first process outcome = %s, want %s", processDispatches[0].Outcome, workerexecution.OutcomeContinue)
	}
	assertDispatchHasOutputToPlace(t, processDispatches[0], "task:init")
	assertDispatchOutputTagAbsent(t, processDispatches[0], "_rejection_feedback")

	for _, dispatch := range processDispatches {
		if dispatch.Outcome == workerexecution.OutcomeRejected {
			t.Fatalf("process dispatch unexpectedly used rejection outcome: %#v", dispatch)
		}
	}

	reviewDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review")
	if len(reviewDispatches) != 1 {
		t.Fatalf("review dispatch count = %d, want 1", len(reviewDispatches))
	}
	if reviewDispatches[0].Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("review outcome = %s, want %s", reviewDispatches[0].Outcome, workerexecution.OutcomeAccepted)
	}
}

func TestAdhocProcessReviewContract_ReviewRejectionRoutesBackWithFeedback(t *testing.T) {
	support.SkipLongFunctional(t, "slow process/review rejection feedback sweep")

	_, provider, harness := newAdhocProcessReviewHarness(t, []workerexecution.InferenceResponse{
		{Content: "<COMPLETE>\n"},
		{Content: "missing tests"},
		{Content: "<COMPLETE>\n"},
		{Content: "<COMPLETE>\n"},
	})

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:failed")

	if got := provider.CallCount("processor"); got != 4 {
		t.Fatalf("processor call count = %d, want 4", got)
	}

	calls := provider.Calls("processor")
	assertProviderCallWorkstations(t, calls, []string{"process", "review", "process", "review"})

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	reviewDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review")
	if len(reviewDispatches) != 2 {
		t.Fatalf("review dispatch count = %d, want 2", len(reviewDispatches))
	}
	if reviewDispatches[0].Outcome != workerexecution.OutcomeRejected {
		t.Fatalf("first review outcome = %s, want %s", reviewDispatches[0].Outcome, workerexecution.OutcomeRejected)
	}
	assertDispatchHasOutputToPlace(t, reviewDispatches[0], "task:init")

	processDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "process")
	if len(processDispatches) != 2 {
		t.Fatalf("process dispatch count = %d, want 2", len(processDispatches))
	}
	for i, dispatch := range processDispatches {
		if dispatch.Outcome != workerexecution.OutcomeAccepted {
			t.Fatalf("process dispatch %d outcome = %s, want %s", i, dispatch.Outcome, workerexecution.OutcomeAccepted)
		}
	}
}

func TestAdhocProcessReviewContract_ReviewLoopBreakerTripsAfterTrueRejections(t *testing.T) {
	support.SkipLongFunctional(t, "slow process/review loop-breaker rejection sweep")

	responses := make([]workerexecution.InferenceResponse, 0, 21)
	for i := 0; i < 10; i++ {
		responses = append(responses,
			workerexecution.InferenceResponse{Content: "<COMPLETE>\n"},
			workerexecution.InferenceResponse{Content: fmt.Sprintf("review rejection %d", i+1)},
		)
	}
	responses = append(responses, workerexecution.InferenceResponse{Content: "<COMPLETE>\n"})

	_, provider, harness := newAdhocProcessReviewHarness(t, responses)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:complete")

	if got := provider.CallCount("processor"); got != 21 {
		t.Fatalf("processor call count = %d, want 21", got)
	}

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	processDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "process")
	if len(processDispatches) != 11 {
		t.Fatalf("process dispatch count = %d, want 11", len(processDispatches))
	}

	reviewDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review")
	if len(reviewDispatches) != 10 {
		t.Fatalf("review dispatch count = %d, want 10", len(reviewDispatches))
	}
	for i, dispatch := range reviewDispatches {
		if dispatch.Outcome != workerexecution.OutcomeRejected {
			t.Fatalf("review dispatch %d outcome = %s, want %s", i, dispatch.Outcome, workerexecution.OutcomeRejected)
		}
	}

	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "review-loop-breaker", "task:failed")
}

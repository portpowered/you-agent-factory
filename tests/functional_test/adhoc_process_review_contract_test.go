package functional_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestAdhocProcessReviewContract_ProcessContinueUsesContinuePath(t *testing.T) {
	_, provider, harness := newAdhocProcessReviewHarness(t, []interfaces.InferenceResponse{
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
	if processDispatches[0].Outcome != interfaces.OutcomeContinue {
		t.Fatalf("first process outcome = %s, want %s", processDispatches[0].Outcome, interfaces.OutcomeContinue)
	}
	assertDispatchHasOutputToPlace(t, processDispatches[0], "task:init")
	assertDispatchOutputTagAbsent(t, processDispatches[0], "_rejection_feedback")

	for _, dispatch := range processDispatches {
		if dispatch.Outcome == interfaces.OutcomeRejected {
			t.Fatalf("process dispatch unexpectedly used rejection outcome: %#v", dispatch)
		}
	}

	reviewDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review")
	if len(reviewDispatches) != 1 {
		t.Fatalf("review dispatch count = %d, want 1", len(reviewDispatches))
	}
	if reviewDispatches[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("review outcome = %s, want %s", reviewDispatches[0].Outcome, interfaces.OutcomeAccepted)
	}
}

func TestAdhocProcessReviewContract_ReviewRejectionRoutesBackWithFeedback(t *testing.T) {
	_, provider, harness := newAdhocProcessReviewHarness(t, []interfaces.InferenceResponse{
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
	if reviewDispatches[0].Outcome != interfaces.OutcomeRejected {
		t.Fatalf("first review outcome = %s, want %s", reviewDispatches[0].Outcome, interfaces.OutcomeRejected)
	}
	assertDispatchHasOutputToPlace(t, reviewDispatches[0], "task:init")

	processDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "process")
	if len(processDispatches) != 2 {
		t.Fatalf("process dispatch count = %d, want 2", len(processDispatches))
	}
	for i, dispatch := range processDispatches {
		if dispatch.Outcome != interfaces.OutcomeAccepted {
			t.Fatalf("process dispatch %d outcome = %s, want %s", i, dispatch.Outcome, interfaces.OutcomeAccepted)
		}
	}
}

func TestAdhocProcessReviewContract_ReviewLoopBreakerTripsAfterTrueRejections(t *testing.T) {
	responses := make([]interfaces.InferenceResponse, 0, 21)
	for i := 0; i < 10; i++ {
		responses = append(responses,
			interfaces.InferenceResponse{Content: "<COMPLETE>\n"},
			interfaces.InferenceResponse{Content: fmt.Sprintf("review rejection %d", i+1)},
		)
	}
	responses = append(responses, interfaces.InferenceResponse{Content: "<COMPLETE>\n"})

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
		if dispatch.Outcome != interfaces.OutcomeRejected {
			t.Fatalf("review dispatch %d outcome = %s, want %s", i, dispatch.Outcome, interfaces.OutcomeRejected)
		}
	}

	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "review-loop-breaker", "task:failed")
}

func newAdhocProcessReviewHarness(
	t *testing.T,
	responses []interfaces.InferenceResponse,
) (string, *testutil.MockWorkerMapProvider, *testutil.ServiceTestHarness) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/adhoc/factory"))
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": responses,
	})
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExecutionBaseDir(dir),
	)

	harness.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		WorkID:     "task-process-review-contract",
		TraceID:    "trace-process-review-contract",
		Name:       "align-process-review-loop-contract",
		Payload:    []byte("process review contract coverage"),
	}})

	return dir, provider, harness
}

func dispatchesForWorkstation(history []interfaces.CompletedDispatch, workstationName string) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		if dispatch.WorkstationName == workstationName {
			dispatches = append(dispatches, dispatch)
		}
	}
	return dispatches
}

func assertProviderCallWorkstations(
	t *testing.T,
	calls []interfaces.ProviderInferenceRequest,
	want []string,
) {
	t.Helper()

	if len(calls) != len(want) {
		t.Fatalf("provider call count = %d, want %d", len(calls), len(want))
	}
	for i, workstationName := range want {
		if calls[i].Dispatch.WorkstationName != workstationName {
			t.Fatalf("provider call %d workstation = %q, want %q", i, calls[i].Dispatch.WorkstationName, workstationName)
		}
	}
}

func assertDispatchHasOutputToPlace(t *testing.T, dispatch interfaces.CompletedDispatch, placeID string) {
	t.Helper()

	for _, mutation := range dispatch.OutputMutations {
		if mutation.ToPlace == placeID {
			return
		}
	}

	t.Fatalf("dispatch %#v missing output mutation to %q", dispatch, placeID)
}

func assertDispatchOutputTagAbsent(t *testing.T, dispatch interfaces.CompletedDispatch, key string) {
	t.Helper()

	for _, mutation := range dispatch.OutputMutations {
		if mutation.Token == nil || mutation.Token.Color.Tags == nil {
			continue
		}
		if _, ok := mutation.Token.Color.Tags[key]; ok {
			t.Fatalf("dispatch %#v unexpectedly set tag %q", dispatch, key)
		}
	}
}

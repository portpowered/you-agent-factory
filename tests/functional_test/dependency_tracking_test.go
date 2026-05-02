package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestDependencyTracking_BlocksUntilSatisfied validates that a token with a
// DEPENDS_ON relation is blocked from transitioning until its dependency
// reaches the required state.
//
//	Net: task (init -> processing -> complete, failed)
//	     "start" transition with DependencyGuard on input arc
//	     "finish" transition (no guard)
//
// Flow:
//  1. Submit A (no deps) -> processes through start, lands in processing
//  2. Submit B (DEPENDS_ON A, RequiredState: "complete") -> blocked at init
//  3. A finishes -> lands in complete
//  4. B unblocks -> processes through start -> processing -> complete
func TestDependencyTracking_BlocksUntilSatisfied(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dependency_tracking_dir"))

	workIDA := "task-A-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     workIDA,
		Payload:    []byte("task A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("task B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		acceptedProviderResponse(),
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion: A completes, B unblocks, processes, and completes.
	h.RunUntilComplete(t, 10*time.Second)

	// Both tokens should now be in task:complete.
	h.Assert().
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		PlaceTokenCount("task:complete", 2)

	// Starter called exactly 2 times total (once for A, once for B).
	if got := len(providerCallsForWorker(provider, "starter")); got != 2 {
		t.Errorf("expected starter called 2 times, got %d", got)
	}
}

// TestDependencyTracking_NoDepsPassThrough verifies that tokens without
// DEPENDS_ON relations pass through the DependencyGuard without blocking.
func TestDependencyTracking_NoDepsPassThrough(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dependency_tracking_simple_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("no deps"))

	provider := testutil.NewMockProvider(acceptedProviderResponse())
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	// Token should be in complete after running.
	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)
}

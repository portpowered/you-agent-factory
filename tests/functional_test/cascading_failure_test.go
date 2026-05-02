package functional_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestCascadingFailure_DirectChild validates that when a parent token fails,
// a child token that depends on it is automatically moved to the failed state.
//
//	Net: task (init -> processing -> complete, failed)
//	     "start" with DependencyGuard, "finish" transition
//
// Flow:
//  1. Submit P (no deps) in processing
//  2. Submit C (DEPENDS_ON P, RequiredState: "complete") -> blocked at init
//  3. P fails (worker returns FAILED) -> P goes to task:failed
//  4. Cascading failure moves C to task:failed automatically
func TestCascadingFailure_DirectChild(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "cascading_failure"))

	parentWorkID := "parent-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID:  "task",
		WorkID:      parentWorkID,
		TargetState: "processing",
		Payload:     []byte("parent"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("child"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: parentWorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"finisher": {
			{Error: errors.New("upstream service down")}, // P finish fails
		},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion: P's "finish" fires and fails -> P to task:failed,
	// cascading failure moves C to task:failed automatically.
	h.RunUntilComplete(t, 10*time.Second)

	// Both P and C should now be in task:failed.
	h.Assert().
		PlaceTokenCount("task:failed", 2).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:complete")

	// Verify C's FailureRecord references P, proving the child failed from
	// dependency cascade routing rather than an edge provider failure.
	snap := h.Marking()
	foundChild := false
	for _, tok := range snap.Tokens {
		if tok.Color.WorkTypeID == "task" && tokenDependsOn(tok, parentWorkID) {
			// This is C.
			foundChild = true
			if len(tok.History.FailureLog) == 0 {
				t.Error("child token should have a FailureRecord from cascading failure")
			} else {
				record := tok.History.FailureLog[0]
				if !strings.Contains(record.Error, parentWorkID) {
					t.Errorf("FailureRecord should reference parent WorkID %q, got: %q", parentWorkID, record.Error)
				}
			}
			if !strings.Contains(tok.History.LastError, parentWorkID) {
				t.Errorf("LastError should reference parent WorkID %q, got: %q", parentWorkID, tok.History.LastError)
			}
		}
	}
	if !foundChild {
		t.Fatal("child token with dependency on parent was not found")
	}
}

func tokenDependsOn(tok *interfaces.Token, workID string) bool {
	for _, rel := range tok.Color.Relations {
		if rel.Type == interfaces.RelationDependsOn && rel.TargetWorkID == workID {
			return true
		}
	}
	return false
}

// TestCascadingFailure_Transitive validates transitive cascading:
// P fails -> C1 (depends on P) fails -> C2 (depends on C1) also fails.
//
//	Net: task (init -> processing -> complete, failed)
//
// Flow:
//  1. Submit P -> starts processing
//  2. Submit C1 (DEPENDS_ON P) -> blocked
//  3. Submit C2 (DEPENDS_ON C1) -> blocked
//  4. P fails -> cascading moves C1 to failed -> cascading moves C2 to failed
func TestCascadingFailure_Transitive(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "cascading_failure"))

	pWorkID := "P-work-id"
	c1WorkID := "C1-work-id"

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     pWorkID,
		Payload:    []byte("P"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     c1WorkID,
		Payload:    []byte("C1"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: pWorkID, RequiredState: "complete"},
		},
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("C2"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: c1WorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"}, // P start
			{Content: "COMPLETE"}, // C1 start (if dispatched before cascade)
			{Content: "COMPLETE"}, // C2 start (if dispatched before cascade)
		},
		"finisher": {
			{Error: errors.New("crash")}, // P finish fails
			{Error: errors.New("crash")}, // C1 finish (if dispatched before cascade)
			{Error: errors.New("crash")}, // C2 finish (if dispatched before cascade)
		},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion -- all cascading should have happened.
	h.RunUntilComplete(t, 10*time.Second)

	// All 3 tokens should be in task:failed.
	h.Assert().
		PlaceTokenCount("task:failed", 3).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:complete")

	// Verify C2 has a FailureRecord (either from cascading failure referencing
	// C1, or from direct finisher failure if dispatched before cascade).
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.Color.WorkID != pWorkID && tok.Color.WorkID != c1WorkID {
			// This is C2.
			if len(tok.History.FailureLog) == 0 && tok.History.LastError == "" {
				t.Error("C2 should have a failure record")
			}
		}
	}
}

// TestCascadingFailure_CompletedNotCascaded verifies that tokens which have
// already completed are NOT affected by cascading failure.
func TestCascadingFailure_CompletedNotCascaded(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "cascading_failure"))

	aWorkID := "A-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     aWorkID,
		Payload:    []byte("A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: aWorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"}, // A start
			{Content: "COMPLETE"}, // B start
		},
		"finisher": {
			{Content: "COMPLETE"},       // A finish succeeds
			{Error: errors.New("oops")}, // B finish fails
		},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion.
	h.RunUntilComplete(t, 10*time.Second)

	// B failed, A still complete -- cascading failure should NOT move A to failed.
	h.Assert().
		HasTokenInPlace("task:complete"). // A still complete
		HasTokenInPlace("task:failed")    // B failed on its own
}

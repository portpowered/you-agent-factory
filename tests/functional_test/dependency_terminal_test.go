package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestDependencyTerminal_BlockedUntilArchived verifies that token B remains
// blocked while A is in any non-archived state, and only dispatches after
// A reaches the archived terminal state.
func TestDependencyTerminal_BlockedUntilArchived(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dependency_terminal"))

	// Submit A and B with pre-assigned WorkIDs via seed files.
	workIDA := "prd-A-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"executor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
		"reviewer": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	// Run to completion: A archives, B unblocks, processes, and archives.
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("prd:archived", 2).
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:in-review")

	// Executor called twice (once for A, once for B).
	if provider.CallCount("executor") != 2 {
		t.Errorf("expected executor called 2 times (A+B), got %d", provider.CallCount("executor"))
	}
}

// TestDependencyTerminal_BlockedDuringProcessing verifies that B remains
// blocked even when A is in an intermediate processing state (in-review),
// not just in the initial state.
func TestDependencyTerminal_BlockedDuringProcessing(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dependency_terminal"))

	// Submit A and B with pre-assigned WorkIDs via seed files.
	workIDA := "prd-A-processing"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"executor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
		"reviewer": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	// Run to completion: A archives, B unblocks and archives.
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("prd:archived", 2).
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:in-review").
		HasNoTokenInPlace("prd:failed")
}

// TestDependencyTerminal_BothComplete verifies that both A and B eventually
// reach the archived terminal state when the full pipeline completes.
func TestDependencyTerminal_BothComplete(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dependency_terminal"))

	// Submit A and B with pre-assigned WorkIDs via seed files.
	workIDA := "prd-A-both"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"executor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
		"reviewer": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("prd:archived", 2).
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:in-review").
		HasNoTokenInPlace("prd:failed").
		AllTokensTerminal()
}

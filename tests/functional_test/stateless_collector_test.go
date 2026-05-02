package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestStatelessCollector_TwoStagePipeline validates end-to-end processing
// through the two-stage pipeline: tokens injected at init flow through
// stage1 → done, proving results flow through the full service layer
// with MockProvider driving stop-token evaluation.
func TestStatelessCollector_TwoStagePipeline(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w1"}`))

	// Two provider calls: one per stage.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	// Verify exactly one token in terminal place.
	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:stage1").
		HasNoTokenInPlace("task:failed")

	// Verify provider was called exactly twice (once per stage).
	if provider.CallCount() != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.CallCount())
	}
}

// TestStatelessCollector_MultipleWorkItems validates that multiple work items
// all flow through the pipeline independently.
func TestStatelessCollector_MultipleWorkItems(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w1"}`))

	// Provide enough responses for 1 work item × 2 stages.
	// Submit additional items after harness construction.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Submit 2 more work items (1 already seeded via file).
	if err := h.SubmitWork("task", []byte(`{"item": "w2"}`)); err != nil {
		t.Fatalf("submit w2: %v", err)
	}
	if err := h.SubmitWork("task", []byte(`{"item": "w3"}`)); err != nil {
		t.Fatalf("submit w3: %v", err)
	}

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 3).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:stage1").
		HasNoTokenInPlace("task:failed")
}

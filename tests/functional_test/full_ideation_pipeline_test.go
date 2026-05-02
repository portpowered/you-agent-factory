package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestFullIdeationPipeline_HappyPath verifies the full ideation pipeline:
// idea (FileWatcher) → plan-idea (planner) → prd → convert-prd (logical-move) →
// story → execute-story (executor) → in-review → review-story (reviewer) → complete.
//
// Uses ServiceTestHarness with MockProvider and seed files for initial submission.
func TestFullIdeationPipeline_HappyPath(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "full_ideation_pipeline"))

	// Write the idea file as a proper SubmitRequest JSON so that the
	// FileWatcher preserves the TraceID for lineage verification.
	originTraceID := "trace-idea-lineage-001"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"search bar on docs"}`),
	})

	// MockProvider responses interleaved in execution order:
	// 1. planner (idea:init → prd:init) — stop_token COMPLETE
	// 2. executor (story:init → story:in-review) — stop_token COMPLETE
	// 3. reviewer (story:in-review → story:complete) — stop_token ACCEPTED
	// converter is LOGICAL_MOVE — no provider call.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "PRD created. COMPLETE"},
		interfaces.InferenceResponse{Content: "Code written. COMPLETE"},
		interfaces.InferenceResponse{Content: "Looks good. ACCEPTED"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion — factory.Run picks up the seeded submission and
	// processes through: planner → converter → executor → reviewer.
	h.RunUntilComplete(t, 15*time.Second)

	// Token reaches story:complete.
	h.Assert().HasTokenInPlace("story:complete")

	// No tokens remain in intermediate places.
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review")

	// TraceID lineage: the story:complete token carries the same TraceID
	// as the original idea token, proving lineage across cross-work-type
	// transitions (idea → prd → story).
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "story:complete" {
			if tok.Color.TraceID != originTraceID {
				t.Errorf("TraceID lineage broken: idea had %q, story:complete has %q",
					originTraceID, tok.Color.TraceID)
			}
		}
	}

	// Provider called exactly 3 times: planner + executor + reviewer.
	// Converter is LOGICAL_MOVE — no provider call.
	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}
}

// TestFullIdeationPipeline_RejectionLoop verifies that reviewer rejections
// loop the token back through execution and review multiple times.
//
// Flow: idea:init → plan-idea → prd:init → convert-prd → story:init →
// execute-story → story:in-review → review-story(REJECT) → story:init →
// execute-story → story:in-review → review-story(REJECT) → story:init →
// execute-story → story:in-review → review-story(ACCEPT) → story:complete
//
// Provider calls: planner(1) + executor(3) + reviewer(3) = 7
// Converter is LOGICAL_MOVE — no provider call.
func TestFullIdeationPipeline_RejectionLoop(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "full_ideation_pipeline"))

	originTraceID := "trace-rejection-loop-001"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"rejection loop test"}`),
	})

	// MockProvider responses in execution order:
	// 1. planner: accepts (contains COMPLETE)
	// 2. executor #1: accepts (contains COMPLETE)
	// 3. reviewer #1: rejects (no ACCEPTED token)
	// 4. executor #2: accepts (contains COMPLETE)
	// 5. reviewer #2: rejects (no ACCEPTED token)
	// 6. executor #3: accepts (contains COMPLETE)
	// 7. reviewer #3: accepts (contains ACCEPTED)
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "PRD created. COMPLETE"},
		interfaces.InferenceResponse{Content: "Code written. COMPLETE"},
		interfaces.InferenceResponse{Content: "Needs more work. REJECTED"},
		interfaces.InferenceResponse{Content: "Code revised. COMPLETE"},
		interfaces.InferenceResponse{Content: "Still not right. REJECTED"},
		interfaces.InferenceResponse{Content: "Code revised again. COMPLETE"},
		interfaces.InferenceResponse{Content: "Looks good now. ACCEPTED"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Longer timeout — rejection loop requires 7 provider calls.
	h.RunUntilComplete(t, 30*time.Second)

	// Token reaches story:complete after 2 rejections + 1 acceptance.
	h.Assert().HasTokenInPlace("story:complete")

	// No tokens remain in intermediate places.
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review")

	// TraceID lineage preserved across all iterations.
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "story:complete" {
			if tok.Color.TraceID != originTraceID {
				t.Errorf("TraceID lineage broken: idea had %q, story:complete has %q",
					originTraceID, tok.Color.TraceID)
			}
		}
	}

	// Executor called 3 times, reviewer called 3 times, planner called 1 time.
	// Converter is LOGICAL_MOVE — no provider call. Total: 7.
	if provider.CallCount() != 7 {
		t.Errorf("expected provider called 7 times, got %d", provider.CallCount())
	}
}

// TestFullIdeationPipeline_CrossWorkTypeLineage verifies that tokens correctly
// transition across work type boundaries (idea → prd → story) with TraceID
// lineage preserved through to the final state.
func TestFullIdeationPipeline_CrossWorkTypeLineage(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "full_ideation_pipeline"))

	originTraceID := "trace-cross-wt-lineage-001"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"cross-work-type lineage test"}`),
	})

	// MockProvider responses: planner, executor, reviewer (happy path).
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "PRD created. COMPLETE"},
		interfaces.InferenceResponse{Content: "Code written. COMPLETE"},
		interfaces.InferenceResponse{Content: "Looks good. ACCEPTED"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion.
	h.RunUntilComplete(t, 15*time.Second)

	// Final state: token in story:complete with preserved lineage.
	h.Assert().
		HasTokenInPlace("story:complete").
		TokenHasTraceID("story:complete", originTraceID)

	// No orphaned tokens remain in intermediate places.
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review").
		HasNoTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:failed").
		HasNoTokenInPlace("story:failed")

	// All tokens in terminal places: only story:complete should exist.
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID != "story:complete" {
			t.Errorf("unexpected token in non-terminal place %q (id=%s, workType=%s)",
				tok.PlaceID, tok.ID, tok.Color.WorkTypeID)
		}
	}

	// Provider called exactly 3 times: planner + executor + reviewer.
	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}
}

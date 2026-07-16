//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFullIdeationPipeline_HappyPath verifies the full ideation pipeline:
// idea (FileWatcher) -> plan-idea (planner) -> prd -> convert-prd (logical-move) ->
// story -> execute-story (executor) -> in-review -> review-story (reviewer) -> complete.
func TestFullIdeationPipeline_HappyPath(t *testing.T) {
	support.SkipLongFunctional(t, "slow ideation happy-path sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))

	originTraceID := "trace-idea-lineage-001"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"search bar on docs"}`),
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "PRD created. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Code written. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Looks good. ACCEPTED"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().HasTokenInPlace("story:complete")
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review")

	for _, tok := range h.Marking().Tokens {
		if tok.PlaceID == "story:complete" && tok.Color.TraceID != originTraceID {
			t.Errorf("TraceID lineage broken: idea had %q, story:complete has %q", originTraceID, tok.Color.TraceID)
		}
	}

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}
}

// TestFullIdeationPipeline_RejectionLoop verifies that reviewer rejections
// loop the token back through execution and review multiple times.
func TestFullIdeationPipeline_RejectionLoop(t *testing.T) {
	support.SkipLongFunctional(t, "slow ideation rejection-loop sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))

	originTraceID := "trace-rejection-loop-001"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"rejection loop test"}`),
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "PRD created. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Code written. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Needs more work. REJECTED"},
		workerexecution.InferenceResponse{Content: "Code revised. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Still not right. REJECTED"},
		workerexecution.InferenceResponse{Content: "Code revised again. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Looks good now. ACCEPTED"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 30*time.Second)

	h.Assert().HasTokenInPlace("story:complete")
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review")

	for _, tok := range h.Marking().Tokens {
		if tok.PlaceID == "story:complete" && tok.Color.TraceID != originTraceID {
			t.Errorf("TraceID lineage broken: idea had %q, story:complete has %q", originTraceID, tok.Color.TraceID)
		}
	}

	if provider.CallCount() != 7 {
		t.Errorf("expected provider called 7 times, got %d", provider.CallCount())
	}
}

// TestFullIdeationPipeline_CrossWorkTypeLineage verifies that tokens correctly
// transition across work type boundaries (idea -> prd -> story) with TraceID
// lineage preserved through to the final state.
func TestFullIdeationPipeline_CrossWorkTypeLineage(t *testing.T) {
	support.SkipLongFunctional(t, "slow ideation cross-work-type lineage sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))

	originTraceID := "trace-cross-wt-lineage-001"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"cross-work-type lineage test"}`),
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "PRD created. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Code written. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Looks good. ACCEPTED"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace("story:complete").
		TokenHasTraceID("story:complete", originTraceID)
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review").
		HasNoTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:failed").
		HasNoTokenInPlace("story:failed")

	for _, tok := range h.Marking().Tokens {
		if tok.PlaceID != "story:complete" {
			t.Errorf("unexpected token in non-terminal place %q (id=%s, workType=%s)", tok.PlaceID, tok.ID, tok.Color.WorkTypeID)
		}
	}

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}
}

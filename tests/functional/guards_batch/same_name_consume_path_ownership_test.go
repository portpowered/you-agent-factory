package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryeventprojection "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// consumePathOwnershipLayer records which subsystem owns an observed stranded
// consume-path outcome. Story 001 uses these labels to classify live queue
// cells without introducing new customer-facing vocabulary.
type consumePathOwnershipLayer string

const (
	consumePathLayerRuntimeCorrect          consumePathOwnershipLayer = "runtime_correct"
	consumePathLayerHistoricalQueueArtifact consumePathOwnershipLayer = "historical_queue_artifact"
	consumePathLayerProjectionVisibilityGap consumePathOwnershipLayer = "projection_visibility_gap"
)

func scaffoldConsumePathFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "same_name_consume_path",
		"workTypes": []map[string]any{
			{
				"name": "idea",
				"states": []map[string]any{
					{"name": "to-complete", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "task",
				"states": []map[string]any{
					{"name": "to-complete", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workstations": []map[string]any{
			{
				"name":   "consume",
				"type":   "LOGICAL_MOVE",
				"worker": "",
				"inputs": []map[string]any{
					{
						"workType": "task",
						"state":    "to-complete",
					},
					{
						"workType": "idea",
						"state":    "to-complete",
						"guards": []map[string]any{
							{"type": "SAME_NAME", "matchInput": "task"},
						},
					},
				},
				"outputs": []map[string]any{
					{"workType": "idea", "state": "complete"},
					{"workType": "task", "state": "complete"},
				},
			},
		},
	})
	support.WriteWorkstationConfig(t, dir, "consume", "---\ntype: LOGICAL_MOVE\n---\nConsume reviewed same-name pairs.\n")
	return dir
}

func TestSameNameConsumePathOwnership_MatchingPairCompletesThroughLogicalMove(t *testing.T) {
	dir := scaffoldConsumePathFactory(t)
	h := support.NewGuardsBatchHarness(t, dir)

	const cellName = "dynamic-workflows-cell-cli-validate-list"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	submitConsumePathPair(t, h, cellName)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)

	h.Assert().
		HasNoTokenInPlace("idea:to-complete").
		HasNoTokenInPlace("task:to-complete")

	layer := classifyConsumePathOutcome(t, h, cellName)
	if layer != consumePathLayerRuntimeCorrect {
		t.Fatalf("ownership layer = %q, want %q for matching reviewed same-name pair", layer, consumePathLayerRuntimeCorrect)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathOwnership_TaskOnlyWithoutIdeaTwin_StrandedAsHistoricalArtifact(t *testing.T) {
	dir := scaffoldConsumePathFactory(t)
	h := support.NewGuardsBatchHarness(t, dir)

	const cellName = "dynamic-workflows-cell-cli-run-status-result"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	h.SubmitFull(context.Background(), []work.SubmitRequest{{
		Name:        cellName,
		WorkTypeID:  "task",
		TargetState: "to-complete",
		TraceID:     "trace-task-only",
	}})

	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)
	time.Sleep(150 * time.Millisecond)

	h.Assert().
		PlaceTokenCount("task:to-complete", 1).
		HasNoTokenInPlace("idea:to-complete").
		HasNoTokenInPlace("idea:complete")

	layer := classifyConsumePathOutcome(t, h, cellName)
	if layer != consumePathLayerHistoricalQueueArtifact {
		t.Fatalf("ownership layer = %q, want %q when only task:to-complete is present", layer, consumePathLayerHistoricalQueueArtifact)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathOwnership_OrphanTaskAfterPriorConsume_MatchesLiveQueuePattern(t *testing.T) {
	dir := scaffoldConsumePathFactory(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	const cellName = "dynamic-workflows-cell-mcp-tools"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	submitSameNameOrphanAfterConsumePattern(t, h, cellName, "trace-task-b-"+cellName)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)
	time.Sleep(150 * time.Millisecond)

	h.Assert().
		PlaceTokenCount("task:to-complete", 1).
		PlaceTokenCount("idea:complete", 1).
		HasNoTokenInPlace("idea:to-complete")

	layer := classifyConsumePathOutcome(t, h, cellName)
	if layer != consumePathLayerHistoricalQueueArtifact {
		t.Fatalf("ownership layer = %q, want %q for orphan task after prior consume", layer, consumePathLayerHistoricalQueueArtifact)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathOwnership_ProjectionMatchesRuntimeBeforeConsume(t *testing.T) {
	dir := scaffoldConsumePathFactory(t)
	h := support.NewGuardsBatchHarness(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	h.SubmitFull(context.Background(), []work.SubmitRequest{
		{
			Name:        "idea-only",
			WorkTypeID:  "idea",
			TargetState: "to-complete",
			TraceID:     "trace-projection-idea",
		},
		{
			Name:        "task-only",
			WorkTypeID:  "task",
			TargetState: "to-complete",
			TraceID:     "trace-projection-task",
		},
	})

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:to-complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)

	runtimeSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	projectionView, err := consumePathProjectionView(t, h)
	if err != nil {
		t.Fatalf("consumePathProjectionView: %v", err)
	}

	if !hasNamedTokenInPlace(runtimeSnap.Marking, "idea:to-complete", "idea-only") ||
		!hasNamedTokenInPlace(runtimeSnap.Marking, "task:to-complete", "task-only") {
		t.Fatalf("runtime marking for mismatched pair = %#v", runtimeSnap.Marking.PlaceTokens)
	}
	if !hasNamedWorkItemAtPlace(projectionView, "idea:to-complete", "idea-only") ||
		!hasNamedWorkItemAtPlace(projectionView, "task:to-complete", "task-only") {
		t.Fatalf("projection queue for mismatched pair = %#v; classification=%q",
			projectionView.Runtime.CurrentWorkItemsByPlaceID, consumePathLayerProjectionVisibilityGap)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func submitConsumePathPair(t *testing.T, h *testutil.ServiceTestHarness, cellName string) {
	t.Helper()

	for _, req := range []work.SubmitRequest{
		{
			Name:        cellName,
			WorkTypeID:  "idea",
			TargetState: "to-complete",
			TraceID:     "trace-idea-" + cellName,
		},
		{
			Name:        cellName,
			WorkTypeID:  "task",
			TargetState: "to-complete",
			TraceID:     "trace-task-" + cellName,
		},
	} {
		h.SubmitFull(context.Background(), []work.SubmitRequest{req})
	}
}

// submitSameNameOrphanAfterConsumePattern injects a reviewed same-name pair,
// waits for consume to finish, then submits the duplicate orphan task. Service
// mode is required so the runtime accepts the post-consume orphan submission.
func submitSameNameOrphanAfterConsumePattern(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	cellName string,
	orphanTraceID string,
) {
	t.Helper()

	h.SubmitFull(context.Background(), []work.SubmitRequest{{
		Name:        cellName,
		WorkTypeID:  "idea",
		TargetState: "to-complete",
		TraceID:     "trace-idea-" + cellName,
	}})
	h.SubmitFull(context.Background(), []work.SubmitRequest{{
		Name:        cellName,
		WorkTypeID:  "task",
		TargetState: "to-complete",
		TraceID:     "trace-task-a-" + cellName,
	}})
	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)
	h.SubmitFull(context.Background(), []work.SubmitRequest{{
		Name:        cellName,
		WorkTypeID:  "task",
		TargetState: "to-complete",
		TraceID:     orphanTraceID,
	}})
}

func newSameNameConsumePathServiceHarness(t *testing.T, dir string) *testutil.ServiceTestHarness {
	t.Helper()
	return support.NewGuardsBatchHarness(t, dir)
}

func classifyConsumePathOutcome(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	cellName string,
) consumePathOwnershipLayer {
	t.Helper()

	runtimeSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	runtimeMarking := runtimeSnap.Marking

	projectionView, err := consumePathProjectionView(t, h)
	if err != nil {
		t.Fatalf("consumePathProjectionView: %v", err)
	}

	runtimeIdeaToComplete := hasNamedTokenInPlace(runtimeMarking, "idea:to-complete", cellName)
	runtimeTaskToComplete := hasNamedTokenInPlace(runtimeMarking, "task:to-complete", cellName)
	runtimeIdeaComplete := hasNamedTokenInPlace(runtimeMarking, "idea:complete", cellName)
	runtimeTaskComplete := hasNamedTokenInPlace(runtimeMarking, "task:complete", cellName)

	projectionIdeaToComplete := hasNamedWorkItemAtPlace(projectionView, "idea:to-complete", cellName)
	projectionTaskToComplete := hasNamedWorkItemAtPlace(projectionView, "task:to-complete", cellName)

	if runtimeIdeaToComplete != projectionIdeaToComplete || runtimeTaskToComplete != projectionTaskToComplete {
		return consumePathLayerProjectionVisibilityGap
	}

	switch {
	case runtimeIdeaComplete && runtimeTaskComplete && !runtimeIdeaToComplete && !runtimeTaskToComplete:
		return consumePathLayerRuntimeCorrect
	case runtimeTaskToComplete && !runtimeIdeaToComplete:
		return consumePathLayerHistoricalQueueArtifact
	case runtimeIdeaToComplete && runtimeTaskToComplete:
		return consumePathLayerRuntimeCorrect
	default:
		t.Fatalf("unclassified consume-path outcome for %q: runtime=%#v", cellName, runtimeMarking.PlaceTokens)
		return ""
	}
}

func consumePathProjectionView(t *testing.T, h *testutil.ServiceTestHarness) (interfaces.FactoryWorldView, error) {
	t.Helper()

	ctx := context.Background()
	events, err := h.GetFactoryEvents(ctx)
	if err != nil {
		return interfaces.FactoryWorldView{}, err
	}
	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		return interfaces.FactoryWorldView{}, err
	}
	worldState, err := factoryeventprojection.ReconstructFactoryWorldState(events, snapshot.TickCount)
	if err != nil {
		return interfaces.FactoryWorldView{}, err
	}
	return projections.BuildFactoryWorldView(worldState), nil
}

func hasNamedWorkItemAtPlace(view interfaces.FactoryWorldView, placeID, name string) bool {
	for _, ref := range view.Runtime.CurrentWorkItemsByPlaceID[placeID] {
		if ref.DisplayName == name {
			return true
		}
	}
	return false
}

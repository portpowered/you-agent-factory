package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// historicalManualRepairPreconditions records the observable evidence required
// before a bounded historical repair move is allowed for a stranded same-name
// consume cell. This is not a generic completion shortcut: it applies only
// when the idea twin already reached idea:complete and the remaining
// task:to-complete token is an orphan duplicate from earlier recovery residue.
type historicalManualRepairPreconditions struct {
	IdeaCompletePresent   bool
	TaskToCompletePresent bool
	IdeaToCompleteAbsent  bool
}

func (p historicalManualRepairPreconditions) AllowsBoundedHistoricalRepair() bool {
	return p.IdeaCompletePresent &&
		p.TaskToCompletePresent &&
		p.IdeaToCompleteAbsent
}

func evaluateHistoricalManualRepairPreconditions(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	cellName string,
) historicalManualRepairPreconditions {
	t.Helper()

	runtimeSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	runtimeIdeaToComplete := hasNamedTokenInPlace(runtimeSnap.Marking, "idea:to-complete", cellName)
	runtimeTaskToComplete := hasNamedTokenInPlace(runtimeSnap.Marking, "task:to-complete", cellName)

	return historicalManualRepairPreconditions{
		IdeaCompletePresent:   hasNamedTokenInPlace(runtimeSnap.Marking, "idea:complete", cellName),
		TaskToCompletePresent: runtimeTaskToComplete,
		IdeaToCompleteAbsent:  !runtimeIdeaToComplete,
	}
}

func workIDForNamedTokenAtPlace(marking petri.MarkingSnapshot, placeID, name string) (string, bool) {
	for _, token := range marking.Tokens {
		if token.PlaceID == placeID && token.Color.Name == name && token.Color.WorkID != "" {
			return token.Color.WorkID, true
		}
	}
	return "", false
}

func TestSameNameConsumePathHistoricalRecovery_HiddenIdeaTwinAtComplete_ClassifiesAsArtifact(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	const cellName = "dynamic-workflows-cell-cli-validate-list"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	submitSameNameOrphanAfterConsumePattern(t, h, cellName, "trace-orphan-"+cellName)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)
	time.Sleep(150 * time.Millisecond)

	layer := classifyConsumePathOutcome(t, h, cellName)
	if layer != consumePathLayerHistoricalQueueArtifact {
		t.Fatalf("ownership layer = %q, want %q for hidden idea twin at complete", layer, consumePathLayerHistoricalQueueArtifact)
	}

	preconditions := evaluateHistoricalManualRepairPreconditions(t, h, cellName)
	if !preconditions.AllowsBoundedHistoricalRepair() {
		t.Fatalf("historical repair preconditions = %#v, want all true for hidden twin orphan", preconditions)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathHistoricalRecovery_LivePairWaitingForConsume_IsNotHistoricalArtifact(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	const cellName = "dynamic-workflows-cell-cli-run-status-result"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:        cellName,
		WorkTypeID:  "idea",
		TargetState: "to-complete",
		TraceID:     "trace-idea-" + cellName,
	}})
	support.WaitForHarnessPlaceTokenCount(t, h, "idea:to-complete", 1, 3*time.Second)

	preconditions := evaluateHistoricalManualRepairPreconditions(t, h, cellName)
	if preconditions.AllowsBoundedHistoricalRepair() {
		t.Fatalf("historical repair preconditions = %#v, want blocked while idea twin is still queued", preconditions)
	}

	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:        cellName,
		WorkTypeID:  "task",
		TargetState: "to-complete",
		TraceID:     "trace-task-" + cellName,
	}})
	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)

	layer := classifyConsumePathOutcome(t, h, cellName)
	if layer != consumePathLayerRuntimeCorrect {
		t.Fatalf("ownership layer = %q, want %q after live pair consumes", layer, consumePathLayerRuntimeCorrect)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathHistoricalRecovery_BoundedManualMoveCompletesOrphanTask(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	const cellName = "dynamic-workflows-cell-mcp-tools"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	submitSameNameOrphanAfterConsumePattern(t, h, cellName, "trace-manual-repair-"+cellName)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)

	preconditions := evaluateHistoricalManualRepairPreconditions(t, h, cellName)
	if !preconditions.AllowsBoundedHistoricalRepair() {
		t.Fatalf("historical repair preconditions = %#v, want allowed before manual move", preconditions)
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	orphanWorkID, ok := workIDForNamedTokenAtPlace(snapshot.Marking, "task:to-complete", cellName)
	if !ok {
		t.Fatalf("missing orphan task token for %q in %#v", cellName, snapshot.Marking.Tokens)
	}

	moveResult, err := h.MoveWork(context.Background(), orphanWorkID, "complete")
	if err != nil {
		t.Fatalf("MoveWork orphan task to complete: %v", err)
	}
	if moveResult.FromState != "to-complete" || moveResult.ToState != "complete" {
		t.Fatalf("move result = %#v, want task:to-complete -> task:complete", moveResult)
	}

	h.Assert().
		PlaceTokenCount("task:complete", 2).
		HasNoTokenInPlace("task:to-complete")

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathHistoricalRecovery_UnrelatedLanesCompleteWithOrphanPresent(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	const (
		orphanCell = "dynamic-workflows-cell-cli-validate-list"
		liveCell   = "unrelated-reviewed-cell"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	submitSameNameOrphanAfterConsumePattern(t, h, orphanCell, "trace-orphan-unrelated")
	submitConsumePathPair(t, h, liveCell)
	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 2, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 2, 3*time.Second)

	h.Assert().
		PlaceTokenCount("task:to-complete", 1).
		HasNoTokenInPlace("idea:to-complete")

	if layer := classifyConsumePathOutcome(t, h, orphanCell); layer != consumePathLayerHistoricalQueueArtifact {
		t.Fatalf("orphan cell layer = %q, want %q", layer, consumePathLayerHistoricalQueueArtifact)
	}
	if layer := classifyConsumePathOutcome(t, h, liveCell); layer != consumePathLayerRuntimeCorrect {
		t.Fatalf("live cell layer = %q, want %q", layer, consumePathLayerRuntimeCorrect)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathHistoricalRecovery_TaskOnlyWithoutIdeaTwin_BlocksBoundedRepair(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	const cellName = "dynamic-workflows-cell-cli-run-status-result"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:        cellName,
		WorkTypeID:  "task",
		TargetState: "to-complete",
		TraceID:     "trace-task-only-repair",
	}})

	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)
	time.Sleep(150 * time.Millisecond)

	layer := classifyConsumePathOutcome(t, h, cellName)
	if layer != consumePathLayerHistoricalQueueArtifact {
		t.Fatalf("ownership layer = %q, want %q for task-only residue", layer, consumePathLayerHistoricalQueueArtifact)
	}

	preconditions := evaluateHistoricalManualRepairPreconditions(t, h, cellName)
	if preconditions.AllowsBoundedHistoricalRepair() {
		t.Fatalf("historical repair preconditions = %#v, want blocked without idea:complete twin", preconditions)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

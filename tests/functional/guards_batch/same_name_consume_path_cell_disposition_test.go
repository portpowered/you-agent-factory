package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// consumePathCellDisposition records the follow-up planner action for a
// reviewed dynamic workflow cell stranded at task:to-complete. Story 004 uses
// observable runtime and projection outcomes to classify each PRD cell without
// introducing new customer-facing vocabulary.
type consumePathCellDisposition string

const (
	cellDispositionComplete                   consumePathCellDisposition = "complete"
	cellDispositionNeedsBoundedManualMove     consumePathCellDisposition = "needs_bounded_manual_move"
	cellDispositionNeedsBoundedFollowupRepair consumePathCellDisposition = "needs_bounded_followup_repair"
	consumePathCellTestTimeout                                           = 15 * time.Second
	consumePathCellWaitTimeout                                           = 3 * time.Second
)

// consumePathCellDispositionEvidence is reviewer-verifiable output for a
// stranded CLI or MCP implementation cell. Each field maps to an observable
// runtime check from the ownership and historical-recovery tests.
type consumePathCellDispositionEvidence struct {
	CellName               string
	Disposition            consumePathCellDisposition
	OwnershipLayer         consumePathOwnershipLayer
	ManualRepairAllowed    bool
	ExpectedPostRepairTask string
}

func reproduceLiveQueueOrphanPattern(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	cellName string,
) {
	t.Helper()

	submitSameNameOrphanAfterConsumePattern(t, h, cellName, "trace-orphan-"+cellName)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, consumePathCellWaitTimeout)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, consumePathCellWaitTimeout)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, consumePathCellWaitTimeout)
	time.Sleep(150 * time.Millisecond)
}

func evaluateConsumePathCellDisposition(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	cellName string,
) consumePathCellDispositionEvidence {
	t.Helper()

	layer := classifyConsumePathOutcome(t, h, cellName)
	preconditions := evaluateHistoricalManualRepairPreconditions(t, h, cellName)

	evidence := consumePathCellDispositionEvidence{
		CellName:               cellName,
		OwnershipLayer:         layer,
		ManualRepairAllowed:    preconditions.AllowsBoundedHistoricalRepair(),
		ExpectedPostRepairTask: "task:complete",
	}

	switch {
	case layer == consumePathLayerRuntimeCorrect:
		evidence.Disposition = cellDispositionComplete
	case layer == consumePathLayerHistoricalQueueArtifact && preconditions.AllowsBoundedHistoricalRepair():
		evidence.Disposition = cellDispositionNeedsBoundedManualMove
	case layer == consumePathLayerHistoricalQueueArtifact && !preconditions.AllowsBoundedHistoricalRepair():
		evidence.Disposition = cellDispositionNeedsBoundedFollowupRepair
	default:
		t.Fatalf("unclassified disposition for %q: layer=%q preconditions=%#v", cellName, layer, preconditions)
	}

	return evidence
}

func TestSameNameConsumePathCellDisposition_ReviewedCLIAndMCPCells(t *testing.T) {
	cases := []struct {
		cellName    string
		disposition consumePathCellDisposition
	}{
		{
			cellName:    "dynamic-workflows-cell-cli-validate-list",
			disposition: cellDispositionNeedsBoundedManualMove,
		},
		{
			cellName:    "dynamic-workflows-cell-cli-run-status-result",
			disposition: cellDispositionNeedsBoundedManualMove,
		},
		{
			cellName:    "dynamic-workflows-cell-mcp-tools",
			disposition: cellDispositionNeedsBoundedManualMove,
		},
	}

	for _, tc := range cases {
		t.Run(tc.cellName, func(t *testing.T) {
			dir := scaffoldConsumePathFactoryBuiltInOrder(t)
			h := newSameNameConsumePathServiceHarness(t, dir)

			ctx, cancel := context.WithTimeout(context.Background(), consumePathCellTestTimeout)
			defer cancel()
			errCh := support.RunGuardsBatchHarness(t, h, ctx)

			reproduceLiveQueueOrphanPattern(t, h, tc.cellName)

			h.Assert().
				PlaceTokenCount("idea:complete", 1).
				PlaceTokenCount("task:complete", 1).
				PlaceTokenCount("task:to-complete", 1).
				HasNoTokenInPlace("idea:to-complete")

			evidence := evaluateConsumePathCellDisposition(t, h, tc.cellName)
			if evidence.Disposition != tc.disposition {
				t.Fatalf("disposition = %q, want %q; evidence=%#v", evidence.Disposition, tc.disposition, evidence)
			}
			if evidence.OwnershipLayer != consumePathLayerHistoricalQueueArtifact {
				t.Fatalf("ownership layer = %q, want %q", evidence.OwnershipLayer, consumePathLayerHistoricalQueueArtifact)
			}
			if !evidence.ManualRepairAllowed {
				t.Fatalf("manual repair preconditions not met for %q: %#v", tc.cellName, evidence)
			}

			cancel()
			if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("factory run error: %v", err)
			}
		})
	}
}

func TestSameNameConsumePathCellDisposition_ManualMoveReachesExpectedPostRepairState(t *testing.T) {
	const cellName = "dynamic-workflows-cell-mcp-tools"

	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), consumePathCellTestTimeout)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	reproduceLiveQueueOrphanPattern(t, h, cellName)

	evidence := evaluateConsumePathCellDisposition(t, h, cellName)
	if evidence.Disposition != cellDispositionNeedsBoundedManualMove {
		t.Fatalf("disposition = %q, want %q before manual move", evidence.Disposition, cellDispositionNeedsBoundedManualMove)
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	orphanWorkID, ok := workIDForNamedTokenAtPlace(snapshot.Marking, "task:to-complete", cellName)
	if !ok {
		t.Fatalf("missing orphan task token for %q", cellName)
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

	postRepair := evaluateConsumePathCellDisposition(t, h, cellName)
	if postRepair.Disposition != cellDispositionComplete {
		t.Fatalf("post-repair disposition = %q, want %q; evidence=%#v", postRepair.Disposition, cellDispositionComplete, postRepair)
	}
	if postRepair.OwnershipLayer != consumePathLayerRuntimeCorrect {
		t.Fatalf("post-repair ownership layer = %q, want %q", postRepair.OwnershipLayer, consumePathLayerRuntimeCorrect)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathCellDisposition_FreshReviewedPairIsCompleteWithoutManualMove(t *testing.T) {
	const cellName = "dynamic-workflows-cell-cli-validate-list"

	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := newSameNameConsumePathServiceHarness(t, dir)

	submitConsumePathPair(t, h, cellName)

	ctx, cancel := context.WithTimeout(context.Background(), consumePathCellTestTimeout)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, consumePathCellWaitTimeout)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, consumePathCellWaitTimeout)

	h.Assert().
		HasNoTokenInPlace("idea:to-complete").
		HasNoTokenInPlace("task:to-complete")

	evidence := evaluateConsumePathCellDisposition(t, h, cellName)
	if evidence.Disposition != cellDispositionComplete {
		t.Fatalf("disposition = %q, want %q for fresh reviewed pair", evidence.Disposition, cellDispositionComplete)
	}
	if evidence.ManualRepairAllowed {
		t.Fatalf("manual repair should not be allowed for fresh reviewed pair: %#v", evidence)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

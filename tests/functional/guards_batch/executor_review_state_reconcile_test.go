package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestExecutorReviewStateReconcile_ReviewCompletionCollapsesDuplicateReviewInit(t *testing.T) {
	dir := scaffoldExecutorReviewReconcileFactory(t)
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "<COMPLETE>\n"}},
	})
	h := support.NewGuardsBatchHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExecutionBaseDir(dir),
	)

	const (
		laneName = "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene"
		traceID  = "trace-duplicate-review-reconcile"
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	submitExecutorReviewDuplicateAndStalePattern(t, h, laneName, traceID)

	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "review:complete", 1, 3*time.Second)

	h.Assert().
		HasNoTokenInPlace("review:init").
		HasNoTokenInPlace("task:in-review")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if count := activeReviewInitForTrace(snapshot, traceID); count != 0 {
		t.Fatalf("active review:init for trace %q = %d, want 0", traceID, count)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func scaffoldExecutorReviewReconcileFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "executor_review_state_reconcile",
		"workers": []map[string]any{
			{
				"name":             "processor",
				"type":             "MODEL_WORKER",
				"modelProvider":    "CURSOR",
				"executorProvider": "SCRIPT_WRAP",
				"stopToken":        "<COMPLETE>",
			},
		},
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "in-review", "type": "INITIAL"},
					{"name": "to-complete", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "review",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "fin", "type": "FAILED"},
				},
			},
		},
		"workstations": []map[string]any{
			{
				"name":   "review",
				"type":   "MODEL_WORKSTATION",
				"worker": "processor",
				"inputs": []map[string]any{
					{"workType": "task", "state": "in-review"},
					{"workType": "review", "state": "init"},
				},
				"outputs": []map[string]any{
					{"workType": "task", "state": "to-complete"},
					{"workType": "review", "state": "complete"},
				},
				"onRejection": []map[string]any{
					{"workType": "task", "state": "init"},
				},
				"onFailure": []map[string]any{
					{"workType": "task", "state": "failed"},
					{"workType": "review", "state": "fin"},
				},
			},
		},
	})
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: MODEL_WORKER\nmodelProvider: CURSOR\nexecutorProvider: SCRIPT_WRAP\nstopToken: <COMPLETE>\n---\nReview reconciled recovery lanes.\n")
	support.WriteWorkstationConfig(t, dir, "review", "---\ntype: MODEL_WORKSTATION\n---\nReview reconciled recovery lanes.\n")
	return dir
}

func submitExecutorReviewDuplicateAndStalePattern(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	laneName string,
	traceID string,
) {
	t.Helper()

	// Submit each work item in its own batch so shared lane names are not
	// uniquified by WorkRequestFromSubmitRequests (duplicate review residue
	// keeps the same Color.Name as the active task/review dispatch).
	requests := []work.SubmitRequest{
		{
			Name:                   laneName,
			WorkTypeID:             "task",
			TargetState:            "in-review",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     2,
		},
		{
			WorkID:                 "work-review-64",
			Name:                   laneName,
			WorkTypeID:             "review",
			TargetState:            "init",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     3,
		},
		{
			WorkID:                 "work-review-65",
			Name:                   laneName,
			WorkTypeID:             "review",
			TargetState:            "init",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     3,
		},
		{
			WorkID:                 "work-task-stale-init",
			Name:                   laneName,
			WorkTypeID:             "task",
			TargetState:            "init",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     2,
		},
		{
			WorkID:                 "work-task-stale-failed",
			Name:                   laneName,
			WorkTypeID:             "task",
			TargetState:            "failed",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     2,
		},
	}
	for _, req := range requests {
		h.SubmitFull(context.Background(), []work.SubmitRequest{req})
	}
}

func activeReviewInitForTrace(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], traceID string) int {
	if snapshot == nil {
		return 0
	}
	count := 0
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.PlaceID != "review:init" {
			continue
		}
		if token.Color.CurrentChainingTraceID != traceID && token.Color.TraceID != traceID {
			continue
		}
		count++
	}
	return count
}

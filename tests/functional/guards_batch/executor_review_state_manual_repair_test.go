package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestExecutorReviewManualRepair_LiveFailedPostProcessingLanesAllowBoundedMove(t *testing.T) {
	cases := []struct {
		laneName string
		items    []queueWorkSnapshot
		taskID   string
	}{
		{
			laneName: "dynamic-workflows-recovery-session-backend-runtime",
			items:    liveSessionBackendRuntimeQueueEvidence,
			taskID:   "work-task-24",
		},
		{
			laneName: "dynamic-workflows-recovery-mcp-install-plan-scope",
			items:    liveMCPInstallPlanScopeQueueEvidence,
			taskID:   "work-task-58",
		},
	}

	for _, tc := range cases {
		t.Run(tc.laneName, func(t *testing.T) {
			classification, err := classifyExecutorReviewLaneFromQueueEvidence(tc.laneName, tc.items, true)
			if err != nil {
				t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
			}
			preconditions := evaluateExecutorReviewManualRepairPreconditions(classification, tc.items)
			if !preconditions.AllowsBoundedExecutorReviewManualRepair() {
				t.Fatalf("manual repair preconditions = %#v, want allowed for %q", preconditions, tc.laneName)
			}
			if classification.TaskWorkID != tc.taskID {
				t.Fatalf("task work id = %q, want %q", classification.TaskWorkID, tc.taskID)
			}
		})
	}
}

func TestExecutorReviewManualRepair_DuplicateReviewLaneBlocksManualMove(t *testing.T) {
	const laneName = "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene"

	classification, err := classifyExecutorReviewLaneFromQueueEvidence(
		laneName,
		liveSetupWorkspaceGitPullHygieneQueueEvidence,
		true,
	)
	if err != nil {
		t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
	}
	if classification.PlannerDisposition != executorReviewDispositionNeedsRuntimeReconcile {
		t.Fatalf("planner disposition = %q, want %q", classification.PlannerDisposition, executorReviewDispositionNeedsRuntimeReconcile)
	}

	preconditions := evaluateExecutorReviewManualRepairPreconditions(classification, liveSetupWorkspaceGitPullHygieneQueueEvidence)
	if preconditions.AllowsBoundedExecutorReviewManualRepair() {
		t.Fatalf("manual repair preconditions = %#v, want blocked for duplicate review lane", preconditions)
	}
}

func TestExecutorReviewManualRepair_IncompleteWorktreeBlocksBoundedMove(t *testing.T) {
	classification, err := classifyExecutorReviewLaneFromQueueEvidence(
		"dynamic-workflows-recovery-session-backend-runtime",
		liveSessionBackendRuntimeQueueEvidence,
		false,
	)
	if err != nil {
		t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
	}

	preconditions := evaluateExecutorReviewManualRepairPreconditions(classification, liveSessionBackendRuntimeQueueEvidence)
	if preconditions.AllowsBoundedExecutorReviewManualRepair() {
		t.Fatalf("manual repair preconditions = %#v, want blocked when worktree is incomplete", preconditions)
	}
}

func TestExecutorReviewManualRepair_BoundedMoveFailedTaskBackToInit(t *testing.T) {
	dir := scaffoldExecutorReviewManualRepairFactory(t)
	h := support.NewGuardsBatchHarness(t, dir)

	const (
		laneName = "lane-failed-post-processing"
		traceID  = "trace-failed-post-processing"
		taskID   = "work-task-failed"
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	injectFailedPostProcessingPattern(t, h, laneName, traceID, taskID)
	h.SubmitFull(context.Background(), []work.SubmitRequest{{
		Name:                   "keep-engine-alive",
		WorkTypeID:             "task",
		TargetState:            "in-review",
		TraceID:                "trace-keep-alive",
		CurrentChainingTraceID: "trace-keep-alive",
	}})

	support.WaitForHarnessPlaceTokenCount(t, h, "task:failed", 1, time.Second)

	moveResult, err := h.MoveWork(context.Background(), taskID, "init")
	if err != nil {
		t.Fatalf("MoveWork failed task to init: %v", err)
	}
	if moveResult.FromState != "failed" || moveResult.ToState != "init" {
		t.Fatalf("move result = %#v, want task:failed -> task:init", moveResult)
	}

	h.Assert().HasNoTokenInPlace("task:failed")

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func scaffoldExecutorReviewManualRepairFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "executor_review_manual_repair",
		"workTypes": []map[string]any{
			{
				"name": "plan",
				"states": []map[string]any{
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
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
		},
	})
}

func injectFailedPostProcessingPattern(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	laneName string,
	traceID string,
	taskID string,
) {
	t.Helper()

	h.SubmitFull(context.Background(), []work.SubmitRequest{
		{
			Name:                   laneName,
			WorkTypeID:             "plan",
			TargetState:            "complete",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     1,
		},
		{
			WorkID:                 taskID,
			Name:                   laneName,
			WorkTypeID:             "task",
			TargetState:            "failed",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     2,
		},
	})
}

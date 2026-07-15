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

// Live queue evidence captured from `you work list --session '~default' --name <lane> --json`
// on 2026-06-15 UTC against the default factory session.
var (
	liveSessionBackendRuntimeQueueEvidence = []queueWorkSnapshot{
		{
			WorkID:                 "batch-dynamic-workflows-session-backend-recovery-20260614-dynamic-workflows-recovery-session-backend-runtime",
			Name:                   "dynamic-workflows-recovery-session-backend-runtime",
			WorkTypeName:           "idea",
			StateName:              "to-complete",
			StateType:              "PROCESSING",
			TraceID:                "trace-dynamic-workflows-session-backend-recovery-20260614",
			CurrentChainingTraceID: "trace-dynamic-workflows-session-backend-recovery-20260614",
			ChainingTraceDepth:     1,
		},
		{
			WorkID:                 "work-plan-23",
			Name:                   "dynamic-workflows-recovery-session-backend-runtime",
			WorkTypeName:           "plan",
			StateName:              "complete",
			StateType:              "TERMINAL",
			TraceID:                "trace-dynamic-workflows-session-backend-recovery-20260614",
			CurrentChainingTraceID: "trace-dynamic-workflows-session-backend-recovery-20260614",
			ChainingTraceDepth:     2,
		},
		{
			WorkID:                 "work-task-24",
			Name:                   "dynamic-workflows-recovery-session-backend-runtime",
			WorkTypeName:           "task",
			StateName:              "failed",
			StateType:              "FAILED",
			TraceID:                "trace-dynamic-workflows-session-backend-recovery-20260614",
			CurrentChainingTraceID: "trace-dynamic-workflows-session-backend-recovery-20260614",
			ChainingTraceDepth:     3,
		},
	}

	liveMCPInstallPlanScopeQueueEvidence = []queueWorkSnapshot{
		{
			WorkID:                 "batch-dynamic-workflows-mcp-install-plan-recovery-20260615-dynamic-workflows-recovery-mcp-install-plan-scope",
			Name:                   "dynamic-workflows-recovery-mcp-install-plan-scope",
			WorkTypeName:           "idea",
			StateName:              "to-complete",
			StateType:              "PROCESSING",
			TraceID:                "trace-dynamic-workflows-mcp-install-plan-recovery-20260615",
			CurrentChainingTraceID: "trace-dynamic-workflows-mcp-install-plan-recovery-20260615",
			ChainingTraceDepth:     1,
		},
		{
			WorkID:                 "work-plan-42",
			Name:                   "dynamic-workflows-recovery-mcp-install-plan-scope",
			WorkTypeName:           "plan",
			StateName:              "failed",
			StateType:              "FAILED",
			TraceID:                "trace-dynamic-workflows-mcp-install-plan-recovery-20260615",
			CurrentChainingTraceID: "trace-dynamic-workflows-mcp-install-plan-recovery-20260615",
			ChainingTraceDepth:     2,
		},
		{
			WorkID:                 "batch-request-3cd2c196a6298845ba8df93e3da96747-dynamic-workflows-recovery-mcp-install-plan-scope",
			Name:                   "dynamic-workflows-recovery-mcp-install-plan-scope",
			WorkTypeName:           "plan",
			StateName:              "complete",
			StateType:              "TERMINAL",
			TraceID:                "trace-385de5ce7824a0a692250026d9388463",
			CurrentChainingTraceID: "trace-385de5ce7824a0a692250026d9388463",
			ChainingTraceDepth:     1,
		},
		{
			WorkID:                 "work-task-58",
			Name:                   "dynamic-workflows-recovery-mcp-install-plan-scope",
			WorkTypeName:           "task",
			StateName:              "failed",
			StateType:              "FAILED",
			TraceID:                "trace-385de5ce7824a0a692250026d9388463",
			CurrentChainingTraceID: "trace-385de5ce7824a0a692250026d9388463",
			ChainingTraceDepth:     2,
		},
	}

	liveSetupWorkspaceGitPullHygieneQueueEvidence = []queueWorkSnapshot{
		{
			WorkID:                 "batch-dynamic-workflows-setup-workspace-recovery-20260615-dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			Name:                   "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			WorkTypeName:           "idea",
			StateName:              "to-complete",
			StateType:              "PROCESSING",
			TraceID:                "trace-dynamic-workflows-setup-workspace-recovery-20260615",
			CurrentChainingTraceID: "trace-dynamic-workflows-setup-workspace-recovery-20260615",
			ChainingTraceDepth:     1,
		},
		{
			WorkID:                 "work-plan-49",
			Name:                   "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			WorkTypeName:           "plan",
			StateName:              "failed",
			StateType:              "FAILED",
			TraceID:                "trace-dynamic-workflows-setup-workspace-recovery-20260615",
			CurrentChainingTraceID: "trace-dynamic-workflows-setup-workspace-recovery-20260615",
			ChainingTraceDepth:     2,
		},
		{
			WorkID:                 "batch-request-ff69335cae42b911f05ad8e790fb207d-dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			Name:                   "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			WorkTypeName:           "plan",
			StateName:              "complete",
			StateType:              "TERMINAL",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
			ChainingTraceDepth:     1,
		},
		{
			WorkID:                 "work-task-59",
			Name:                   "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			WorkTypeName:           "task",
			StateName:              "in-review",
			StateType:              "PROCESSING",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
			ChainingTraceDepth:     2,
		},
		{
			WorkID:                 "work-review-64",
			Name:                   "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			WorkTypeName:           "review",
			StateName:              "init",
			StateType:              "PROCESSING",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
			ChainingTraceDepth:     3,
		},
		{
			WorkID:                 "work-review-65",
			Name:                   "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			WorkTypeName:           "review",
			StateName:              "init",
			StateType:              "INITIAL",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
			ChainingTraceDepth:     3,
		},
		{
			WorkID:                 "work-review-69",
			Name:                   "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			WorkTypeName:           "review",
			StateName:              "init",
			StateType:              "INITIAL",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
			ChainingTraceDepth:     3,
		},
	}
)

func TestExecutorReviewLaneEvidence_LiveQueueSnapshotsClassifyThreePRDLanes(t *testing.T) {
	cases := []struct {
		laneName    string
		items       []queueWorkSnapshot
		wantCause   executorReviewMismatchCause
		wantDisp    executorReviewPlannerDisposition
		wantTaskID  string
		wantReviews []string
	}{
		{
			laneName:    "dynamic-workflows-recovery-session-backend-runtime",
			items:       liveSessionBackendRuntimeQueueEvidence,
			wantCause:   executorReviewCauseFailedPostProcessing,
			wantDisp:    executorReviewDispositionSafeManualRepair,
			wantTaskID:  "work-task-24",
			wantReviews: nil,
		},
		{
			laneName:    "dynamic-workflows-recovery-mcp-install-plan-scope",
			items:       liveMCPInstallPlanScopeQueueEvidence,
			wantCause:   executorReviewCauseFailedPostProcessing,
			wantDisp:    executorReviewDispositionSafeManualRepair,
			wantTaskID:  "work-task-58",
			wantReviews: nil,
		},
		{
			laneName:    "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			items:       liveSetupWorkspaceGitPullHygieneQueueEvidence,
			wantCause:   executorReviewCauseDuplicateReviewCreation,
			wantDisp:    executorReviewDispositionNeedsRuntimeReconcile,
			wantTaskID:  "work-task-59",
			wantReviews: []string{"work-review-64", "work-review-65", "work-review-69"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.laneName, func(t *testing.T) {
			got, err := classifyExecutorReviewLaneFromQueueEvidence(tc.laneName, tc.items, true)
			if err != nil {
				t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
			}
			if got.MismatchCause != tc.wantCause {
				t.Fatalf("mismatch cause = %q, want %q; evidence=%#v", got.MismatchCause, tc.wantCause, got)
			}
			if got.PlannerDisposition != tc.wantDisp {
				t.Fatalf("planner disposition = %q, want %q; evidence=%#v", got.PlannerDisposition, tc.wantDisp, got)
			}
			if got.TaskWorkID != tc.wantTaskID {
				t.Fatalf("task work id = %q, want %q", got.TaskWorkID, tc.wantTaskID)
			}
			if len(got.ReviewWorkIDs) != len(tc.wantReviews) {
				t.Fatalf("review work ids = %#v, want %#v", got.ReviewWorkIDs, tc.wantReviews)
			}
			for i, wantID := range tc.wantReviews {
				if got.ReviewWorkIDs[i] != wantID {
					t.Fatalf("review work ids[%d] = %q, want %q", i, got.ReviewWorkIDs[i], wantID)
				}
			}
		})
	}
}

func TestExecutorReviewLaneEvidence_ReproducedShapesClassifyLikeLiveQueue(t *testing.T) {
	const laneName = "dynamic-workflows-recovery-session-backend-runtime"
	reproduced := []queueWorkSnapshot{
		{
			WorkID:                 "batch-dynamic-workflows-session-backend-recovery-20260614-dynamic-workflows-recovery-session-backend-runtime",
			Name:                   laneName,
			WorkTypeName:           "idea",
			StateName:              "to-complete",
			StateType:              "PROCESSING",
			TraceID:                "trace-dynamic-workflows-session-backend-recovery-20260614",
			CurrentChainingTraceID: "trace-dynamic-workflows-session-backend-recovery-20260614",
		},
		{
			WorkID:                 "work-plan-23",
			Name:                   laneName,
			WorkTypeName:           "plan",
			StateName:              "complete",
			StateType:              "TERMINAL",
			TraceID:                "trace-dynamic-workflows-session-backend-recovery-20260614",
			CurrentChainingTraceID: "trace-dynamic-workflows-session-backend-recovery-20260614",
		},
		{
			WorkID:                 "work-task-24",
			Name:                   laneName,
			WorkTypeName:           "task",
			StateName:              "failed",
			StateType:              "FAILED",
			TraceID:                "trace-dynamic-workflows-session-backend-recovery-20260614",
			CurrentChainingTraceID: "trace-dynamic-workflows-session-backend-recovery-20260614",
		},
	}

	classification, err := classifyExecutorReviewLaneFromQueueEvidence(laneName, reproduced, true)
	if err != nil {
		t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
	}
	if classification.MismatchCause != executorReviewCauseFailedPostProcessing {
		t.Fatalf("mismatch cause = %q, want %q", classification.MismatchCause, executorReviewCauseFailedPostProcessing)
	}
	if classification.PlannerDisposition != executorReviewDispositionSafeManualRepair {
		t.Fatalf("planner disposition = %q, want %q", classification.PlannerDisposition, executorReviewDispositionSafeManualRepair)
	}
}

func TestExecutorReviewLaneEvidence_DuplicateReviewShapeClassifiesForRuntimeReconcile(t *testing.T) {
	const laneName = "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene"
	reproduced := []queueWorkSnapshot{
		{
			WorkID:                 "work-task-59",
			Name:                   laneName,
			WorkTypeName:           "task",
			StateName:              "in-review",
			StateType:              "PROCESSING",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
		},
		{
			WorkID:                 "work-review-64",
			Name:                   laneName,
			WorkTypeName:           "review",
			StateName:              "init",
			StateType:              "PROCESSING",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
		},
		{
			WorkID:                 "work-review-65",
			Name:                   laneName,
			WorkTypeName:           "review",
			StateName:              "init",
			StateType:              "INITIAL",
			TraceID:                "trace-e3bfbf2efbff251737c0df2a5433efb3",
			CurrentChainingTraceID: "trace-e3bfbf2efbff251737c0df2a5433efb3",
		},
	}

	classification, err := classifyExecutorReviewLaneFromQueueEvidence(laneName, reproduced, true)
	if err != nil {
		t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
	}
	if classification.MismatchCause != executorReviewCauseDuplicateReviewCreation {
		t.Fatalf("mismatch cause = %q, want %q", classification.MismatchCause, executorReviewCauseDuplicateReviewCreation)
	}
	if classification.PlannerDisposition != executorReviewDispositionNeedsRuntimeReconcile {
		t.Fatalf("planner disposition = %q, want %q", classification.PlannerDisposition, executorReviewDispositionNeedsRuntimeReconcile)
	}
}

func TestExecutorReviewLaneEvidence_SubmitFullPlacesDuplicateReviewInitTokens(t *testing.T) {
	dir := scaffoldExecutorReviewLaneFactory(t)
	h := support.NewGuardsBatchHarness(t, dir)

	const (
		laneName = "lane-duplicate-review"
		traceID  = "trace-duplicate-review"
	)

	injectDuplicateReviewInitPattern(t, h, laneName, traceID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "review:init", 2, time.Second)

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func scaffoldExecutorReviewLaneFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "executor_review_lane_evidence",
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
				},
			},
		},
	})
}

func injectDuplicateReviewInitPattern(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	laneName string,
	traceID string,
) {
	t.Helper()

	h.SubmitFull(context.Background(), []work.SubmitRequest{
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
	})
}

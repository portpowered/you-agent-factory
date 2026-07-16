package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestExecutorReviewStateLifecycle_ProcessCompletionLeavesSingleReviewInit(t *testing.T) {
	dir := scaffoldExecutorReviewProcessOnlyFactory(t)
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "<COMPLETE>\n"}},
	})
	h := support.NewGuardsBatchHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExecutionBaseDir(dir),
	)

	const (
		laneName = "dynamic-workflows-recovery-session-backend-runtime"
		traceID  = "trace-process-reconcile"
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	submitExecutorReviewProcessResiduePattern(t, h, laneName, traceID)

	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "task:in-review", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "review:init", 1, 3*time.Second)

	h.Assert().
		HasNoTokenInPlace("task:init").
		PlaceTokenCount("review:init", 1)

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if count := activeReviewInitForTrace(snapshot, traceID); count != 1 {
		t.Fatalf("active review:init for trace %q = %d, want 1", traceID, count)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestExecutorReviewStateLifecycle_CompletedReviewClearsResidueAndReplayMatches(t *testing.T) {
	dir := scaffoldExecutorReviewReconcileFactory(t)
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "<COMPLETE>\n"}},
	})
	h := support.NewGuardsBatchHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExecutionBaseDir(dir),
	)

	const (
		laneName = "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene"
		traceID  = "trace-review-reconcile-lifecycle"
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

	assertExecutorReviewReplayProjectionIdempotent(t, h)

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestExecutorReviewStateLifecycle_CompletedExecutorAndReviewConvergeToTerminalOutcome(t *testing.T) {
	dir := scaffoldExecutorReviewLifecycleFactory(t)
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {
			{Content: "<COMPLETE>\n"},
			{Content: "<COMPLETE>\n"},
		},
	})
	h := support.NewGuardsBatchHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExecutionBaseDir(dir),
	)

	const (
		laneName = "dynamic-workflows-recovery-mcp-install-plan-scope"
		traceID  = "trace-executor-review-terminal"
	)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:                   laneName,
		WorkTypeID:             "task",
		TargetState:            "init",
		TraceID:                traceID,
		CurrentChainingTraceID: traceID,
		ChainingTraceDepth:     2,
	}})

	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 5*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "review:complete", 1, 5*time.Second)

	h.Assert().
		HasNoTokenInPlace("review:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	assertExecutorReviewProjectionMatchesRuntime(t, h)

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestExecutorReviewStateLifecycle_DuplicateReviewRegressionMatchesLaneThreeShape(t *testing.T) {
	classification, err := classifyExecutorReviewLaneFromQueueEvidence(
		"dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
		liveSetupWorkspaceGitPullHygieneQueueEvidence,
		true,
	)
	if err != nil {
		t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
	}
	if classification.MismatchCause != executorReviewCauseDuplicateReviewCreation {
		t.Fatalf("mismatch cause = %q, want %q", classification.MismatchCause, executorReviewCauseDuplicateReviewCreation)
	}
	if len(classification.ReviewWorkIDs) < 2 {
		t.Fatalf("review work ids = %#v, want duplicate review:init evidence", classification.ReviewWorkIDs)
	}

	dir := scaffoldExecutorReviewReconcileFactory(t)
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "<COMPLETE>\n"}},
	})
	h := support.NewGuardsBatchHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExecutionBaseDir(dir),
	)

	const traceID = "trace-e3bfbf2efbff251737c0df2a5433efb3"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	submitExecutorReviewDuplicateAndStalePattern(
		t,
		h,
		"dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
		traceID,
	)

	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if count := activeReviewInitForTrace(snapshot, traceID); count != 0 {
		t.Fatalf("post-reconcile review:init for spawned trace = %d, want 0", count)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestExecutorReviewStateLifecycle_ThreeNamedLanesExposePlannerClassificationEvidence(t *testing.T) {
	cases := []struct {
		laneName          string
		items             []queueWorkSnapshot
		wantCause         executorReviewMismatchCause
		wantDisp          executorReviewPlannerDisposition
		requireSpawned    bool
		requireReviews    bool
		requireFailedTask bool
	}{
		{
			laneName:          "dynamic-workflows-recovery-session-backend-runtime",
			items:             liveSessionBackendRuntimeQueueEvidence,
			wantCause:         executorReviewCauseFailedPostProcessing,
			wantDisp:          executorReviewDispositionSafeManualRepair,
			requireFailedTask: true,
		},
		{
			laneName:          "dynamic-workflows-recovery-mcp-install-plan-scope",
			items:             liveMCPInstallPlanScopeQueueEvidence,
			wantCause:         executorReviewCauseFailedPostProcessing,
			wantDisp:          executorReviewDispositionSafeManualRepair,
			requireSpawned:    true,
			requireFailedTask: true,
		},
		{
			laneName:       "dynamic-workflows-recovery-setup-workspace-git-pull-hygiene",
			items:          liveSetupWorkspaceGitPullHygieneQueueEvidence,
			wantCause:      executorReviewCauseDuplicateReviewCreation,
			wantDisp:       executorReviewDispositionNeedsRuntimeReconcile,
			requireSpawned: true,
			requireReviews: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.laneName, func(t *testing.T) {
			got, err := classifyExecutorReviewLaneFromQueueEvidence(tc.laneName, tc.items, true)
			if err != nil {
				t.Fatalf("classifyExecutorReviewLaneFromQueueEvidence: %v", err)
			}
			if got.LaneName != tc.laneName {
				t.Fatalf("lane name = %q, want %q", got.LaneName, tc.laneName)
			}
			if got.RecoveryTraceID == "" {
				t.Fatalf("recovery trace id missing from planner evidence: %#v", got)
			}
			if tc.requireSpawned && got.SpawnedTraceID == "" {
				t.Fatalf("spawned trace id missing from planner evidence: %#v", got)
			}
			if got.TaskWorkID == "" {
				t.Fatalf("task work id missing from planner evidence: %#v", got)
			}
			if tc.requireFailedTask && got.MismatchCause != executorReviewCauseFailedPostProcessing {
				t.Fatalf("mismatch cause = %q, want failed_post_processing evidence", got.MismatchCause)
			}
			if tc.requireReviews && len(got.ReviewWorkIDs) < 2 {
				t.Fatalf("review work ids = %#v, want duplicate review evidence", got.ReviewWorkIDs)
			}
			if got.MismatchCause != tc.wantCause {
				t.Fatalf("mismatch cause = %q, want %q", got.MismatchCause, tc.wantCause)
			}
			if got.PlannerDisposition != tc.wantDisp {
				t.Fatalf("planner disposition = %q, want %q", got.PlannerDisposition, tc.wantDisp)
			}
			if !got.WorktreeComplete {
				t.Fatalf("worktreeComplete = false, want true for live recovery evidence")
			}
		})
	}
}

func scaffoldExecutorReviewProcessOnlyFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "executor_review_process_only",
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
				"name":   "process",
				"type":   "MODEL_WORKSTATION",
				"worker": "processor",
				"inputs": []map[string]any{
					{"workType": "task", "state": "init"},
				},
				"outputs": []map[string]any{
					{"workType": "task", "state": "in-review"},
					{"workType": "review", "state": "init"},
				},
				"onFailure": []map[string]any{
					{"workType": "task", "state": "failed"},
				},
			},
		},
	})
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: MODEL_WORKER\nmodelProvider: CURSOR\nexecutorProvider: SCRIPT_WRAP\nstopToken: <COMPLETE>\n---\nProcess-only executor/review lifecycle verification.\n")
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nProcess-only reconciled recovery lanes.\n")
	return dir
}

func scaffoldExecutorReviewLifecycleFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "executor_review_state_lifecycle",
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
				"name":   "process",
				"type":   "MODEL_WORKSTATION",
				"worker": "processor",
				"inputs": []map[string]any{
					{"workType": "task", "state": "init"},
				},
				"outputs": []map[string]any{
					{"workType": "task", "state": "in-review"},
					{"workType": "review", "state": "init"},
				},
				"onFailure": []map[string]any{
					{"workType": "task", "state": "failed"},
				},
			},
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
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: MODEL_WORKER\nmodelProvider: CURSOR\nexecutorProvider: SCRIPT_WRAP\nstopToken: <COMPLETE>\n---\nExecutor/review lifecycle verification.\n")
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nProcess reconciled recovery lanes.\n")
	support.WriteWorkstationConfig(t, dir, "review", "---\ntype: MODEL_WORKSTATION\n---\nReview reconciled recovery lanes.\n")
	return dir
}

func submitExecutorReviewProcessResiduePattern(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	laneName string,
	traceID string,
) {
	t.Helper()

	requests := []interfaces.SubmitRequest{
		{
			Name:                   laneName,
			WorkTypeID:             "task",
			TargetState:            "init",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     2,
		},
		{
			WorkID:                 "work-review-residue-1",
			Name:                   laneName,
			WorkTypeID:             "review",
			TargetState:            "init",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     3,
		},
		{
			WorkID:                 "work-review-residue-2",
			Name:                   laneName,
			WorkTypeID:             "review",
			TargetState:            "init",
			TraceID:                traceID,
			CurrentChainingTraceID: traceID,
			ChainingTraceDepth:     3,
		},
	}
	for _, req := range requests {
		h.SubmitFull(context.Background(), []interfaces.SubmitRequest{req})
	}
}

func executorReviewCustomerWorldView(t *testing.T, h *testutil.ServiceTestHarness) (interfaces.FactoryWorldView, error) {
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
	return testutil.BuildFactoryWorldView(events, snapshot.TickCount, snapshot.ActiveThrottlePauses)
}

func assertExecutorReviewProjectionMatchesRuntime(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	runtimeSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	worldView, err := executorReviewCustomerWorldView(t, h)
	if err != nil {
		t.Fatalf("executorReviewCustomerWorldView: %v", err)
	}

	for placeID, tokenIDs := range runtimeSnap.Marking.PlaceTokens {
		count, ok := worldView.Runtime.PlaceTokenCounts[placeID]
		if !ok {
			if len(tokenIDs) == 0 {
				continue
			}
			t.Fatalf("customer world view missing runtime place %q with %d tokens", placeID, len(tokenIDs))
		}
		if len(tokenIDs) != count {
			t.Fatalf("place %q runtime tokens = %d, customer world view = %d", placeID, len(tokenIDs), count)
		}
	}
}

func assertExecutorReviewReplayProjectionIdempotent(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	ctx := context.Background()
	events, err := h.GetFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	first, err := testutil.BuildFactoryWorldView(events, snapshot.TickCount, snapshot.ActiveThrottlePauses)
	if err != nil {
		t.Fatalf("BuildFactoryWorldView first: %v", err)
	}
	second, err := testutil.BuildFactoryWorldView(events, snapshot.TickCount, snapshot.ActiveThrottlePauses)
	if err != nil {
		t.Fatalf("BuildFactoryWorldView second: %v", err)
	}

	for placeID, count := range first.Runtime.PlaceTokenCounts {
		again, ok := second.Runtime.PlaceTokenCounts[placeID]
		if !ok {
			t.Fatalf("second customer world view missing place %q", placeID)
		}
		if count != again {
			t.Fatalf("customer world view place %q token count changed: first=%d second=%d", placeID, count, again)
		}
	}
}

package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func sameTraceIDGuardFactoryDir(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "same_trace_id_guard",
		"workTypes": []map[string]any{
			{
				"name": "plan",
				"states": []map[string]any{
					{"name": "ready", "type": "INITIAL"},
				},
			},
			{
				"name": "task",
				"states": []map[string]any{
					{"name": "ready", "type": "INITIAL"},
					{"name": "matched", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "matcher"},
		},
		"workstations": []map[string]any{
			{
				"name":   "match-items",
				"worker": "matcher",
				"inputs": []map[string]any{
					{"workType": "plan", "state": "ready"},
					{
						"workType": "task",
						"state":    "ready",
						"guards": []map[string]any{
							{"type": "SAME_TRACE_ID", "matchInput": "plan"},
						},
					},
				},
				"outputs": []map[string]any{
					{"workType": "task", "state": "matched"},
				},
			},
		},
	})

	support.WriteAgentConfig(t, dir, "matcher", support.BuildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5.4"))
	return dir
}

func TestSameTraceIDGuard_MatchingCurrentChainingTraceCompletesJoin(t *testing.T) {
	dir := sameTraceIDGuardFactoryDir(t)
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "joined COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:                   "alpha-plan",
		WorkTypeID:             "plan",
		CurrentChainingTraceID: "chain-shared",
		TraceID:                "trace-legacy-plan",
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:                   "beta-task",
		WorkTypeID:             "task",
		CurrentChainingTraceID: "chain-shared",
		TraceID:                "trace-legacy-task",
	}})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "task:matched", 1, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "plan:ready", 0, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:ready", 0, time.Second)

	h.Assert().
		PlaceTokenCount("task:matched", 1).
		HasNoTokenInPlace("plan:ready").
		HasNoTokenInPlace("task:ready")

	if provider.CallCount() != 1 {
		t.Fatalf("expected matcher provider call once, got %d", provider.CallCount())
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameTraceIDGuard_FallsBackToLegacyTraceIDWhenCurrentChainingTraceIsMissing(t *testing.T) {
	dir := sameTraceIDGuardFactoryDir(t)
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "joined COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "alpha-plan",
		WorkTypeID: "plan",
		TraceID:    "trace-shared",
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "beta-task",
		WorkTypeID: "task",
		TraceID:    "trace-shared",
	}})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "task:matched", 1, time.Second)

	if provider.CallCount() != 1 {
		t.Fatalf("expected matcher provider call once, got %d", provider.CallCount())
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameTraceIDGuard_DifferentTraceIdentityStaysBlocked(t *testing.T) {
	dir := sameTraceIDGuardFactoryDir(t)
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "joined COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:                   "alpha-plan",
		WorkTypeID:             "plan",
		CurrentChainingTraceID: "chain-a",
		TraceID:                "trace-shared-name",
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:                   "alpha-plan",
		WorkTypeID:             "task",
		CurrentChainingTraceID: "chain-b",
		TraceID:                "trace-shared-name",
	}})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "plan:ready", 1, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:ready", 1, time.Second)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if provider.CallCount() != 0 {
			t.Fatalf("expected matcher to remain blocked, got %d provider calls", provider.CallCount())
		}
		snapshot, err := h.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if support.PlaceTokenCount(snapshot.Marking, "task:matched") != 0 {
			t.Fatalf("expected no matched output for mismatched trace identities, got marking %#v", snapshot.Marking.PlaceTokens)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameTraceIDGuard_MissingTraceIdentityFailsClosed(t *testing.T) {
	dir := sameTraceIDGuardFactoryDir(t)
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "joined COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "alpha-plan",
		WorkTypeID: "plan",
		TraceID:    "trace-only-plan",
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "beta-task",
		WorkTypeID: "task",
	}})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "plan:ready", 1, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:ready", 1, time.Second)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if provider.CallCount() != 0 {
			t.Fatalf("expected matcher to remain blocked, got %d provider calls", provider.CallCount())
		}
		snapshot, err := h.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if support.PlaceTokenCount(snapshot.Marking, "task:matched") != 0 {
			t.Fatalf("expected no matched output when trace identity is missing, got marking %#v", snapshot.Marking.PlaceTokens)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

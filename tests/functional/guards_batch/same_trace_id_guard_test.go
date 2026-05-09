package guards_batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSameTraceIDGuard_FixtureBoundaryMapsToRuntimeConfig(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "same_trace_id_guard_dir"))
	factoryJSON, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	generated, err := config.GeneratedFactoryFromOpenAPIJSON(factoryJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one guarded workstation", generated.Workstations)
	}

	workstation := (*generated.Workstations)[0]
	if workstation.Name != "match-items" {
		t.Fatalf("generated workstation name = %q, want match-items", workstation.Name)
	}
	if len(workstation.Inputs) != 2 {
		t.Fatalf("generated inputs = %#v, want plan/task inputs", workstation.Inputs)
	}
	if workstation.Inputs[1].Guards == nil || len(*workstation.Inputs[1].Guards) != 1 {
		t.Fatalf("generated guarded task input = %#v, want one same-trace guard", workstation.Inputs[1])
	}

	guard := (*workstation.Inputs[1].Guards)[0]
	if guard.Type != factoryapi.GuardTypeSameTraceID {
		t.Fatalf("generated guard type = %q, want SAME_TRACE_ID", guard.Type)
	}
	if guard.MatchInput == nil || *guard.MatchInput != "plan" {
		t.Fatalf("generated guard matchInput = %#v, want plan", guard.MatchInput)
	}

	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	matcher, ok := loaded.Worker("matcher")
	if !ok {
		t.Fatal("expected matcher worker definition")
	}
	if matcher.Type != interfaces.WorkerTypeModel || matcher.StopToken != "COMPLETE" {
		t.Fatalf("matcher worker runtime config = %#v", matcher)
	}

	runtimeWorkstation, ok := loaded.Workstation("match-items")
	if !ok {
		t.Fatal("expected match-items workstation definition")
	}
	if runtimeWorkstation.Type != interfaces.WorkstationTypeModel || runtimeWorkstation.WorkerTypeName != "matcher" {
		t.Fatalf("match-items runtime config = %#v", runtimeWorkstation)
	}
	if len(runtimeWorkstation.Inputs) != 2 || runtimeWorkstation.Inputs[1].Guard == nil {
		t.Fatalf("runtime workstation inputs = %#v, want guarded task input", runtimeWorkstation.Inputs)
	}

	runtimeGuard := runtimeWorkstation.Inputs[1].Guard
	if runtimeGuard.Type != interfaces.GuardTypeSameTraceID || runtimeGuard.MatchInput != "plan" {
		t.Fatalf("runtime same-trace guard = %#v, want type same_trace_id and match_input plan", runtimeGuard)
	}
}

func TestSameTraceIDGuard_MatchingCurrentChainingTraceCompletesJoin(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "same_trace_id_guard_dir"))
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "same_trace_id_guard_dir"))
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "same_trace_id_guard_dir"))
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "same_trace_id_guard_dir"))
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

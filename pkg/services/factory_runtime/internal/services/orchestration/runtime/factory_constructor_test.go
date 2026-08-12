package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNew_RequiresNet(t *testing.T) {
	_, err := newTestFactory()
	if err == nil {
		t.Fatal("expected error when Net is not provided")
	}
}

func TestNew_RequiresClock(t *testing.T) {
	_, err := newTestFactory(withNet(buildSimpleNet()), withClock(nil))
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime clock is required") {
		t.Fatalf("New() error = %v, want required clock error", err)
	}
}

type resourceCapacityRuntimeHarness struct {
	capacity  factoryruntime.ResourceCapacityService
	admitted  factoryruntime.AdmittedResourceCapacityService
	admission factoryruntime.ResourceCapacityAdmission
	leases    factoryruntime.ResourceCapacityLeaseAdmission
	revision  factoryruntime.ResourceCapacityRevisionService
}

func newResourceCapacityRuntimeHarness(t *testing.T) resourceCapacityRuntimeHarness {
	t.Helper()
	net := buildSimpleNet()
	net.Resources = map[string]*state.ResourceDef{
		"gpu-slot": {ID: "gpu-slot", Name: "GPU slot", Capacity: 2},
		"orphan":   {ID: "orphan", Name: "Orphan pool", Capacity: 1},
	}
	for _, resource := range net.Resources {
		place, _ := state.GenerateResourcePlaces(resource, time.Unix(0, 0))
		net.Places[place.ID] = place
	}

	f, err := newTestFactory(
		withNet(net),
		withRuntimeConfig(runtimefixtures.RuntimeDefinitionLookupFixture{
			Factory: &interfaces.FactoryConfig{
				Name: "capacity-factory",
				Resources: []interfaces.ResourceConfig{{
					ID:       "gpu-slot",
					Capacity: 2,
				}},
			},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	harness := resourceCapacityRuntimeHarness{}
	var ok bool
	harness.capacity, ok = f.(factoryruntime.ResourceCapacityService)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity service")
	}
	harness.admitted, ok = f.(factoryruntime.AdmittedResourceCapacityService)
	if !ok {
		t.Fatal("Factory Runtime does not expose admitted resource capacity service")
	}
	harness.admission, ok = f.(factoryruntime.ResourceCapacityAdmission)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity admission")
	}
	harness.leases, ok = f.(factoryruntime.ResourceCapacityLeaseAdmission)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity leases")
	}
	harness.revision, ok = f.(factoryruntime.ResourceCapacityRevisionService)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity revision")
	}
	return harness
}

func TestResourceCapacityRuntimePreviewAttachesEffectiveFactory(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	harness.revision.SetFactoryRevision(4)
	harness.revision.SetFactoryRevision(2)
	if got := harness.revision.CurrentFactoryRevision(); got != 4 {
		t.Fatalf("Factory Runtime revision = %d, want monotonic revision 4", got)
	}

	ctx := context.Background()
	preview, err := harness.capacity.PreviewResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 2,
	})
	if err != nil {
		t.Fatalf("PreviewResourceCapacity: %v", err)
	}
	assertResourceCapacityFactorySnapshot(t, preview, 2, 1)
	previewConfig := decodeResourceCapacityFactorySnapshot(t, preview)
	if previewConfig.Resources[0].Name != "GPU slot" {
		t.Fatalf("preview resource name = %q, want runtime resource name", previewConfig.Resources[0].Name)
	}
}

func TestResourceCapacityRuntimeAdmittedMutationAndNoOp(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	ctx := context.Background()
	releaseAdmission, err := harness.admission.AcquireResourceCapacityAdmission(ctx)
	if err != nil {
		t.Fatalf("AcquireResourceCapacityAdmission: %v", err)
	}
	admittedPreview, err := harness.admitted.PreviewResourceCapacityAdmitted(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 3,
	})
	if err != nil {
		releaseAdmission()
		t.Fatalf("PreviewResourceCapacityAdmitted: %v", err)
	}
	if admittedPreview.Outcome != factoryruntime.ResourceCapacityOutcomeApplied {
		releaseAdmission()
		t.Fatalf("admitted preview outcome = %q, want applied", admittedPreview.Outcome)
	}
	updated, err := harness.admitted.SetResourceCapacityAdmitted(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 3,
	})
	releaseAdmission()
	if err != nil {
		t.Fatalf("SetResourceCapacityAdmitted: %v", err)
	}
	assertResourceCapacityFactorySnapshot(t, updated, 3, 1)

	noOp, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 3,
	})
	if err != nil || noOp.Outcome != factoryruntime.ResourceCapacityOutcomeNoOp {
		t.Fatalf("SetResourceCapacity no-op = (%#v, %v), want NO_OP", noOp, err)
	}
}

func TestResourceCapacityRuntimeOrphanMutationAndLease(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	harness.revision.SetFactoryRevision(4)
	ctx := context.Background()
	orphan, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "orphan", RequestedCapacity: 2,
	})
	if err != nil {
		t.Fatalf("SetResourceCapacity append: %v", err)
	}
	assertResourceCapacityFactorySnapshot(t, orphan, 2, 2)

	lease, err := harness.leases.AcquireResourceCapacityLease(ctx, factoryruntime.ResourceCapacityLeaseRequest{ResourceID: "orphan"})
	if err != nil {
		t.Fatalf("AcquireResourceCapacityLease: %v", err)
	}
	if lease.FactoryRevision != 4 {
		t.Fatalf("resource lease revision = %d, want 4", lease.FactoryRevision)
	}
	lease.Release()
	lease.Release()
}

func TestResourceCapacityRuntimeSnapshotsRetainEarlierCapacityChanges(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	ctx := context.Background()
	if _, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 4,
	}); err != nil {
		t.Fatalf("SetResourceCapacity gpu-slot: %v", err)
	}
	second, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "orphan", RequestedCapacity: 2,
	})
	if err != nil {
		t.Fatalf("SetResourceCapacity orphan: %v", err)
	}
	config := decodeResourceCapacityFactorySnapshot(t, second)
	capacities := make(map[string]int, len(config.Resources))
	for _, resource := range config.Resources {
		capacities[resource.ID] = resource.Capacity
	}
	if capacities["gpu-slot"] != 4 || capacities["orphan"] != 2 {
		t.Fatalf("effective capacities = %#v, want gpu-slot=4 and orphan=2", capacities)
	}
}

func assertResourceCapacityFactorySnapshot(t *testing.T, result factoryruntime.ResourceCapacityResult, capacity, resourceCount int) {
	t.Helper()
	if result.Outcome == "" {
		t.Fatalf("resource capacity result has no outcome: %#v", result)
	}
	if result.Factory == nil {
		t.Fatal("resource capacity result has no effective Factory snapshot")
	}
	config := decodeResourceCapacityFactorySnapshot(t, result)
	if len(config.Resources) != resourceCount {
		t.Fatalf("effective Factory resources = %d, want %d", len(config.Resources), resourceCount)
	}
	for _, resource := range config.Resources {
		if resource.ID == result.ResourceID && resource.Capacity != capacity {
			t.Fatalf("effective %s capacity = %d, want %d", result.ResourceID, resource.Capacity, capacity)
		}
	}
}

func decodeResourceCapacityFactorySnapshot(t *testing.T, result factoryruntime.ResourceCapacityResult) interfaces.FactoryConfig {
	t.Helper()
	var config interfaces.FactoryConfig
	if err := result.Factory.Decode(&config); err != nil {
		t.Fatalf("decode effective Factory snapshot: %v", err)
	}
	return config
}

func TestNew_ConfiguresProvidedRuntimeAwareScheduler(t *testing.T) {
	net := buildSimpleNet()
	customScheduler := &runtimeAwareScheduler{}
	runtimeCfg := runtimeSchedulerConfig(&runtimefixtures.RuntimeDefinitionLookupFixture{})

	_, err := newTestFactory(
		withNet(net),
		withScheduler(customScheduler),
		withRuntimeConfig(runtimeCfg),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if customScheduler.configured != runtimeCfg {
		t.Fatal("expected New to inject runtime config into provided scheduler")
	}

	var _ scheduler.Scheduler = customScheduler
}

func TestNew_InlineDispatchWithNoopExecutorCompletesWorkflow(t *testing.T) {
	n := buildSimpleNet()
	f, err := newTestFactory(
		withNet(n),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = submitWorkRequests(ctx, f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-1"}})
	}()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
}

func TestNew_InlineDispatchExecutorPanicRoutesFailedWork(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &panicExecutor{message: "simulated catastrophic panic"}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     "work-panic",
		WorkTypeID: "task",
		TraceID:    "trace-panic",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-panic", "task:failed") {
		t.Fatalf("expected work-panic to reach task:failed, marking=%#v", snapshot.Marking.PlaceTokens)
	}
	if markingContainsWorkAtPlace(&snapshot.Marking, "work-panic", "task:done") {
		t.Fatal("expected work-panic to avoid task:done after executor panic")
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("dispatch history count = %d, want 1", len(snapshot.DispatchHistory))
	}
	completed := snapshot.DispatchHistory[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want %q", completed.Outcome, workerexecution.OutcomeFailed)
	}
	if !strings.Contains(completed.Reason, "executor panic:") || !strings.Contains(completed.Reason, "simulated catastrophic panic") {
		t.Fatalf("dispatch reason = %q, want panic-derived failure message", completed.Reason)
	}
}

func TestNew_InlineDispatchWithoutRegisteredExecutorRecordsMissingExecutorFailure(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable := tickableFactory(t, f)

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     "work-missing-executor",
		WorkTypeID: "task",
		TraceID:    "trace-missing-executor",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("dispatch history count = %d, want 1", len(snap.DispatchHistory))
	}
	completed := snap.DispatchHistory[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want %q", completed.Outcome, workerexecution.OutcomeFailed)
	}
	if !strings.Contains(completed.Reason, `no executor registered for worker type "mock"`) {
		t.Fatalf("dispatch reason = %q, want missing executor error", completed.Reason)
	}
}

func TestNew_CompletesWorkflowThroughActiveSubsystems(t *testing.T) {
	f, ledger := newPassingInlineRuntimeWithLedger(t)
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     "work-active-path",
		WorkTypeID: "task",
		TraceID:    "trace-active-path",
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-active-path", "task:done") {
		t.Fatalf("expected work-active-path to reach task:done, marking=%#v", snapshot.Marking.PlaceTokens)
	}

	if ledger.CallCount("RecordWorkstationRequest") != 1 {
		t.Fatalf("RecordWorkstationRequest calls = %d, want 1", ledger.CallCount("RecordWorkstationRequest"))
	}
	if ledger.CallCount("RecordWorkstationResponse") != 1 {
		t.Fatalf("RecordWorkstationResponse calls = %d, want 1", ledger.CallCount("RecordWorkstationResponse"))
	}
}

func TestNew_InitialStructureIncludesRuntimeConfigWorkerMetadata(t *testing.T) {
	_, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withRuntimeConfig(runtimeProjectionConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"mock": {
					Type:             interfaces.WorkerTypeModel,
					ExecutorProvider: "codex-cli",
					ModelProvider:    "openai",
					Model:            "gpt-5.4",
				},
			},
		}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if ledger.CallCount("RecordInitialStructure") != 1 {
		t.Fatalf("RecordInitialStructure calls = %d, want 1", ledger.CallCount("RecordInitialStructure"))
	}
	// Recordings owns the serialized worker metadata assertion in
	// TestFactoryEventHistory_RecordInitialStructure_UsesRuntimeConfigProjection.
}

func TestNew_WithMockExecutor(t *testing.T) {
	if _, err := newTestFactory(withNet(buildSimpleNet()), withWorkerExecutor("mock", &passExecutor{})); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestSubmit_AssignsTraceIDWhenMissing(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable := tickableFactory(t, f)
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.Color.TraceID == "" {
			t.Fatal("expected submitted token to have an assigned trace ID")
		}
	}
}

func TestNew_WithClockStampsDispatchesDeterministically(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
		withClock(clock),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable := tickableFactory(t, f)
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-clock"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("expected 1 completed dispatch, got %d", len(snap.DispatchHistory))
	}
	want := base.Add(time.Second)
	completed := snap.DispatchHistory[0]
	if !completed.StartTime.Equal(want) {
		t.Fatalf("dispatch start = %s, want %s", completed.StartTime, want)
	}
	if !completed.EndTime.Equal(want) {
		t.Fatalf("dispatch end = %s, want %s", completed.EndTime, want)
	}
}

package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func newRootContractTestFactory(t *testing.T) *factoryImpl {
	t.Helper()
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("newTestFactory: %v", err)
	}
	impl, ok := f.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", f)
	}
	return impl
}

func requireRootErrIs(t *testing.T, err error, want error, label string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s error = %v, want %v", label, err, want)
	}
}

func requireNoRootErr(t *testing.T, err error, label string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s error = %v, want nil", label, err)
	}
}

func TestFactoryImpl_Terminate_MapsLifecycleStates(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStateRunning
	got, err := impl.Terminate(ctx, factory.TerminateRequest{Reason: "stop"})
	requireNoRootErr(t, err, "Terminate(running)")
	if got.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Terminate(running) outcome = %q, want ACCEPTED", got.Outcome)
	}

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.Terminate(ctx, factory.TerminateRequest{Reason: "stop"})
	requireRootErrIs(t, err, factory.ErrAlreadyStopped, "Terminate(completed)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.Terminate(ctx, factory.TerminateRequest{Reason: "stop"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "Terminate(unknown)")
}

func TestFactoryImpl_Observe_ProjectsSanitizedObservation(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStateIdle
	got, err := impl.Observe(ctx, factory.ObserveRequest{Scope: factory.ObservationScopeStatus})
	requireNoRootErr(t, err, "Observe(idle)")
	if got.Observation.Status == "" {
		t.Fatal("Observe(idle) status is empty, want projected status")
	}
	if got.Observation.Progress != (factory.ObservationProgress{}) {
		t.Fatalf("Observe(STATUS) progress = %#v, want empty scoped view", got.Observation.Progress)
	}
	if len(got.Observation.InFlightDispatches) != 0 {
		t.Fatalf("Observe(STATUS) dispatches = %#v, want empty scoped view", got.Observation.InFlightDispatches)
	}

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.Observe(ctx, factory.ObserveRequest{})
	requireRootErrIs(t, err, factory.ErrNotRunning, "Observe(unknown)")
}

func TestFactoryImpl_PlanAndAcceptDispatch_MapsLifecycleAvailability(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStateRunning
	planned, err := impl.PlanDispatch(ctx, factory.PlanDispatchRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	})
	requireNoRootErr(t, err, "PlanDispatch(running)")
	if planned != (factory.PlanDispatchResult{
		Outcome:       factory.DispatchPlanOutcomeAccepted,
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	}) {
		t.Fatalf("PlanDispatch result = %#v, want accepted root shape", planned)
	}

	accepted, err := impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	})
	requireNoRootErr(t, err, "AcceptDispatchResult(running)")
	if accepted.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult outcome = %q, want RETIRED", accepted.Outcome)
	}

	impl.state = interfaces.FactoryStateFailed
	_, err = impl.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "dispatch-2", CorrelationID: "corr-2"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "PlanDispatch(failed)")
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "dispatch-2", CorrelationID: "corr-2"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "AcceptDispatchResult(failed)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "dispatch-3", CorrelationID: "corr-3"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "PlanDispatch(unknown)")
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "dispatch-3", CorrelationID: "corr-3"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "AcceptDispatchResult(unknown)")
}

func TestFactoryImpl_CaptureCheckpoint_ReturnsOpaquePayload(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()
	impl.state = interfaces.FactoryStatePaused

	captured, err := impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{})
	requireNoRootErr(t, err, "CaptureCheckpoint(paused)")
	if captured.Outcome != factory.CheckpointOutcomeCaptured {
		t.Fatalf("CaptureCheckpoint outcome = %q, want CAPTURED", captured.Outcome)
	}
	if captured.Checkpoint.CheckpointID == "" || len(captured.Checkpoint.Payload) == 0 {
		t.Fatalf("CaptureCheckpoint checkpoint = %#v, want opaque payload and stub id", captured.Checkpoint)
	}

	named, err := impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-1"})
	requireNoRootErr(t, err, "CaptureCheckpoint(named)")
	if named.Checkpoint.CheckpointID != "cp-1" {
		t.Fatalf("CaptureCheckpoint named id = %q, want cp-1", named.Checkpoint.CheckpointID)
	}
}

func TestFactoryImpl_LoadCheckpoint_MapsMissingAndLifecycle(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()
	impl.state = interfaces.FactoryStatePaused

	_, err := impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{})
	requireRootErrIs(t, err, factory.ErrCheckpointNotFound, "LoadCheckpoint(empty)")
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "missing"})
	requireRootErrIs(t, err, factory.ErrCheckpointNotFound, "LoadCheckpoint(missing)")

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-2"})
	requireRootErrIs(t, err, factory.ErrCheckpointNotFound, "LoadCheckpoint(completed)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-3"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "LoadCheckpoint(unknown)")
}

func TestFactoryImpl_RestoreCheckpoint_MapsLifecycle(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()
	impl.state = interfaces.FactoryStatePaused

	restored, err := impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-1", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	requireNoRootErr(t, err, "RestoreCheckpoint(paused)")
	if restored.Outcome != factory.CheckpointOutcomeRestored || restored.CheckpointID != "cp-1" {
		t.Fatalf("RestoreCheckpoint result = %#v, want restored cp-1", restored)
	}

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-2"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "CaptureCheckpoint(completed)")
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-2", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	requireRootErrIs(t, err, factory.ErrNotRunning, "RestoreCheckpoint(completed)")
}

func sampleRootObservationSnapshot() *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus:          interfaces.RuntimeStatusActive,
		InFlightCount:          1,
		TickCount:              7,
		FactoryState:           string(interfaces.FactoryStateRunning),
		LifecycleControlStatus: "RUNNING",
		StreamGenerationID:     "gen-1",
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d1": {
				DispatchID:      "d1",
				WorkstationName: "desk",
				ConsumedTokens: []workerexecution.Token{
					{Color: workerexecution.Color{WorkID: "work-1"}},
					{Color: workerexecution.Color{WorkID: ""}},
				},
			},
			"nil": nil,
		},
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				DispatchID: "done-1",
				Outcome:    workerexecution.WorkOutcome("SUCCESS"),
				ConsumedTokens: []workerexecution.Token{
					{Color: workerexecution.Color{WorkID: "work-done"}},
				},
			},
		},
	}
}

func TestProjectRootObservation_NilSnapshotIsEmpty(t *testing.T) {
	empty := projectRootObservation(nil, factory.ObservationScopeFull)
	if empty.Status != "" || empty.Progress != (factory.ObservationProgress{}) || len(empty.InFlightDispatches) != 0 || len(empty.Results) != 0 {
		t.Fatalf("projectRootObservation(nil) = %#v, want empty", empty)
	}
}

func TestProjectRootObservation_FullProjectionOmitsNilDispatch(t *testing.T) {
	full := projectRootObservation(sampleRootObservationSnapshot(), factory.ObservationScopeFull)
	if full.Status != factory.ObservationStatusActive {
		t.Fatalf("status = %q, want ACTIVE", full.Status)
	}
	if full.Progress.InFlightDispatchCount != 1 || full.Progress.TickCount != 7 {
		t.Fatalf("progress = %#v, want in-flight=1 tick=7", full.Progress)
	}
	if len(full.InFlightDispatches) != 1 {
		t.Fatalf("in-flight count = %d, want 1 (nil entry skipped)", len(full.InFlightDispatches))
	}
	if len(full.InFlightDispatches[0].WorkIDs) != 1 || full.InFlightDispatches[0].WorkIDs[0] != "work-1" {
		t.Fatalf("in-flight work ids = %#v, want [work-1]", full.InFlightDispatches[0].WorkIDs)
	}
	if len(full.Results) != 1 || full.Results[0].WorkID != "work-done" {
		t.Fatalf("results = %#v, want work-done", full.Results)
	}
	if full.Health.StreamGenerationID != "gen-1" {
		t.Fatalf("health = %#v, want gen-1", full.Health)
	}
}

func TestProjectRootObservation_ScopeFilters(t *testing.T) {
	snap := sampleRootObservationSnapshot()

	statusOnly := projectRootObservation(snap, factory.ObservationScopeStatus)
	if statusOnly.Status != factory.ObservationStatusActive || statusOnly.Progress != (factory.ObservationProgress{}) {
		t.Fatalf("STATUS scope = %#v, want status-only", statusOnly)
	}

	progressOnly := projectRootObservation(snap, factory.ObservationScopeProgress)
	if progressOnly.Progress.TickCount != 7 || progressOnly.Status != "" {
		t.Fatalf("PROGRESS scope = %#v, want progress-only", progressOnly)
	}

	dispatchOnly := projectRootObservation(snap, factory.ObservationScopeDispatches)
	if len(dispatchOnly.InFlightDispatches) != 1 || dispatchOnly.Status != "" {
		t.Fatalf("DISPATCHES scope = %#v, want dispatches-only", dispatchOnly)
	}

	resultsOnly := projectRootObservation(snap, factory.ObservationScopeResults)
	if len(resultsOnly.Results) != 1 || resultsOnly.Status != "" {
		t.Fatalf("RESULTS scope = %#v, want results-only", resultsOnly)
	}

	resourcesOnly := projectRootObservation(snap, factory.ObservationScopeResources)
	if resourcesOnly.Status != "" || len(resourcesOnly.Resources) != 0 {
		t.Fatalf("RESOURCES scope = %#v, want resources-only empty", resourcesOnly)
	}

	healthOnly := projectRootObservation(snap, factory.ObservationScopeHealth)
	if healthOnly.Health.FactoryState == "" || healthOnly.Status != "" {
		t.Fatalf("HEALTH scope = %#v, want health-only", healthOnly)
	}

	unknownScope := projectRootObservation(snap, factory.ObservationScope("OTHER"))
	if unknownScope.Status != factory.ObservationStatusActive {
		t.Fatalf("unknown scope = %#v, want full observation fallback", unknownScope)
	}
}

func TestProjectRootObservation_MapsRuntimeStatus(t *testing.T) {
	idleSnap := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatusIdle}
	if got := projectRootObservation(idleSnap, ""); got.Status != factory.ObservationStatusIdle {
		t.Fatalf("idle status = %q, want IDLE", got.Status)
	}

	finishedSnap := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatusFinished}
	if got := projectRootObservation(finishedSnap, factory.ObservationScopeFull); got.Status != factory.ObservationStatusFinished {
		t.Fatalf("finished status = %q, want FINISHED", got.Status)
	}

	unknownStatus := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatus("weird")}
	if got := projectRootObservation(unknownStatus, factory.ObservationScopeFull); got.Status != factory.ObservationStatusIdle {
		t.Fatalf("unknown runtime status = %q, want IDLE default", got.Status)
	}
}

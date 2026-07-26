package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
	if impl.state != interfaces.FactoryStateCompleted {
		t.Fatalf("Terminate(running) state = %q, want COMPLETED", impl.state)
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
	duplicate, err := impl.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "dispatch-1", CorrelationID: "corr-1"})
	requireNoRootErr(t, err, "PlanDispatch(duplicate)")
	if duplicate.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("PlanDispatch duplicate outcome = %q, want DUPLICATE_IDEMPOTENT", duplicate.Outcome)
	}
	_, err = impl.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "dispatch-other", CorrelationID: "corr-1"})
	requireRootErrIs(t, err, factory.ErrDuplicateDispatchIntent, "PlanDispatch(conflict)")

	accepted, err := impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	})
	requireNoRootErr(t, err, "AcceptDispatchResult(running)")
	if accepted.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult outcome = %q, want RETIRED", accepted.Outcome)
	}
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "dispatch-unknown", CorrelationID: "unknown"})
	requireRootErrIs(t, err, factory.ErrUnknownDispatchCorrelation, "AcceptDispatchResult(unknown)")
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "dispatch-1", CorrelationID: "corr-1", ResultOutcome: "INVALID"})
	requireRootErrIs(t, err, factory.ErrInvalidDispatchResultBoundary, "AcceptDispatchResult(invalid)")

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
	captured, err := impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-2"})
	requireNoRootErr(t, err, "CaptureCheckpoint(cp-2)")
	loaded, err := impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-2", ExpectedSchemaVersion: 1})
	requireNoRootErr(t, err, "LoadCheckpoint(cp-2)")
	if loaded.Outcome != factory.CheckpointOutcomeLoaded || loaded.Checkpoint.CheckpointID != captured.Checkpoint.CheckpointID || !loaded.Compatible {
		t.Fatalf("LoadCheckpoint result = %#v, want compatible captured checkpoint", loaded)
	}
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-2", ExpectedSchemaVersion: 2})
	requireRootErrIs(t, err, factory.ErrIncompatibleCheckpoint, "LoadCheckpoint(incompatible)")

	impl.state = interfaces.FactoryStateCompleted
	loaded, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-2"})
	requireNoRootErr(t, err, "LoadCheckpoint(completed)")
	if loaded.Checkpoint.CheckpointID != "cp-2" {
		t.Fatalf("LoadCheckpoint(completed) id = %q, want cp-2", loaded.Checkpoint.CheckpointID)
	}

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
	loaded, err := impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-1"})
	requireNoRootErr(t, err, "LoadCheckpoint(restored)")
	if loaded.Checkpoint.CheckpointID != "cp-1" {
		t.Fatalf("LoadCheckpoint(restored) id = %q, want cp-1", loaded.Checkpoint.CheckpointID)
	}
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{Checkpoint: factory.Checkpoint{CheckpointID: "bad", SchemaVersion: 1}})
	requireRootErrIs(t, err, factory.ErrCorruptCheckpoint, "RestoreCheckpoint(corrupt)")

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-2"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "CaptureCheckpoint(completed)")
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-2", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	requireRootErrIs(t, err, factory.ErrNotRunning, "RestoreCheckpoint(completed)")
}

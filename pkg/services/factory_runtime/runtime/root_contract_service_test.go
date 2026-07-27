package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type testRuntimeClock struct{}

func (testRuntimeClock) Now() time.Time { return time.Now() }

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

	if done := impl.ControlWaitToComplete(factory.WaitToCompleteRequest{}).Done; done == nil {
		t.Fatal("ControlWaitToComplete Done channel is nil")
	}

	impl.state = interfaces.FactoryStateRunning
	got, err := impl.ControlTerminate(ctx, factory.TerminateRequest{Reason: "stop"})
	requireNoRootErr(t, err, "Terminate(running)")
	if got.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Terminate(running) outcome = %q, want ACCEPTED", got.Outcome)
	}
	if impl.state != interfaces.FactoryStateCompleted {
		t.Fatalf("Terminate(running) state = %q, want COMPLETED", impl.state)
	}

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.ControlTerminate(ctx, factory.TerminateRequest{Reason: "stop"})
	requireRootErrIs(t, err, factory.ErrAlreadyStopped, "Terminate(completed)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.ControlTerminate(ctx, factory.TerminateRequest{Reason: "stop"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "Terminate(unknown)")
}

func TestFactoryImpl_PauseResume_MapLifecycleStatesAndNoOps(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStateRunning
	paused, err := impl.ControlPause(ctx, factory.PauseRequest{})
	requireNoRootErr(t, err, "Pause(running)")
	if paused.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Pause(running) outcome = %q, want ACCEPTED", paused.Outcome)
	}
	paused, err = impl.ControlPause(ctx, factory.PauseRequest{})
	requireNoRootErr(t, err, "Pause(paused)")
	if paused.Outcome != factory.ControlOutcomeNoOp {
		t.Fatalf("Pause(paused) outcome = %q, want NO_OP", paused.Outcome)
	}

	resumed, err := impl.ControlResume(ctx, factory.ResumeRequest{})
	requireNoRootErr(t, err, "Resume(paused)")
	if resumed.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Resume(paused) outcome = %q, want ACCEPTED", resumed.Outcome)
	}
	resumed, err = impl.ControlResume(ctx, factory.ResumeRequest{})
	requireNoRootErr(t, err, "Resume(running)")
	if resumed.Outcome != factory.ControlOutcomeNoOp {
		t.Fatalf("Resume(running) outcome = %q, want NO_OP", resumed.Outcome)
	}

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.ControlPause(ctx, factory.PauseRequest{})
	requireRootErrIs(t, err, factory.ErrNotRunning, "Pause(completed)")
	impl.state = interfaces.FactoryStateFailed
	_, err = impl.ControlResume(ctx, factory.ResumeRequest{})
	requireRootErrIs(t, err, factory.ErrNotRunning, "Resume(failed)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.ControlPause(ctx, factory.PauseRequest{})
	requireRootErrIs(t, err, factory.ErrInvalidLifecycleTransition, "Pause(unknown)")
	_, err = impl.ControlResume(ctx, factory.ResumeRequest{})
	requireRootErrIs(t, err, factory.ErrInvalidLifecycleTransition, "Resume(unknown)")
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

	_, err = impl.Observe(ctx, factory.ObserveRequest{Scope: factory.ObservationScope("INVALID")})
	requireRootErrIs(t, err, factory.ErrInvalidObservationScope, "Observe(invalid scope)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.Observe(ctx, factory.ObserveRequest{})
	requireRootErrIs(t, err, factory.ErrNotRunning, "Observe(unknown)")
}

func TestFactoryImpl_DispatchContracts_UseCanonicalPlanningState(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()
	plan := factory.PlanDispatchRequest{
		DispatchID:      "dispatch-1",
		CorrelationID:   "corr-1",
		WorkIDs:         []string{"work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/trace-1/work-1",
	}

	impl.state = interfaces.FactoryStateRunning
	planned, err := impl.PlanDispatch(ctx, plan)
	requireNoRootErr(t, err, "PlanDispatch(running)")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted ||
		planned.DispatchID != plan.DispatchID ||
		planned.CorrelationID != plan.CorrelationID {
		t.Fatalf("PlanDispatch(running) = %#v, want accepted canonical identities", planned)
	}
	duplicate, err := impl.PlanDispatch(ctx, plan)
	requireNoRootErr(t, err, "PlanDispatch(duplicate)")
	if duplicate.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("PlanDispatch(duplicate) outcome = %q, want DUPLICATE_IDEMPOTENT", duplicate.Outcome)
	}
	intent, ok := impl.dispatchPlan.Intent(plan.DispatchID)
	if !ok || intent.Attempts != 1 {
		t.Fatalf("dispatch intent = %#v, %t; want one Workers publication attempt", intent, ok)
	}

	_, err = impl.PlanDispatch(ctx, factory.PlanDispatchRequest{})
	requireRootErrIs(t, err, factory.ErrInvalidDispatchResultBoundary, "PlanDispatch(invalid)")

	retired, err := impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
		WorkID:        "work-1",
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult(running)")
	if retired.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult(running) outcome = %q, want RETIRED", retired.Outcome)
	}
	duplicateResult, err := impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
		WorkID:        "work-1",
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult(duplicate)")
	if duplicateResult.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("AcceptDispatchResult(duplicate) outcome = %q, want DUPLICATE_IDEMPOTENT", duplicateResult.Outcome)
	}
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{})
	requireRootErrIs(t, err, factory.ErrUnknownDispatchCorrelation, "AcceptDispatchResult(unknown)")
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID: "dispatch-1", CorrelationID: "corr-1", WorkID: "work-1", ResultOutcome: "INVALID",
	})
	requireRootErrIs(t, err, factory.ErrInvalidDispatchResultBoundary, "AcceptDispatchResult(invalid)")

	impl.state = interfaces.FactoryStateFailed
	_, err = impl.PlanDispatch(ctx, plan)
	requireRootErrIs(t, err, factory.ErrNotRunning, "PlanDispatch(failed)")
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID: "dispatch-2", CorrelationID: "corr-2", WorkID: "work-2",
		ResultOutcome: factory.DispatchResultOutcomeFailure,
	})
	requireRootErrIs(t, err, factory.ErrNotRunning, "AcceptDispatchResult(failed)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.PlanDispatch(ctx, plan)
	requireRootErrIs(t, err, factory.ErrNotRunning, "PlanDispatch(unknown)")
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID: "dispatch-3", CorrelationID: "corr-3", WorkID: "work-3",
		ResultOutcome: factory.DispatchResultOutcomeCancelled,
	})
	requireRootErrIs(t, err, factory.ErrNotRunning, "AcceptDispatchResult(unknown)")
}

func TestFactoryImpl_PausedDispatchPlanPublishesOnlyAfterResume(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()
	impl.state = interfaces.FactoryStateRunning

	if _, err := impl.ControlPause(ctx, factory.PauseRequest{}); err != nil {
		t.Fatalf("ControlPause: %v", err)
	}
	plan := factory.PlanDispatchRequest{
		DispatchID:      "dispatch-paused",
		CorrelationID:   "corr-paused",
		WorkIDs:         []string{"work-paused"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/trace-paused/work-paused",
	}
	accepted, err := impl.PlanDispatch(ctx, plan)
	requireNoRootErr(t, err, "PlanDispatch(paused)")
	if accepted.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch(paused) outcome = %q, want ACCEPTED", accepted.Outcome)
	}
	intent, ok := impl.dispatchPlan.Intent(plan.DispatchID)
	if !ok || intent.Attempts != 0 {
		t.Fatalf("paused intent = %#v, %t; want accepted without publication", intent, ok)
	}

	if _, err := impl.ControlResume(ctx, factory.ResumeRequest{}); err != nil {
		t.Fatalf("ControlResume: %v", err)
	}
	intent, ok = impl.dispatchPlan.Intent(plan.DispatchID)
	if !ok || intent.Attempts != 1 {
		t.Fatalf("resumed intent = %#v, %t; want one Workers publication", intent, ok)
	}
}

func TestFactoryImpl_CheckpointContracts_DoNotReportFalseSuccess(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()
	impl.state = interfaces.FactoryStatePaused

	_, err := impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-1"})
	requireRootErrIs(t, err, factory.ErrCapabilityUnavailable, "CaptureCheckpoint(paused)")
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{})
	requireRootErrIs(t, err, factory.ErrCheckpointNotFound, "LoadCheckpoint(empty)")
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-1"})
	requireRootErrIs(t, err, factory.ErrCapabilityUnavailable, "LoadCheckpoint(cp-1)")

	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-1", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	requireRootErrIs(t, err, factory.ErrCapabilityUnavailable, "RestoreCheckpoint(paused)")
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{Checkpoint: factory.Checkpoint{CheckpointID: "bad", SchemaVersion: 1}})
	requireRootErrIs(t, err, factory.ErrCorruptCheckpoint, "RestoreCheckpoint(corrupt)")
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-2", SchemaVersion: 2, Payload: []byte(`{}`)},
	})
	requireRootErrIs(t, err, factory.ErrIncompatibleCheckpoint, "RestoreCheckpoint(incompatible)")

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-2"})
	requireRootErrIs(t, err, factory.ErrNotRunning, "CaptureCheckpoint(completed)")
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-2"})
	requireRootErrIs(t, err, factory.ErrCapabilityUnavailable, "LoadCheckpoint(completed)")
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-2", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	requireRootErrIs(t, err, factory.ErrNotRunning, "RestoreCheckpoint(completed)")
}

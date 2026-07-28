package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type testRuntimeClock struct{}

func (testRuntimeClock) Now() time.Time { return time.Now() }

type controlledWorkstationBoundary struct {
	requests chan workers.WorkstationDispatchRequest
	results  chan workers.WorkstationDispatchResult
	cancels  chan workers.WorkstationDispatchCancelRequest
	stops    atomic.Int32
}

type countingWorkerExecutor struct {
	calls atomic.Int32
}

func (e *countingWorkerExecutor) Execute(
	context.Context,
	work.WorkDispatch,
) (workers.WorkResult, error) {
	e.calls.Add(1)
	return workers.WorkResult{}, errors.New("Runtime invoked executor outside Workers boundary")
}

func newControlledWorkstationBoundary() *controlledWorkstationBoundary {
	return &controlledWorkstationBoundary{
		requests: make(chan workers.WorkstationDispatchRequest, 1),
		results:  make(chan workers.WorkstationDispatchResult, 1),
		cancels:  make(chan workers.WorkstationDispatchCancelRequest, 1),
	}
}

func (*controlledWorkstationBoundary) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{
		Outcome: workers.WorkstationPoolLifecycleOutcomeStarted,
	}, nil
}

func (b *controlledWorkstationBoundary) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	b.stops.Add(1)
	return workers.WorkstationPoolStopResult{
		Outcome: workers.WorkstationPoolLifecycleOutcomeStopped,
	}, nil
}

func (b *controlledWorkstationBoundary) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	b.requests <- request
	select {
	case result := <-b.results:
		return result, nil
	case <-ctx.Done():
		return workers.WorkstationDispatchResult{}, ctx.Err()
	}
}

func (b *controlledWorkstationBoundary) CancelWorkstationDispatch(
	_ context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	b.cancels <- request
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

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
	if retired.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf(
			"AcceptDispatchResult(after canonical Workers completion) outcome = %q, want DUPLICATE_IDEMPOTENT",
			retired.Outcome,
		)
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
	requireRootErrIs(t, err, factory.ErrUnknownDispatchCorrelation, "AcceptDispatchResult(failed unknown)")

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.PlanDispatch(ctx, plan)
	requireRootErrIs(t, err, factory.ErrNotRunning, "PlanDispatch(unknown)")
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID: "dispatch-3", CorrelationID: "corr-3", WorkID: "work-3",
		ResultOutcome: factory.DispatchResultOutcomeCancelled,
	})
	requireRootErrIs(t, err, factory.ErrNotRunning, "AcceptDispatchResult(unknown)")
}

func TestFactoryImpl_AcceptDispatchResultAppliesCanonicalProgressionOnce(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	executor := &countingWorkerExecutor{}
	runtime, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withWorkerService(boundary),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: "work-result-ingress", WorkTypeID: "task", TraceID: "trace-result-ingress",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(t.Context()) }()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	assertCompleteCanonicalWorkersRequest(t, request)
	if executor.calls.Load() != 0 {
		t.Fatalf("Runtime invoked executor %d times outside Workers boundary", executor.calls.Load())
	}

	accepted := factory.AcceptDispatchResultRequest{
		DispatchID: request.Execution.Dispatch.DispatchID, CorrelationID: request.Execution.Dispatch.DispatchID,
		WorkID: "work-result-ingress", ResultOutcome: factory.DispatchResultOutcomeSuccess,
	}
	first, err := impl.AcceptDispatchResult(t.Context(), accepted)
	requireNoRootErr(t, err, "AcceptDispatchResult(first)")
	if first.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("first result outcome = %q, want RETIRED", first.Outcome)
	}
	duplicate, err := impl.AcceptDispatchResult(t.Context(), accepted)
	requireNoRootErr(t, err, "AcceptDispatchResult(duplicate)")
	if duplicate.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("duplicate result outcome = %q, want DUPLICATE_IDEMPOTENT", duplicate.Outcome)
	}
	boundary.results <- completedWorkersResult(request)
	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCanonicalResultProgression(t, runtime, ledger)
	if executor.calls.Load() != 0 {
		t.Fatalf("Runtime invoked executor %d times outside Workers boundary", executor.calls.Load())
	}
}

func TestFactoryImpl_TerminationStopsOutboxCancelsWorkersAndAcceptsLateDuplicate(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()), withServiceMode(), withWorkerService(boundary),
		withWorkerExecutor("mock", &passExecutor{}), withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: "work-terminate", WorkTypeID: "task", TraceID: "trace-terminate",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(t.Context()) }()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	terminated, err := impl.ControlTerminate(t.Context(), factory.TerminateRequest{Reason: "operator stop"})
	requireNoRootErr(t, err, "ControlTerminate")
	if terminated.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("ControlTerminate outcome = %q, want ACCEPTED", terminated.Outcome)
	}
	cancelRequest := awaitWorkersCancellation(t, boundary.cancels)
	if cancelRequest.DispatchID != request.Execution.Dispatch.DispatchID {
		t.Fatalf("cancel dispatch ID = %q, want %q", cancelRequest.DispatchID, request.Execution.Dispatch.DispatchID)
	}
	boundary.results <- canceledWorkersResult(request)
	if err := <-runDone; err != nil {
		t.Fatalf("Run after termination: %v", err)
	}
	assertStoppedRuntimeLateResult(t, impl, request)
}

func TestFactoryImpl_RunCancellationPropagatesThroughWorkersBoundary(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()), withServiceMode(), withWorkerService(boundary),
		withWorkerExecutor("mock", &passExecutor{}), withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: "work-cancel", WorkTypeID: "task", TraceID: "trace-cancel",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	runCtx, cancel := context.WithCancel(t.Context())
	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(runCtx) }()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	cancel()
	cancelRequest := awaitWorkersCancellation(t, boundary.cancels)
	if cancelRequest.DispatchID != request.Execution.Dispatch.DispatchID {
		t.Fatalf("cancel dispatch ID = %q, want %q", cancelRequest.DispatchID, request.Execution.Dispatch.DispatchID)
	}
	boundary.results <- canceledWorkersResult(request)
	if err := <-runDone; err != nil {
		t.Fatalf("Run after context cancellation: %v", err)
	}
	if state := impl.dispatchPlan.State(); state.Mode != "STOPPED" || state.StopReason != "CANCELLED" {
		t.Fatalf("cancelled Runtime outbox state = %#v", state)
	}
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

func awaitCanonicalWorkersRequest(
	t *testing.T,
	requests <-chan workers.WorkstationDispatchRequest,
) workers.WorkstationDispatchRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for canonical Workers publication")
		return workers.WorkstationDispatchRequest{}
	}
}

func awaitWorkersCancellation(
	t *testing.T,
	requests <-chan workers.WorkstationDispatchCancelRequest,
) workers.WorkstationDispatchCancelRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for canonical Workers cancellation")
		return workers.WorkstationDispatchCancelRequest{}
	}
}

func assertCompleteCanonicalWorkersRequest(
	t *testing.T,
	request workers.WorkstationDispatchRequest,
) {
	t.Helper()
	dispatch := request.Execution.Dispatch
	if request.WorkstationName != "Process" || dispatch.WorkstationName != "Process" {
		t.Fatalf("canonical workstation route = (%q, %q), want Process", request.WorkstationName, dispatch.WorkstationName)
	}
	if request.Execution.WorkerType != "mock" || dispatch.WorkerType != "mock" {
		t.Fatalf("canonical Worker type = (%q, %q), want mock", request.Execution.WorkerType, dispatch.WorkerType)
	}
	if request.Execution.FactorySessionID != "~default" {
		t.Fatalf("canonical Factory Session ID = %q, want ~default", request.Execution.FactorySessionID)
	}
	if dispatch.DispatchID == "" || dispatch.TransitionID != "t-process" ||
		dispatch.Execution.ReplayKey == "" || len(dispatch.Execution.WorkIDs) != 1 ||
		dispatch.Execution.WorkIDs[0] != "work-result-ingress" || len(request.Execution.InputTokens) == 0 {
		t.Fatalf("canonical Workers request lost dispatch facts: %#v", request)
	}
}

func completedWorkersResult(
	request workers.WorkstationDispatchRequest,
) workers.WorkstationDispatchResult {
	dispatch := request.Execution.Dispatch
	return workers.WorkstationDispatchResult{
		DispatchID: dispatch.DispatchID, WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
			Outcome: workers.OutcomeAccepted, Output: "late duplicate",
		},
	}
}

func canceledWorkersResult(
	request workers.WorkstationDispatchRequest,
) workers.WorkstationDispatchResult {
	dispatch := request.Execution.Dispatch
	return workers.WorkstationDispatchResult{
		DispatchID: dispatch.DispatchID, WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		Result: workers.WorkResult{
			DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
			Outcome: workers.OutcomeFailed, Error: workers.ErrWorkstationDispatchCanceled.Error(),
		},
	}
}

func assertCanonicalResultProgression(
	t *testing.T,
	runtime factory.Factory,
	ledger interface {
		CallCount(string) int
		CallsSnapshot() []string
	},
) {
	t.Helper()
	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatal("runtime is not factoryImpl")
	}
	observed, err := impl.Observe(t.Context(), factory.ObserveRequest{Scope: factory.ObservationScopeFull})
	requireNoRootErr(t, err, "Observe")
	if observed.Observation.Progress.InFlightDispatchCount != 0 ||
		len(observed.Observation.Results) != 1 ||
		observed.Observation.Results[0].WorkID != "work-result-ingress" ||
		observed.Observation.Results[0].Outcome != string(workers.OutcomeAccepted) {
		t.Fatalf("canonical result progression observation = %#v", observed.Observation)
	}
	if ledger.CallCount("RecordWorkstationRequest") != 1 ||
		ledger.CallCount("RecordWorkstationResponse") != 1 {
		t.Fatalf(
			"canonical event calls = request:%d response:%d, want one each",
			ledger.CallCount("RecordWorkstationRequest"),
			ledger.CallCount("RecordWorkstationResponse"),
		)
	}
	calls := ledger.CallsSnapshot()
	requestIndex, responseIndex := -1, -1
	for index, call := range calls {
		switch call {
		case "RecordWorkstationRequest":
			requestIndex = index
		case "RecordWorkstationResponse":
			responseIndex = index
		}
	}
	if requestIndex < 0 || responseIndex <= requestIndex {
		t.Fatalf("canonical dispatch event order = request:%d response:%d", requestIndex, responseIndex)
	}
}

func assertStoppedRuntimeLateResult(
	t *testing.T,
	impl *factoryImpl,
	request workers.WorkstationDispatchRequest,
) {
	t.Helper()
	dispatchID := request.Execution.Dispatch.DispatchID
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		intent, ok := impl.dispatchPlan.Intent(dispatchID)
		if ok && intent.Result != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	state := impl.dispatchPlan.State()
	if state.Mode != "STOPPED" || impl.workers == nil {
		t.Fatalf("stopped Runtime state = %#v, Workers = %#v", state, impl.workers)
	}
	late, err := impl.AcceptDispatchResult(t.Context(), factory.AcceptDispatchResultRequest{
		DispatchID: dispatchID, CorrelationID: dispatchID, WorkID: "work-terminate",
		ResultOutcome: factory.DispatchResultOutcomeCancelled,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult(late duplicate)")
	if late.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("late result outcome = %q, want DUPLICATE_IDEMPOTENT", late.Outcome)
	}
}

func TestFactoryImpl_CheckpointContracts_DoNotReportFalseSuccess(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()
	impl.state = interfaces.FactoryStatePaused

	captured, err := impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-1"})
	requireNoRootErr(t, err, "CaptureCheckpoint(paused)")
	if captured.Outcome != factory.CheckpointOutcomeCaptured {
		t.Fatalf("CaptureCheckpoint(paused) outcome = %q, want CAPTURED", captured.Outcome)
	}
	if captured.Checkpoint.CheckpointID != "cp-1" ||
		captured.Checkpoint.SchemaVersion <= 0 ||
		len(captured.Checkpoint.Payload) == 0 {
		t.Fatalf("CaptureCheckpoint(paused) checkpoint = %#v, want opaque captured checkpoint", captured.Checkpoint)
	}
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{})
	requireRootErrIs(t, err, factory.ErrCheckpointNotFound, "LoadCheckpoint(empty)")
	loaded, err := impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-1"})
	requireNoRootErr(t, err, "LoadCheckpoint(cp-1)")
	if loaded.Outcome != factory.CheckpointOutcomeLoaded {
		t.Fatalf("LoadCheckpoint(cp-1) outcome = %q, want LOADED", loaded.Outcome)
	}
	if loaded.Checkpoint.CheckpointID != "cp-1" ||
		loaded.Checkpoint.SchemaVersion != captured.Checkpoint.SchemaVersion ||
		len(loaded.Checkpoint.Payload) == 0 {
		t.Fatalf("LoadCheckpoint(cp-1) checkpoint = %#v, want stored opaque checkpoint", loaded.Checkpoint)
	}
	compatible, err := impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{
		CheckpointID:          "cp-1",
		ExpectedSchemaVersion: captured.Checkpoint.SchemaVersion,
	})
	requireNoRootErr(t, err, "LoadCheckpoint(compatible)")
	if !compatible.Compatible {
		t.Fatal("LoadCheckpoint(compatible) Compatible = false, want true")
	}
	incompatible, err := impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{
		CheckpointID:          "cp-1",
		ExpectedSchemaVersion: captured.Checkpoint.SchemaVersion + 1,
	})
	requireNoRootErr(t, err, "LoadCheckpoint(incompatible)")
	if incompatible.Compatible {
		t.Fatal("LoadCheckpoint(incompatible) Compatible = true, want false")
	}
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "missing"})
	requireRootErrIs(t, err, factory.ErrCheckpointNotFound, "LoadCheckpoint(missing)")

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
	loadedAfterComplete, err := impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-1"})
	requireNoRootErr(t, err, "LoadCheckpoint(completed)")
	if loadedAfterComplete.Outcome != factory.CheckpointOutcomeLoaded {
		t.Fatalf("LoadCheckpoint(completed) outcome = %q, want LOADED", loadedAfterComplete.Outcome)
	}
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-2", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	requireRootErrIs(t, err, factory.ErrNotRunning, "RestoreCheckpoint(completed)")
}

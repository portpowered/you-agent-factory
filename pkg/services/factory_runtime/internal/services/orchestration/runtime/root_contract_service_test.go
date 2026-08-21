package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type testRuntimeClock struct{}

func (testRuntimeClock) Now() time.Time { return time.Now() }

type controlledWorkstationBoundary struct {
	workers.ModelInvoker
	requests chan workers.WorkstationDispatchRequest
	results  chan workers.WorkstationDispatchResult
	cancels  chan workers.WorkstationDispatchCancelRequest
}

type countingWorkerExecutor struct {
	calls atomic.Int32
}

type gatedRuntimeLogger struct {
	armed       atomic.Bool
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newGatedRuntimeLogger() *gatedRuntimeLogger {
	return &gatedRuntimeLogger{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (l *gatedRuntimeLogger) arm() {
	l.armed.Store(true)
}

func (l *gatedRuntimeLogger) releaseTick() {
	l.releaseOnce.Do(func() { close(l.release) })
}

func (l *gatedRuntimeLogger) Debug(message string, _ ...any) {
	if message != "transitioner: processing results" || !l.armed.CompareAndSwap(true, false) {
		return
	}
	close(l.entered)
	<-l.release
}

func (l *gatedRuntimeLogger) Info(string, ...any) {}

func (l *gatedRuntimeLogger) Warn(string, ...any)    {}
func (l *gatedRuntimeLogger) Error(string, ...any)   {}
func (l *gatedRuntimeLogger) Verbose(string, ...any) {}

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

func (b *controlledWorkstationBoundary) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	legacy := testLegacyRequestFromExecute(request)
	b.requests <- legacy
	select {
	case result := <-b.results:
		output := workers.ProposedOutputFromLegacyWorkResult(result.Result)
		executeResult := workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     executeOutcomeFromWorkResult(result.Result),
			Failure:     executeFailureFromWorkResult(result.Result),
			Output:      output,
		}
		if result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled {
			executeResult.Outcome = workers.ExecutionOutcomeCanceled
		}
		return executeResult, nil
	case <-ctx.Done():
		b.cancels <- workers.WorkstationDispatchCancelRequest{DispatchID: legacy.Execution.Dispatch.DispatchID}
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeCanceled}, ctx.Err()
	}
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

func TestFactoryImpl_CleanInvocationSnapshotProjectsDetachedRuntimeFacts(t *testing.T) {
	impl := newRootContractTestFactory(t)
	if _, err := submitWorkRequests(context.Background(), impl, []work.SubmitRequest{{
		WorkID: "work-clean-snapshot", WorkTypeID: "task", TraceID: "trace-clean-snapshot",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := impl.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snapshot, err := impl.CleanInvocationSnapshot(context.Background())
	if err != nil {
		t.Fatalf("CleanInvocationSnapshot: %v", err)
	}
	assertCleanInvocationSnapshot(t, snapshot)
}

func assertCleanInvocationSnapshot(t *testing.T, snapshot factory.CleanInvocationSnapshot) {
	t.Helper()
	if len(snapshot.Work) != 1 {
		t.Fatalf("clean Work = %#v, want one terminal item", snapshot.Work)
	}
	terminal := snapshot.Work[0]
	if terminal.WorkID != "work-clean-snapshot" || terminal.WorkTypeID != "task" ||
		terminal.StateCategory != string(factory.StateCategoryTerminal) ||
		terminal.Output != "done" || terminal.TraceID != "trace-clean-snapshot" {
		t.Fatalf("clean terminal Work = %#v, want detached terminal facts", terminal)
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("clean dispatch history = %#v, want one completion", snapshot.DispatchHistory)
	}
	completion := snapshot.DispatchHistory[0]
	if completion.Outcome != string(workers.OutcomeAccepted) || len(completion.Consumed) != 1 || len(completion.Outputs) != 1 {
		t.Fatalf("clean dispatch completion = %#v, want accepted consumed/output facts", completion)
	}
	if completion.Consumed[0].WorkID != terminal.WorkID || completion.Outputs[0].WorkID != terminal.WorkID {
		t.Fatalf("clean dispatch lineage = %#v, want Work %q", completion, terminal.WorkID)
	}
}

func TestCleanInvocationWorkFromTokenHandlesNilAndUnknownTopology(t *testing.T) {
	if got := cleanInvocationWorkFromToken(nil, nil); got != (factory.CleanInvocationWork{}) {
		t.Fatalf("nil token projection = %#v, want zero value", got)
	}
	got := cleanInvocationWorkFromToken(nil, &factorytoken.Token{
		PlaceID: "unknown-place",
		Color: factorytoken.Color{
			WorkID: "work-processing", WorkTypeID: "task", TraceID: "trace-processing",
			DataType: factorytoken.DataTypeWork, Payload: []byte("payload"),
		},
	})
	want := factory.CleanInvocationWork{
		WorkID: "work-processing", WorkTypeID: "task", StateCategory: string(factory.StateCategoryProcessing),
		Output: "payload", TraceID: "trace-processing", DataType: string(factorytoken.DataTypeWork),
	}
	if got != want {
		t.Fatalf("unknown topology projection = %#v, want %#v", got, want)
	}
}

func TestCleanInvocationSnapshotHandlesReaderErrorAndNilSnapshot(t *testing.T) {
	wantErr := errors.New("snapshot unavailable")
	if _, err := cleanInvocationSnapshot(context.Background(), func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return nil, wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("cleanInvocationSnapshot(error) = %v, want %v", err, wantErr)
	}
	got, err := cleanInvocationSnapshot(context.Background(), func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("cleanInvocationSnapshot(nil) = %v, want nil error", err)
	}
	if len(got.Work) != 0 || len(got.DispatchHistory) != 0 {
		t.Fatalf("cleanInvocationSnapshot(nil) = %#v, want zero snapshot", got)
	}
}

func TestProjectCleanInvocationSnapshotSkipsNilTokensAndProjectsFailureFacts(t *testing.T) {
	token := &factorytoken.Token{
		PlaceID: "unknown-place",
		Color: factorytoken.Color{
			WorkID: "work-failed", WorkTypeID: "task", TraceID: "trace-failed",
			DataType: factorytoken.DataTypeWork, Payload: []byte("failed output"),
		},
	}
	workerToken := factorytoken.ToWorker(*token)
	outputToken := workerToken
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			"nil-token":    nil,
			"failed-token": token,
		}},
		DispatchHistory: []interfaces.CompletedDispatch{{
			Outcome:         workers.OutcomeFailed,
			Reason:          "worker failed",
			FailureMetadata: &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeTimeout},
			ConsumedTokens:  []workers.Token{workerToken},
			OutputMutations: []interfaces.TokenMutationRecord{{Token: nil}, {Token: &outputToken}},
		}},
	}

	got := projectCleanInvocationSnapshot(snapshot)
	if len(got.Work) != 1 || got.Work[0].WorkID != "work-failed" {
		t.Fatalf("projected Work = %#v, want one nonnil token", got.Work)
	}
	if len(got.DispatchHistory) != 1 {
		t.Fatalf("projected dispatch history = %#v, want one completion", got.DispatchHistory)
	}
	completion := got.DispatchHistory[0]
	if completion.Outcome != string(workers.OutcomeFailed) || completion.FailureType != string(workers.WorkFailureTypeTimeout) ||
		len(completion.Consumed) != 1 || len(completion.Outputs) != 1 {
		t.Fatalf("projected failure completion = %#v, want failure metadata and nonnil lineage", completion)
	}
}

func TestFactoryImpl_RuntimeConfigurationAccessorsRemainSafe(t *testing.T) {
	impl := newRootContractTestFactory(t)
	impl.SetProgressPublisher(nil)
	impl.SetMockWorkersConfig(&workers.MockWorkersConfig{})
	impl.SetPromptSourceReader(nil)
	if impl.WorkflowContext() != nil {
		t.Fatalf("WorkflowContext() = %#v, want nil for the default test runtime", impl.WorkflowContext())
	}
	var adapter *schedulerAdapter
	if adapter.SupportsRepeatedTransitionBindings() {
		t.Fatal("nil scheduler adapter reports repeated transition bindings")
	}
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

// TestFactoryImpl_RunCancellationAbsorbsLateCanceledResult fixes the ordering
// that previously routed a cancellation-induced result through the ordinary
// FAILED transition path: the result is admitted, the transitioner starts,
// cancellation happens inside Execute, and routing then observes cancellation.
func TestFactoryImpl_RunCancellationAbsorbsLateCanceledResult(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	logger := newGatedRuntimeLogger()
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()), withServiceMode(), withWorkerService(boundary),
		withWorkerExecutor("mock", &passExecutor{}), withLogger(logger),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: "work-cancel-gated", WorkTypeID: "task", TraceID: "trace-cancel-gated",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	defer logger.releaseTick()
	runCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(runCtx) }()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	if request.Execution.Dispatch.TransitionID != "t-process" {
		t.Fatalf("gated dispatch transition = %q, want t-process", request.Execution.Dispatch.TransitionID)
	}
	written := observeNextBufferedResult(t, runtime)
	logger.arm()
	boundary.results <- canceledWorkersResult(request)
	waitForBufferedResult(t, written)
	impl.engine.NotifyResult()
	select {
	case <-logger.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the gated transitioner")
	}
	cancel()
	logger.releaseTick()

	err = <-runDone
	if err != nil {
		t.Fatalf("Run after deterministic late cancellation result = %v, want nil", err)
	}
	state := impl.dispatchPlan.State()
	if state.Mode != "STOPPED" || state.StopReason != "CANCELLED" {
		t.Fatalf("cancelled Runtime outbox state = %#v, want STOPPED/CANCELLED", state)
	}
	snapshot := impl.engine.GetRuntimeStateSnapshot()
	if len(snapshot.Results) != 1 || snapshot.Results[0].Outcome != workers.OutcomeFailed ||
		snapshot.Results[0].Error != workers.ErrWorkstationDispatchCanceled.Error() {
		t.Fatalf("absorbed late cancellation results = %#v, want retained FAILED cancellation result", snapshot.Results)
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

func failedWorkersResult(
	request workers.WorkstationDispatchRequest,
) workers.WorkstationDispatchResult {
	dispatch := request.Execution.Dispatch
	return workers.WorkstationDispatchResult{
		DispatchID: dispatch.DispatchID, WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
			Outcome: workers.OutcomeFailed, Error: "simulated worker session failure",
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
	runtime factoryhost.Engine,
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
	if state.Mode != "STOPPED" {
		t.Fatalf("stopped Runtime state = %#v", state)
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

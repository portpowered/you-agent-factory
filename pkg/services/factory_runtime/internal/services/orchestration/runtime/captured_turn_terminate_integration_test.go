package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionswire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestTerminateFactorySession_FansOutCapturedChildrenBeforeTargetCleanup
// exercises the committed Factory Sessions close boundary against the real
// Runtime selector, Worker Sessions service, and Workers cancellation
// boundary. The controlled dispatches can only finish through the exact
// boundary Cancel call, so caller-context cancellation and target cleanup
// cannot hide a missed child control.
func TestTerminateFactorySession_FansOutCapturedChildrenBeforeTargetCleanup(t *testing.T) {
	execution := newSynchronousFanOutExecution("dispatch-a", "dispatch-b", "dispatch-replacement")
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    execution,
		RouteNames: []string{"review"},
		Async:      false,
	})
	events, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New events service: %v", err)
	}
	workerSessions, err := workersessionswire.NewService(boundary, events, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New Worker Sessions service: %v", err)
	}
	starts, startErrs := startCapturedTurnWorkerSessions(t, workerSessions, execution)
	target, lifecycle := newCapturedTurnTarget(t, workerSessions, execution)

	started, err := target.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	canceledControlContext, cancelControlContext := context.WithCancel(context.Background())
	cancelControlContext()
	err = target.TerminateFactorySession(canceledControlContext, started.SessionID, factorysessions.ControlRequest{
		RequestID: "control-close-captured",
		Reason:    "committed ACP close",
		TurnID:    "turn-captured",
	})
	if err != nil {
		t.Fatalf("TerminateFactorySession: %v", err)
	}
	if lifecycle.stopCallsSnapshot() != 1 {
		t.Fatalf("target cleanup calls = %d, want exactly one after captured child controls", lifecycle.stopCallsSnapshot())
	}
	if got := lifecycle.cleanupCallsSnapshot(); len(got) != 2 {
		t.Fatalf("boundary cancellations before target cleanup = %#v, want both captured children", got)
	}
	if execution.observedCanceledControlContext() {
		t.Fatal("Workers boundary received caller-canceled control context")
	}
	assertCapturedWorkerSessionsTerminated(t, starts, startErrs)
	select {
	case unexpected := <-starts:
		t.Fatalf("replacement Worker Session finished before explicit cleanup: %#v", unexpected)
	default:
	}

	if _, err := workerSessions.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-replacement"}); err != nil {
		t.Fatalf("cleanup replacement Worker Session: %v", err)
	}
	replacement := <-starts
	replacementErr := <-startErrs
	if replacementErr != nil || replacement.Session.ID != "worker-replacement" ||
		replacement.Session.State != workersessions.StateTerminated ||
		!errors.Is(replacement.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("replacement cleanup result = %#v, %v, want separately canceled terminal result", replacement, replacementErr)
	}
}

// TestFactoryResume_IsolatesCapturedChildProviderSessionContinuations enters
// through the real Factory Runtime fan-out and real Worker Sessions service.
// The controlled Workers edge returns one child failure before the other
// succeeds, proving each captured child retains its own exact reference and
// terminal result without reaching the unrelated direct Worker Session.
func TestFactoryResume_IsolatesCapturedChildProviderSessionContinuations(t *testing.T) {
	execution := newContinuationFanOutExecution("dispatch-a", "dispatch-b", "dispatch-direct")
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service: execution, RouteNames: []string{"review"}, Async: true,
	})
	events, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New Events service: %v", err)
	}
	workerSessions, err := workersessionswire.NewService(boundary, events, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New Worker Sessions service: %v", err)
	}
	starts, startErrs := startContinuationFanOutWorkerSessions(t, workerSessions, execution)
	t.Cleanup(func() { execution.cancelInitial("dispatch-direct") })

	references := map[string]providers.SessionRef{
		"worker-a":      {Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-a"},
		"worker-b":      {Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-b"},
		"worker-direct": {Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-direct"},
	}
	for sessionID, dispatchID := range map[string]string{
		"worker-a": "dispatch-a", "worker-b": "dispatch-b", "worker-direct": "dispatch-direct",
	} {
		if _, err := workerSessions.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
			WorkerSessionID: sessionID, DispatchID: dispatchID, Reference: references[sessionID],
		}); err != nil {
			t.Fatalf("AssociateProviderSession(%q): %v", sessionID, err)
		}
	}

	runtimeInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()), withInlineDispatch(), withWorkerSessions(workerSessions),
	)
	if err != nil {
		t.Fatalf("New Factory Runtime: %v", err)
	}
	ledger.Events = append(ledger.Events,
		workerSessionAssociationEvent(t, 20, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 30, "association-other-turn", "turn-other", "worker-direct"),
	)
	control := runtimeInstance.(factoryruntime.Service)

	paused, err := control.ControlPause(context.Background(), factoryruntime.PauseRequest{
		TurnID: "turn-captured", ControlID: "pause-captured-children",
	})
	if err != nil || paused.Outcome != factoryruntime.ControlOutcomeAccepted ||
		paused.WorkerSessionControl.Outcome != factoryruntime.WorkerSessionControlAggregateOutcomeApplied {
		t.Fatalf("ControlPause() = %#v, %v, want accepted captured-child pause", paused, err)
	}
	if got, want := workerSessionIDsFromResults(paused.WorkerSessionControl.Children), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paused Worker Sessions = %v, want %v", got, want)
	}

	resumed, err := control.ControlResume(context.Background(), factoryruntime.ResumeRequest{
		TurnID: "turn-captured", ControlID: "resume-captured-children",
	})
	if err != nil || resumed.Outcome != factoryruntime.ControlOutcomeAccepted ||
		resumed.WorkerSessionControl.Outcome != factoryruntime.WorkerSessionControlAggregateOutcomeApplied {
		t.Fatalf("ControlResume() = %#v, %v, want accepted captured-child continuation", resumed, err)
	}
	requests := execution.continuationRequests(t, 2)
	assertContinuationRequest(t, requests["dispatch-a/resume/1"], references["worker-a"], "turn-captured")
	assertContinuationRequest(t, requests["dispatch-b/resume/1"], references["worker-b"], "turn-captured")

	repeated, err := control.ControlResume(context.Background(), factoryruntime.ResumeRequest{
		TurnID: "turn-captured", ControlID: "resume-captured-children",
	})
	if err != nil || repeated.Outcome != factoryruntime.ControlOutcomeNoOp ||
		!reflect.DeepEqual(repeated.WorkerSessionControl, resumed.WorkerSessionControl) {
		t.Fatalf("duplicate ControlResume() = %#v, %v, want retained no-op evidence", repeated, err)
	}
	if unexpected := execution.continuationRequestIfPresent(); unexpected.Execution.Dispatch.DispatchID != "" {
		t.Fatalf("duplicate resume started unexpected continuation: %#v", unexpected)
	}

	// Finish the foreign child first. The sibling must retain its distinct
	// continuation reference and complete normally despite this failure.
	if err := execution.completeContinuation("dispatch-b/resume/1", failedForeignContinuation("dispatch-b/resume/1")); err != nil {
		t.Fatal(err)
	}
	foreign, foreignStartErr := <-starts, <-startErrs
	if foreignStartErr != nil || foreign.Session.ID != "worker-b" {
		t.Fatalf("out-of-order foreign continuation = %#v, %v, want worker-b first", foreign, foreignStartErr)
	}
	assertContinuationTerminal(t, foreign, "dispatch-b/resume/1", workersessions.StateFailed, references["worker-b"], providers.ContinuationFailureKindForeign)
	if err := execution.completeContinuation("dispatch-a/resume/1", completedContinuation("dispatch-a/resume/1", references["worker-a"])); err != nil {
		t.Fatal(err)
	}
	completed, completedStartErr := <-starts, <-startErrs
	if completedStartErr != nil || completed.Session.ID != "worker-a" {
		t.Fatalf("out-of-order successful continuation = %#v, %v, want worker-a second", completed, completedStartErr)
	}
	assertContinuationTerminal(t, completed, "dispatch-a/resume/1", workersessions.StateCompleted, references["worker-a"], "")

	direct, err := workerSessions.Get(context.Background(), workersessions.GetRequest{ID: "worker-direct"})
	if err != nil || direct.State != workersessions.StateRunning {
		t.Fatalf("unrelated Worker Session = %#v, %v, want unchanged RUNNING session", direct, err)
	}
	if err := execution.assertNoFreshContinuation(); err != nil {
		t.Fatal(err)
	}

	terminated, err := workerSessions.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-direct"})
	if err != nil || terminated.Outcome != workersessions.ControlOutcomeApplied || terminated.Session.State != workersessions.StateTerminated {
		t.Fatalf("Terminate unrelated Worker Session = %#v, %v, want isolated cleanup", terminated, err)
	}
	if result, startErr := <-starts, <-startErrs; startErr != nil || result.Session.ID != "worker-direct" || result.Session.State != workersessions.StateTerminated {
		t.Fatalf("direct Start() result = %#v, %v, want terminated isolated cleanup", result, startErr)
	}
}

func startContinuationFanOutWorkerSessions(
	t *testing.T,
	service workersessions.Service,
	execution *continuationFanOutExecution,
) (<-chan workersessions.StartResult, <-chan error) {
	t.Helper()
	starts := make(chan workersessions.StartResult, 3)
	errs := make(chan error, 3)
	for _, child := range []struct{ sessionID, dispatchID, turnID string }{
		{sessionID: "worker-a", dispatchID: "dispatch-a", turnID: "turn-captured"},
		{sessionID: "worker-b", dispatchID: "dispatch-b", turnID: "turn-captured"},
		{sessionID: "worker-direct", dispatchID: "dispatch-direct", turnID: "turn-direct"},
	} {
		go func(sessionID, dispatchID, turnID string) {
			result, err := service.Start(context.Background(), workersessions.StartRequest{
				ID: sessionID,
				Execution: workers.WorkstationDispatchRequest{WorkstationName: "review", Execution: workers.WorkstationExecutionRequest{
					Dispatch: work.WorkDispatch{DispatchID: dispatchID, WorkstationName: "review", Execution: work.ExecutionMetadata{RequestID: turnID}},
				}},
			})
			starts <- result
			errs <- err
		}(child.sessionID, child.dispatchID, child.turnID)
	}
	for _, dispatchID := range []string{"dispatch-a", "dispatch-b", "dispatch-direct"} {
		<-execution.initialAdmitted(dispatchID)
	}
	return starts, errs
}

func assertContinuationRequest(
	t *testing.T,
	request workers.WorkstationDispatchRequest,
	wantReference providers.SessionRef,
	wantTurnID string,
) {
	t.Helper()
	if request.WorkstationName != "review" || request.Execution.Dispatch.Execution.RequestID != wantTurnID ||
		request.Execution.ResumeSession == nil || *request.Execution.ResumeSession != wantReference {
		t.Fatalf("continuation request = %#v, want review/%q and exact reference %#v", request, wantTurnID, wantReference)
	}
}

func assertContinuationTerminal(
	t *testing.T,
	result workersessions.StartResult,
	wantDispatchID string,
	wantState workersessions.State,
	wantReference providers.SessionRef,
	wantFailure providers.ContinuationFailureKind,
) {
	t.Helper()
	if result.Dispatch.DispatchID != wantDispatchID || result.Session.State != wantState || result.Session.ProviderSessionAssociation == nil ||
		result.Session.ProviderSessionAssociation.Reference != wantReference {
		t.Fatalf("continuation terminal result = %#v, want dispatch %q, %s, and exact association %#v", result, wantDispatchID, wantState, wantReference)
	}
	if wantFailure == "" {
		if result.Session.Result == nil || result.Session.Result.Outcome != workersessions.TerminalOutcomeCompleted {
			t.Fatalf("successful continuation result = %#v, want COMPLETED terminal result", result)
		}
		return
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil ||
		result.Session.Result.Cause.ProviderContinuationFailureKind != wantFailure {
		t.Fatalf("failed continuation result = %#v, want continuation failure %q", result, wantFailure)
	}
}

func startCapturedTurnWorkerSessions(
	t *testing.T,
	workerSessions workersessions.Service,
	execution *synchronousFanOutExecution,
) (<-chan workersessions.StartResult, <-chan error) {
	t.Helper()
	starts := make(chan workersessions.StartResult, 3)
	startErrs := make(chan error, 3)
	children := []struct{ sessionID, dispatchID string }{
		{sessionID: "worker-a", dispatchID: "dispatch-a"},
		{sessionID: "worker-b", dispatchID: "dispatch-b"},
		{sessionID: "worker-replacement", dispatchID: "dispatch-replacement"},
	}
	for _, child := range children {
		go func(sessionID, dispatchID string) {
			started, startErr := workerSessions.Start(context.Background(), workersessions.StartRequest{
				ID: sessionID,
				Execution: workers.WorkstationDispatchRequest{
					WorkstationName: "review",
					Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{
						DispatchID: dispatchID, WorkstationName: "review",
					}},
				},
			})
			starts <- started
			startErrs <- startErr
		}(child.sessionID, child.dispatchID)
	}
	for _, child := range children {
		<-execution.admitted(child.dispatchID)
	}
	return starts, startErrs
}

func newCapturedTurnTarget(
	t *testing.T,
	workerSessions workersessions.Service,
	execution *synchronousFanOutExecution,
) (*factorysessionswire.OnDemandFactoryTargetService, *capturedTurnTargetLifecycle) {
	t.Helper()
	runtimeInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()), withInlineDispatch(), withWorkerSessions(workerSessions),
	)
	if err != nil {
		t.Fatalf("New Factory Runtime: %v", err)
	}
	runtimeService, ok := runtimeInstance.(factoryruntime.Service)
	if !ok {
		t.Fatalf("Factory Runtime = %T, want published Service", runtimeInstance)
	}
	ledger.Events = append(ledger.Events,
		workerSessionAssociationEvent(t, 20, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 30, "association-replacement", "turn-replacement", "worker-replacement"),
	)
	lifecycle := &capturedTurnTargetLifecycle{
		runtime: runtimeService, cancellationCalls: execution.cancelCalls,
		expectedDispatches: []string{"dispatch-a", "dispatch-b"},
	}
	target, err := factorysessionswire.NewOnDemandFactoryTargetService(
		capturedTurnTargetOpening{opened: factorysessionswire.OpenedInvocationRuntime{Lifecycle: lifecycle}},
		factorysessionswire.RuntimeOpeningExternalEffects{},
		func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
			return factorysessions.RuntimeOpeningRequest{}, nil
		},
		func() string { return "target-captured-control" }, zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("New on-demand Factory target: %v", err)
	}
	return target, lifecycle
}

func assertCapturedWorkerSessionsTerminated(
	t *testing.T,
	starts <-chan workersessions.StartResult,
	startErrs <-chan error,
) {
	t.Helper()
	terminatedSessions := make(map[string]workersessions.StartResult, 2)
	for range 2 {
		result := <-starts
		startErr := <-startErrs
		if startErr != nil || result.Session.State != workersessions.StateTerminated ||
			result.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled ||
			!errors.Is(result.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
			t.Fatalf("captured Worker Session result = %#v, %v, want existing canceled callback result", result, startErr)
		}
		terminatedSessions[result.Session.ID] = result
	}
	for _, sessionID := range []string{"worker-a", "worker-b"} {
		if _, ok := terminatedSessions[sessionID]; !ok {
			t.Fatalf("terminated Worker Sessions = %#v, want %q", terminatedSessions, sessionID)
		}
	}
}

type capturedTurnTargetOpening struct {
	opened factorysessionswire.OpenedInvocationRuntime
}

func (o capturedTurnTargetOpening) OpenInvocationRuntime(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
	factorysessionswire.RuntimeOpeningExternalEffects,
	*zap.Logger,
) (factorysessionswire.OpenedInvocationRuntime, error) {
	return o.opened, nil
}

type capturedTurnTargetLifecycle struct {
	runtime            factoryruntime.Service
	cancellationCalls  <-chan workers.WorkstationDispatchCancelRequest
	expectedDispatches []string

	mu           sync.Mutex
	stopCalls    int
	cleanupCalls []workers.WorkstationDispatchCancelRequest
}

func (*capturedTurnTargetLifecycle) StartLifecycle(context.Context, context.Context) error {
	return nil
}

func (*capturedTurnTargetLifecycle) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	return nil, nil
}

func (*capturedTurnTargetLifecycle) CompleteStartup(context.Context) error { return nil }

func (*capturedTurnTargetLifecycle) WaitForRuntime(context.Context) error { return nil }

func (l *capturedTurnTargetLifecycle) StopLifecycle(context.Context) error {
	for _, wantDispatchID := range l.expectedDispatches {
		select {
		case cancellation := <-l.cancellationCalls:
			if cancellation.DispatchID != wantDispatchID {
				return fmt.Errorf("boundary cancellation dispatch = %q, want %q", cancellation.DispatchID, wantDispatchID)
			}
			l.cleanupCalls = append(l.cleanupCalls, cancellation)
		default:
			return fmt.Errorf("target cleanup began before boundary cancellation for %q", wantDispatchID)
		}
	}
	l.mu.Lock()
	l.stopCalls++
	l.mu.Unlock()
	return nil
}

func (*capturedTurnTargetLifecycle) FailStartup(err error) error { return err }

func (l *capturedTurnTargetLifecycle) CurrentRuntimeBundle() factoryruntime.HostedInstance {
	return capturedTurnTargetHostedInstance{runtime: l.runtime}
}

func (l *capturedTurnTargetLifecycle) stopCallsSnapshot() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stopCalls
}

func (l *capturedTurnTargetLifecycle) cleanupCallsSnapshot() []workers.WorkstationDispatchCancelRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]workers.WorkstationDispatchCancelRequest(nil), l.cleanupCalls...)
}

type capturedTurnTargetHostedInstance struct {
	factoryruntime.HostedInstance
	runtime factoryruntime.Service
}

func (i capturedTurnTargetHostedInstance) RuntimeService() factoryruntime.Service { return i.runtime }

// continuationFanOutExecution is a deterministic Workers boundary for the
// multi-child resume integration. Initial attempts can finish only through
// their exact cancellation; resumed attempts publish their detached request
// before the test supplies their independently controlled terminal result.
type continuationFanOutExecution struct {
	mu sync.Mutex

	initial       map[string]*continuationInitialDispatch
	continuations map[string]*continuationDispatch
	started       chan workers.WorkstationDispatchRequest
}

type continuationInitialDispatch struct {
	admitted     chan struct{}
	admittedOnce sync.Once
	canceled     chan struct{}
	canceledOnce sync.Once
}

type continuationDispatch struct {
	completed chan continuationDispatchResult
}

type continuationDispatchResult struct {
	result workers.WorkstationDispatchResult
	err    error
}

var _ workers.WorkstationExecutionService = (*continuationFanOutExecution)(nil)

func newContinuationFanOutExecution(dispatchIDs ...string) *continuationFanOutExecution {
	execution := &continuationFanOutExecution{
		initial:       make(map[string]*continuationInitialDispatch, len(dispatchIDs)),
		continuations: make(map[string]*continuationDispatch),
		started:       make(chan workers.WorkstationDispatchRequest, len(dispatchIDs)),
	}
	for _, dispatchID := range dispatchIDs {
		execution.initial[dispatchID] = &continuationInitialDispatch{
			admitted: make(chan struct{}), canceled: make(chan struct{}),
		}
	}
	return execution
}

func (*continuationFanOutExecution) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStarted}, nil
}

func (*continuationFanOutExecution) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStopped}, nil
}

func (e *continuationFanOutExecution) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return e.DispatchWorkstationWithAdmission(ctx, request, nil)
}

func (e *continuationFanOutExecution) DispatchWorkstationWithAdmission(
	_ context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	if request.Execution.ResumeSession != nil {
		return e.dispatchContinuation(request, admitted)
	}
	return e.dispatchInitial(request, admitted)
}

func (e *continuationFanOutExecution) dispatchInitial(
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	e.mu.Lock()
	dispatch := e.initial[dispatchID]
	e.mu.Unlock()
	if dispatch == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrUnknownWorkstationDispatch
	}
	if admitted != nil {
		admitted()
	}
	dispatch.admittedOnce.Do(func() { close(dispatch.admitted) })
	<-dispatch.canceled
	return canceledContinuationDispatch(dispatchID, request.WorkstationName), workers.ErrWorkstationDispatchCanceled
}

func (e *continuationFanOutExecution) dispatchContinuation(
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	e.mu.Lock()
	dispatch := e.continuations[dispatchID]
	if dispatch == nil {
		dispatch = &continuationDispatch{completed: make(chan continuationDispatchResult, 1)}
		e.continuations[dispatchID] = dispatch
	}
	e.mu.Unlock()
	if admitted != nil {
		admitted()
	}
	e.started <- workers.WorkstationDispatchRequest{
		WorkstationName: request.WorkstationName,
		Execution:       workers.CloneWorkstationExecutionRequest(request.Execution),
	}
	completed := <-dispatch.completed
	return completed.result, completed.err
}

func (e *continuationFanOutExecution) CancelWorkstationDispatch(
	_ context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if !e.cancelInitial(request.DispatchID) {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrUnknownWorkstationDispatch
	}
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

func (e *continuationFanOutExecution) initialAdmitted(dispatchID string) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.initial[dispatchID].admitted
}

func (e *continuationFanOutExecution) cancelInitial(dispatchID string) bool {
	e.mu.Lock()
	dispatch := e.initial[dispatchID]
	e.mu.Unlock()
	if dispatch == nil {
		return false
	}
	dispatch.canceledOnce.Do(func() { close(dispatch.canceled) })
	return true
}

func (e *continuationFanOutExecution) continuationRequests(
	t *testing.T,
	count int,
) map[string]workers.WorkstationDispatchRequest {
	t.Helper()
	requests := make(map[string]workers.WorkstationDispatchRequest, count)
	for range count {
		request := <-e.started
		dispatchID := request.Execution.Dispatch.DispatchID
		if _, duplicate := requests[dispatchID]; duplicate {
			t.Fatalf("duplicate continuation request for dispatch %q", dispatchID)
		}
		requests[dispatchID] = request
	}
	return requests
}

func (e *continuationFanOutExecution) continuationRequestIfPresent() workers.WorkstationDispatchRequest {
	select {
	case request := <-e.started:
		return request
	default:
		return workers.WorkstationDispatchRequest{}
	}
}

func (e *continuationFanOutExecution) assertNoFreshContinuation() error {
	if request := e.continuationRequestIfPresent(); request.Execution.Dispatch.DispatchID != "" {
		return fmt.Errorf("unexpected fresh continuation dispatch %q", request.Execution.Dispatch.DispatchID)
	}
	return nil
}

func (e *continuationFanOutExecution) completeContinuation(
	dispatchID string,
	completed continuationDispatchResult,
) error {
	e.mu.Lock()
	dispatch := e.continuations[dispatchID]
	e.mu.Unlock()
	if dispatch == nil {
		return fmt.Errorf("continuation dispatch %q was not started", dispatchID)
	}
	dispatch.completed <- completed
	return nil
}

func canceledContinuationDispatch(dispatchID, workstationName string) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID: dispatchID, WorkstationName: workstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		Result: workers.WorkResult{DispatchID: dispatchID, Outcome: workers.OutcomeFailed,
			Error: workers.ErrWorkstationDispatchCanceled.Error()},
	}
}

func completedContinuation(dispatchID string, reference providers.SessionRef) continuationDispatchResult {
	return continuationDispatchResult{result: workers.WorkstationDispatchResult{
		DispatchID: dispatchID, WorkstationName: "review",
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeAccepted,
			ProviderSession: &workers.ProviderSessionMetadata{
				Provider: reference.Provider.String(),
				Kind:     reference.Kind,
				ID:       reference.ID,
			},
		},
	}}
}

func failedForeignContinuation(dispatchID string) continuationDispatchResult {
	return continuationDispatchResult{result: workers.WorkstationDispatchResult{
		DispatchID: dispatchID, WorkstationName: "review",
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: dispatchID, Outcome: workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal, Type: workers.WorkFailureTypePermanentBadRequest,
			},
			ProviderContinuationFailureKind: providers.ContinuationFailureKindForeign,
		},
	}}
}

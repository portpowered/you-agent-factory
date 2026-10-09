package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"

	"go.uber.org/goleak"
)

func TestRecordedWorkerSessionLiveIdentityOnlyRebindsRestoredLineage(t *testing.T) {
	t.Parallel()
	const (
		workerSessionID   = "worker-live-identity"
		historicalSession = "factory-historical"
		foreignSession    = "factory-foreign"
		successorSession  = "factory-successor"
	)
	live := &processLocalWorkerSessionService{getByWorkerResult: workersessions.Observation{
		WorkerSessionID: workerSessionID, FactorySessionID: foreignSession,
	}}
	request := workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID}
	restored := &recordedWorkerSessionObservation{
		Service: live, factorySessionID: successorSession,
		restoredWorldState: &interfaces.FactoryWorldState{},
		restoredSessionIDs: map[string]struct{}{historicalSession: {}},
		restoredEventPrefix: []interfaces.FactoryEvent{{Context: interfaces.FactoryEventContext{
			SessionID: stringPointerForRecordedTest(historicalSession),
		}}},
	}
	observation, err := restored.GetObservationByWorkerSessionID(context.Background(), request)
	if err != nil || observation.FactorySessionID != foreignSession {
		t.Fatalf("foreign observation = %#v, %v; want preserved Factory Session", observation, err)
	}
	live.getByWorkerResult.FactorySessionID = historicalSession
	observation, err = restored.GetObservationByWorkerSessionID(context.Background(), request)
	if err != nil || observation.FactorySessionID != successorSession {
		t.Fatalf("restored observation = %#v, %v; want successor Factory Session", observation, err)
	}
}

func TestRecordedWorkerSessionObservationSelectsPreparedRestoredHistory(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-restored-growth"
	events := recordedObservationTestEvents(t, base, workID)
	prefix := append([]interfaces.FactoryEvent(nil), events...)
	events = append(events, interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			Tick: 6, Sequence: 1, EventTime: base.Add(6 * time.Second),
			WorkIDs: stringSliceForRecordedTest([]string{workID}),
		},
		Type: interfaces.FactoryEventTypeWorkStateChange,
	})
	restored := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{workID: {ID: workID}},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{
			{
				DispatchID:  "dispatch-early",
				StartedAt:   base.Add(time.Second),
				CompletedAt: base.Add(3 * time.Second),
				WorkItemIDs: []string{workID},
				Result:      interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			},
			{
				DispatchID:  "dispatch-late",
				StartedAt:   base.Add(5 * time.Second),
				CompletedAt: base.Add(7 * time.Second),
				WorkItemIDs: []string{workID},
				Result:      interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			},
		},
	}

	t.Run("selects appended runtime facts", func(t *testing.T) {
		projectorCalls := 0
		service := newRecordedWorkerSessionObservationWithRestoredState(
			nil,
			&recordingfixtures.ScriptedRuntimeLedger{Events: events},
			func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
				projectorCalls++
				return restored, nil
			},
			platformclock.Real{},
			nil,
			nil,
			"",
			nil,
			&restored,
			prefix,
		)
		prepareScopedTestFacts(service)
		if projectorCalls != 1 {
			t.Fatalf("fixture preparation calls = %d, want one projection of appended facts", projectorCalls)
		}
		projectorCalls = 0

		result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
		if err != nil {
			t.Fatalf("ListObservations() error = %v", err)
		}
		if projectorCalls != 0 {
			t.Fatalf("request projection calls = %d, want prepared selection only", projectorCalls)
		}
		if len(result.Observations) != 2 || result.Observations[0].State != workersessions.StateCompleted || result.Observations[1].State != workersessions.StateCompleted {
			t.Fatalf("selected observations = %#v, want two completed attempts", result.Observations)
		}
	})

	t.Run("propagates projection failure", func(t *testing.T) {
		service := newRecordedWorkerSessionObservationWithRestoredState(
			nil,
			&recordingfixtures.ScriptedRuntimeLedger{Events: events},
			func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
				return interfaces.FactoryWorldState{}, errors.New("projection failed")
			},
			platformclock.Real{},
			nil,
			nil,
			"",
			nil,
			&restored,
			prefix,
		)
		prepareScopedTestFacts(service)

		_, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
		if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
			t.Fatalf("ListObservations() error = %v, want projection unavailable", err)
		}
	})
}

// TestTerminateCapturedTurn_FansOutChildrenBeforeCleanup
// exercises Factory Runtime control against the Runtime selector, a
// Worker Sessions contract double, and Workers cancellation boundary.
// The controlled dispatches can only finish through the exact
// boundary Cancel call, so caller-context cancellation and target cleanup
// cannot hide a missed child control.
func TestTerminateCapturedTurn_FansOutChildrenBeforeCleanup(t *testing.T) {
	execution := newSynchronousFanOutExecution("dispatch-a", "dispatch-b", "dispatch-replacement")
	workerSessions := newCapturedTurnWorkerSessions(execution)
	starts, startErrs := startCapturedTurnWorkerSessions(t, workerSessions, execution)
	runtimeService, cleanup := newCapturedTurnRuntime(t, workerSessions, execution)

	canceledControlContext, cancelControlContext := context.WithCancel(context.Background())
	cancelControlContext()
	_, err := runtimeService.ControlTerminate(context.WithoutCancel(canceledControlContext), factoryruntime.TerminateRequest{
		ControlID:           "control-close-captured",
		Reason:              "committed ACP close",
		TurnID:              "turn-captured",
		WorkerSessionAction: factoryruntime.WorkerSessionControlActionTerminate,
	})
	if err != nil {
		t.Fatalf("ControlTerminate: %v", err)
	}
	if err := cleanup.verifyAfterControl(); err != nil {
		t.Fatalf("verify cleanup after control: %v", err)
	}
	if cleanup.stopCallsSnapshot() != 1 {
		t.Fatalf("target cleanup calls = %d, want exactly one after captured child controls", cleanup.stopCallsSnapshot())
	}
	if got := cleanup.cleanupCallsSnapshot(); len(got) != 2 {
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

	_, err = workerSessions.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-replacement"})
	if err != nil {
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
// through the Factory Runtime fan-out and a Worker Sessions contract double.
// The controlled Workers edge returns one child failure before the other
// succeeds, proving each captured child retains its own exact reference and
// terminal result without reaching the unrelated direct Worker Session.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestFactoryResume_IsolatesCapturedChildProviderSessionContinuations(t *testing.T) {
	execution := newContinuationFanOutExecution("dispatch-a", "dispatch-b", "dispatch-direct")
	workerSessions := newCapturedTurnWorkerSessions(execution)
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
) (<-chan workersessions.InvokeSessionResult, <-chan error) {
	t.Helper()
	starts := make(chan workersessions.InvokeSessionResult, 3)
	errs := make(chan error, 3)
	for _, child := range []struct{ sessionID, dispatchID, turnID string }{
		{sessionID: "worker-a", dispatchID: "dispatch-a", turnID: "turn-captured"},
		{sessionID: "worker-b", dispatchID: "dispatch-b", turnID: "turn-captured"},
		{sessionID: "worker-direct", dispatchID: "dispatch-direct", turnID: "turn-direct"},
	} {
		go func(sessionID, dispatchID, turnID string) {
			result, err := service.InvokeSession(context.Background(), workersessions.InvokeSessionRequest{
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
	continuation := request.Execution.Continuation
	var gotReference providers.SessionRef
	var err error
	if continuation != nil {
		gotReference, err = continuation.ToSessionRef()
	}
	if request.WorkstationName != "review" || request.Execution.Dispatch.Execution.RequestID != wantTurnID ||
		continuation == nil || err != nil || gotReference != wantReference {
		t.Fatalf("continuation request = %#v, want review/%q and exact reference %#v", request, wantTurnID, wantReference)
	}
}

func assertContinuationTerminal(
	t *testing.T,
	result workersessions.InvokeSessionResult,
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
) (<-chan workersessions.InvokeSessionResult, <-chan error) {
	t.Helper()
	starts := make(chan workersessions.InvokeSessionResult, 3)
	startErrs := make(chan error, 3)
	children := []struct{ sessionID, dispatchID string }{
		{sessionID: "worker-a", dispatchID: "dispatch-a"},
		{sessionID: "worker-b", dispatchID: "dispatch-b"},
		{sessionID: "worker-replacement", dispatchID: "dispatch-replacement"},
	}
	for _, child := range children {
		go func(sessionID, dispatchID string) {
			started, startErr := workerSessions.InvokeSession(context.Background(), workersessions.InvokeSessionRequest{
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

func newCapturedTurnRuntime(
	t *testing.T,
	workerSessions workersessions.Service,
	execution *synchronousFanOutExecution,
) (factoryruntime.Service, *capturedTurnCleanupProbe) {
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
	cleanup := &capturedTurnCleanupProbe{
		cancellationCalls:  execution.cancelCalls,
		expectedDispatches: []string{"dispatch-a", "dispatch-b"},
	}
	return runtimeService, cleanup
}

func assertCapturedWorkerSessionsTerminated(
	t *testing.T,
	starts <-chan workersessions.InvokeSessionResult,
	startErrs <-chan error,
) {
	t.Helper()
	terminatedSessions := make(map[string]workersessions.InvokeSessionResult, 2)
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

type capturedTurnCleanupProbe struct {
	cancellationCalls  <-chan workers.WorkstationDispatchCancelRequest
	expectedDispatches []string

	mu           sync.Mutex
	stopCalls    int
	cleanupCalls []workers.WorkstationDispatchCancelRequest
}

func (l *capturedTurnCleanupProbe) verifyAfterControl() error {
	observed := make(map[string]struct{}, len(l.expectedDispatches))
	for range l.expectedDispatches {
		select {
		case cancellation := <-l.cancellationCalls:
			if _, duplicate := observed[cancellation.DispatchID]; duplicate {
				return fmt.Errorf("duplicate boundary cancellation dispatch = %q", cancellation.DispatchID)
			}
			observed[cancellation.DispatchID] = struct{}{}
			l.cleanupCalls = append(l.cleanupCalls, cancellation)
		case <-time.After(time.Second):
			return fmt.Errorf("target cleanup did not observe all boundary cancellations: got %#v", observed)
		}
	}
	for _, wantDispatchID := range l.expectedDispatches {
		if _, ok := observed[wantDispatchID]; !ok {
			return fmt.Errorf("boundary cancellations = %#v, want dispatch %q", observed, wantDispatchID)
		}
	}
	l.mu.Lock()
	l.stopCalls++
	l.mu.Unlock()
	return nil
}

func (l *capturedTurnCleanupProbe) stopCallsSnapshot() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stopCalls
}

func (l *capturedTurnCleanupProbe) cleanupCallsSnapshot() []workers.WorkstationDispatchCancelRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]workers.WorkstationDispatchCancelRequest(nil), l.cleanupCalls...)
}

// continuationFanOutExecution is a deterministic Workers boundary for the
// multi-child resume integration. Initial attempts can finish only through
// their exact cancellation; resumed attempts publish their detached request
// before the test supplies their independently controlled terminal result.
type continuationFanOutExecution struct {
	workers.ModelInvoker
	mu sync.Mutex

	initial       map[string]*continuationInitialDispatch
	continuations map[string]*continuationDispatch
	started       chan workers.WorkstationDispatchRequest
	cancelCalls   chan workers.WorkstationDispatchCancelRequest
}

type continuationInitialDispatch struct {
	admitted     chan struct{}
	admittedOnce sync.Once
	cancel       context.CancelFunc
}

type continuationDispatch struct {
	completed chan continuationDispatchResult
}

type continuationDispatchResult struct {
	result workers.WorkstationDispatchResult
	err    error
}

var _ workers.Service = (*continuationFanOutExecution)(nil)

func newContinuationFanOutExecution(dispatchIDs ...string) *continuationFanOutExecution {
	execution := &continuationFanOutExecution{
		initial:       make(map[string]*continuationInitialDispatch, len(dispatchIDs)),
		continuations: make(map[string]*continuationDispatch),
		started:       make(chan workers.WorkstationDispatchRequest, len(dispatchIDs)),
		cancelCalls:   make(chan workers.WorkstationDispatchCancelRequest, len(dispatchIDs)),
	}
	for _, dispatchID := range dispatchIDs {
		execution.initial[dispatchID] = &continuationInitialDispatch{
			admitted: make(chan struct{}),
		}
	}
	return execution
}

func (e *continuationFanOutExecution) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	legacy := testLegacyRequestFromExecute(request)
	if legacy.Execution.Continuation != nil {
		return e.executeContinuation(ctx, request, legacy)
	}
	return e.executeInitial(ctx, request, legacy)
}

func (e *continuationFanOutExecution) executeInitial(
	ctx context.Context,
	request workers.ExecuteRequest,
	legacy workers.WorkstationDispatchRequest,
) (workers.ExecuteResult, error) {
	dispatchID := legacy.Execution.Dispatch.DispatchID
	e.mu.Lock()
	dispatch := e.initial[dispatchID]
	e.mu.Unlock()
	if dispatch == nil {
		return workers.ExecuteResult{}, workers.ErrUnknownWorkstationDispatch
	}
	attemptContext, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	dispatch.cancel = cancel
	e.mu.Unlock()
	dispatch.admittedOnce.Do(func() { close(dispatch.admitted) })
	<-attemptContext.Done()
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeCanceled}, workers.ErrWorkstationDispatchCanceled
}

func (e *continuationFanOutExecution) executeContinuation(
	ctx context.Context,
	request workers.ExecuteRequest,
	legacy workers.WorkstationDispatchRequest,
) (workers.ExecuteResult, error) {
	dispatchID := legacy.Execution.Dispatch.DispatchID
	e.mu.Lock()
	dispatch := e.continuations[dispatchID]
	if dispatch == nil {
		dispatch = &continuationDispatch{completed: make(chan continuationDispatchResult, 1)}
		e.continuations[dispatchID] = dispatch
	}
	e.mu.Unlock()
	select {
	case e.started <- legacy:
	case <-ctx.Done():
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeCanceled}, ctx.Err()
	}
	completed := <-dispatch.completed
	return testExecuteResultFromDispatchResult(request, completed.result), completed.err
}

func (e *continuationFanOutExecution) initialAdmitted(dispatchID string) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.initial[dispatchID].admitted
}

func (e *continuationFanOutExecution) cancelInitial(dispatchID string) bool {
	e.mu.Lock()
	dispatch := e.initial[dispatchID]
	cancel := func() {}
	if dispatch != nil && dispatch.cancel != nil {
		cancel = dispatch.cancel
	}
	e.mu.Unlock()
	if dispatch == nil {
		return false
	}
	e.cancelCalls <- workers.WorkstationDispatchCancelRequest{DispatchID: dispatchID}
	cancel()
	return true
}

func (e *continuationFanOutExecution) continuationRequests(
	t *testing.T,
	count int,
) map[string]workers.WorkstationDispatchRequest {
	t.Helper()
	requests := make(map[string]workers.WorkstationDispatchRequest, count)
	for range count {
		var request workers.WorkstationDispatchRequest
		select {
		case request = <-e.started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for continuation requests: got %d of %d", len(requests), count)
		}
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

func completedContinuation(dispatchID string, reference providers.SessionRef) continuationDispatchResult {
	return continuationDispatchResult{result: workers.WorkstationDispatchResult{
		DispatchID: dispatchID, WorkstationName: "review",
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeAccepted,
			Continuation: func() *providers.ContinuationRef {
				continuation := reference.ContinuationRef()
				return &continuation
			}(),
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

// capturedTurnWorkerSessions supplies the Worker Sessions contract consumed by
// Runtime while keeping execution and each session's exact reference observable.
type capturedTurnWorkerSessions struct {
	*fakeWorkerSessionsService
	mu       sync.Mutex
	sessions map[string]*capturedTurnSession
}

type capturedTurnSession struct {
	request workersessions.InvokeSessionRequest
	session workersessions.Session
	resume  chan workers.WorkstationDispatchRequest
}

func newCapturedTurnWorkerSessions(execution workers.Service) *capturedTurnWorkerSessions {
	return &capturedTurnWorkerSessions{
		fakeWorkerSessionsService: &fakeWorkerSessionsService{execution: execution},
		sessions:                  make(map[string]*capturedTurnSession),
	}
}

func (s *capturedTurnWorkerSessions) InvokeSession(ctx context.Context, request workersessions.InvokeSessionRequest) (workersessions.InvokeSessionResult, error) {
	entry := &capturedTurnSession{
		request: request,
		session: workersessions.Session{ID: request.ID, State: workersessions.StateRunning},
		resume:  make(chan workers.WorkstationDispatchRequest, 1),
	}
	s.mu.Lock()
	s.sessions[request.ID] = entry
	s.mu.Unlock()
	dispatch := request.Execution
	for {
		result, err := s.execution.Execute(ctx, testExecuteRequestFromDispatch(dispatch))
		converted := testDispatchResultFromExecute(dispatch, result, err)
		if result.Failure != nil {
			converted.Result.ProviderContinuationFailureKind = result.Failure.ProviderContinuationFailureKind
		}
		s.mu.Lock()
		state := entry.session.State
		if state == workersessions.StatePaused {
			s.mu.Unlock()
			dispatch = <-entry.resume
			continue
		}
		// Resume may have queued the successor before the canceled initial
		// execution returned. Consume it before classifying that result.
		if state == workersessions.StateRunning {
			select {
			case dispatch = <-entry.resume:
				s.mu.Unlock()
				continue
			default:
			}
		}
		if state != workersessions.StateTerminated {
			if converted.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCompleted {
				entry.session.State = workersessions.StateCompleted
				entry.session.Result = &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
			} else {
				entry.session.State = workersessions.StateFailed
				entry.session.Result = &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeFailed,
					Cause: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure,
						Detail:                          "controlled Workers execution failed",
						ProviderContinuationFailureKind: converted.Result.ProviderContinuationFailureKind}}
			}
		}
		session := entry.session.Clone()
		s.mu.Unlock()
		return workersessions.InvokeSessionResult{Session: session, Dispatch: converted, DispatchErr: err}, nil
	}
}

func (s *capturedTurnWorkerSessions) Get(_ context.Context, request workersessions.GetRequest) (workersessions.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.sessions[request.ID]
	if entry == nil {
		return workersessions.Session{}, workersessions.ErrSessionNotFound
	}
	return entry.session.Clone(), nil
}

func (s *capturedTurnWorkerSessions) AssociateProviderSession(_ context.Context, request workersessions.ProviderSessionAssociationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.sessions[request.WorkerSessionID]
	association := workersessions.ProviderSessionAssociation{
		WorkerSessionID: request.WorkerSessionID, DispatchID: request.DispatchID,
		AttemptID: request.DispatchID, TurnID: entry.request.Execution.Execution.Dispatch.Execution.RequestID,
		Reference: request.Reference,
	}
	entry.session.ProviderSessionAssociation = &association
	return workersessions.ProviderSessionAssociationResult{Association: association, Outcome: workersessions.ProviderSessionAssociationOutcomeAccepted}, nil
}

func (s *capturedTurnWorkerSessions) Pause(ctx context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	entry := s.sessions[request.ID]
	entry.session.State = workersessions.StatePaused
	dispatchID := entry.request.Execution.Execution.Dispatch.DispatchID
	session := entry.session.Clone()
	s.mu.Unlock()
	err := s.cancel(ctx, dispatchID)
	return workersessions.ControlResult{Session: session, Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeApplied, DispatchID: dispatchID}, err
}

func (s *capturedTurnWorkerSessions) Resume(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	entry := s.sessions[request.ID]
	dispatch := entry.request.Execution
	dispatch.Execution.Dispatch.DispatchID += "/resume/1"
	reference := entry.session.ProviderSessionAssociation.Reference.ContinuationRef()
	dispatch.Execution.Continuation = &reference
	entry.session.State = workersessions.StateRunning
	session := entry.session.Clone()
	entry.resume <- dispatch
	s.mu.Unlock()
	return workersessions.ControlResult{Session: session, Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeApplied, DispatchID: dispatch.Execution.Dispatch.DispatchID}, nil
}

func (s *capturedTurnWorkerSessions) Terminate(ctx context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	entry := s.sessions[request.ID]
	entry.session.State = workersessions.StateTerminated
	dispatchID := entry.request.Execution.Execution.Dispatch.DispatchID
	session := entry.session.Clone()
	s.mu.Unlock()
	err := s.cancel(ctx, dispatchID)
	return workersessions.ControlResult{Session: session, Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeApplied, DispatchID: dispatchID}, err
}

func (s *capturedTurnWorkerSessions) cancel(ctx context.Context, dispatchID string) error {
	switch execution := s.execution.(type) {
	case *synchronousFanOutExecution:
		return execution.cancel(ctx, dispatchID)
	case *continuationFanOutExecution:
		execution.cancelInitial(dispatchID)
		<-execution.cancelCalls
	}
	return nil
}

var _ workersessions.Service = (*capturedTurnWorkerSessions)(nil)

// TestMain fails the package when a test leaves goroutines running, which
// otherwise surfaces as teardown hangs and cross-test interference.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func (*continuationFanOutExecution) ValidateExecution(_ context.Context, request workers.ExecuteRequest) error {
	return request.Validate()
}

func TestRecordedWorkerSessionObservationBindsPreparedRestorationBeforeFirstRead(t *testing.T) {
	t.Parallel()
	const historical = "factory-historical"
	prefix := []interfaces.FactoryEvent{{Context: interfaces.FactoryEventContext{
		SessionID: stringPointerForRecordedTest("  " + historical + "  "),
	}}}
	live := &processLocalWorkerSessionService{getByWorkerResult: workersessions.Observation{
		WorkerSessionID: "worker-restored", FactorySessionID: historical,
	}}
	cfg := &runtimeConfig{workerSessions: live, restoredWorldState: &interfaces.FactoryWorldState{}, restoredEventPrefix: prefix}
	runtime := newFactoryImpl(cfg, nil, nil, nil, nil, nil, nil)
	// The caller retains its startup input; later edits cannot change membership.
	*prefix[0].Context.SessionID = "factory-foreign"
	for _, scope := range []string{"factory-successor-a", "factory-successor-b"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			reader := runtime.WorkerSessionsObservationForSession(scope)
			request := workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-restored"}
			first, err := reader.GetObservationByWorkerSessionID(context.Background(), request)
			if err != nil || first.FactorySessionID != scope {
				t.Fatalf("first restored read = %#v, %v; want scope %q", first, err, scope)
			}
			first.FactorySessionID = "poison"
			second, err := reader.GetObservationByWorkerSessionID(context.Background(), request)
			if err != nil || second.FactorySessionID != scope {
				t.Fatalf("subsequent restored read = %#v, %v; want scope %q", second, err, scope)
			}
		})
	}
}

func TestRecordedWorkerSessionObservationReplayBindingKeepsExistingView(t *testing.T) {
	t.Parallel()
	runtime := newFactoryImpl(&runtimeConfig{restoredWorldState: &interfaces.FactoryWorldState{}}, nil, nil, nil, nil, nil, nil)
	input := []interfaces.FactoryEvent{{Id: "replay-before", Context: interfaces.FactoryEventContext{
		SessionID: stringPointerForRecordedTest("factory-original"),
	}}}
	runtime.SetReplayEvents(input)
	first := runtime.WorkerSessionsObservationForSession("successor").(*recordedWorkerSessionObservation)
	input[0].Id = "caller-mutation"
	*input[0].Context.SessionID = "foreign"
	runtime.SetReplayEvents([]interfaces.FactoryEvent{{Id: "replay-after"}})
	second := runtime.WorkerSessionsObservationForSession("successor").(*recordedWorkerSessionObservation)
	if got := first.canonicalEvents(); len(got) != 1 || got[0].Id != "replay-before" {
		t.Fatalf("existing view lost its replay snapshot: %#v", got)
	}
	if got := second.canonicalEvents(); len(got) != 1 || got[0].Id != "replay-after" {
		t.Fatalf("new view did not observe the replay replacement: %#v", got)
	}
	observation := workersessions.Observation{FactorySessionID: "factory-original"}
	if !first.liveObservationBelongsToRestoredPrefix(observation) || !second.liveObservationBelongsToRestoredPrefix(observation) {
		t.Fatal("replay replacement lost original restoration membership")
	}
	observation.Direct = true
	if first.liveObservationBelongsToRestoredPrefix(observation) {
		t.Fatal("direct worker acquired restored attribution")
	}
}

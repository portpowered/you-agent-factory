package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFanOutWorkerSessionControl_AttemptsEveryCapturedChildInStableOrder(t *testing.T) {
	boom := errors.New("worker session control failed")
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeUnsupported},
		},
		{action: factory.WorkerSessionControlActionPause, id: "worker-c"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-c", Outcome: workersessions.ControlOutcomeFailed},
			err:    boom,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := fanOutWorkerSessionControl(ctx, service, capturedWorkerSessionControlTargets{
		turnID:           "turn-captured",
		workerSessionIDs: []string{"worker-a", "worker-b", "worker-c"},
	}, factory.WorkerSessionControlActionPause)

	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
		t.Fatalf("aggregate outcome = %q, want FAILED", result.Outcome)
	}
	wantChildren := []factory.WorkerSessionControlChildResult{
		{WorkerSessionID: "worker-a", DispatchID: "dispatch-a", Outcome: factory.WorkerSessionControlChildOutcomeApplied},
		{WorkerSessionID: "worker-b", DispatchID: "dispatch-b", Outcome: factory.WorkerSessionControlChildOutcomeUnsupported},
		{WorkerSessionID: "worker-c", DispatchID: "dispatch-c", Outcome: factory.WorkerSessionControlChildOutcomeFailed},
	}
	if !reflect.DeepEqual(result.Children, wantChildren) {
		t.Fatalf("child results = %#v, want %#v", result.Children, wantChildren)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"},
		{action: factory.WorkerSessionControlActionPause, id: "worker-c"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
	if service.observedCanceledContext() {
		t.Fatal("Worker Sessions received canceled fan-out context")
	}
}

func TestFanOutWorkerSessionControl_ClassifiesFullNoOpUnsupportedAndPartialOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []workersessions.ControlOutcome
		want     factory.WorkerSessionControlAggregateOutcome
	}{
		{name: "all applied", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeApplied, workersessions.ControlOutcomeApplied}, want: factory.WorkerSessionControlAggregateOutcomeApplied},
		{name: "all terminal no-op", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeNoop, workersessions.ControlOutcomeNoop}, want: factory.WorkerSessionControlAggregateOutcomeNoOp},
		{name: "all unsupported", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeUnsupported, workersessions.ControlOutcomeUnsupported}, want: factory.WorkerSessionControlAggregateOutcomeUnsupported},
		{name: "mixed outcomes", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeApplied, workersessions.ControlOutcomeNoop, workersessions.ControlOutcomeUnsupported}, want: factory.WorkerSessionControlAggregateOutcomePartial},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := make(map[workerSessionControlCall]workerSessionControlResponse, len(test.outcomes))
			targets := make([]string, 0, len(test.outcomes))
			for index, outcome := range test.outcomes {
				id := string(rune('a' + index))
				targets = append(targets, id)
				responses[workerSessionControlCall{action: factory.WorkerSessionControlActionResume, id: id}] = workerSessionControlResponse{
					result: workersessions.ControlResult{Outcome: outcome},
				}
			}

			result := fanOutWorkerSessionControl(context.Background(), newWorkerSessionControlSpy(responses), capturedWorkerSessionControlTargets{
				turnID: "turn-resume", workerSessionIDs: targets,
			}, factory.WorkerSessionControlActionResume)
			if result.Outcome != test.want {
				t.Fatalf("aggregate outcome = %q, want %q", result.Outcome, test.want)
			}
		})
	}
}

func TestFanOutWorkerSessionControl_MultipleFailuresStillReachEveryChild(t *testing.T) {
	firstFailure := errors.New("first worker failure")
	secondFailure := errors.New("second worker failure")
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"}: {
			result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeFailed}, err: firstFailure,
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"}: {
			result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-c"}: {
			result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeFailed}, err: secondFailure,
		},
	})

	result := fanOutWorkerSessionControl(context.Background(), service, capturedWorkerSessionControlTargets{
		turnID:           "turn-failures",
		workerSessionIDs: []string{"worker-a", "worker-b", "worker-c"},
	}, factory.WorkerSessionControlActionResume)

	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
		t.Fatalf("aggregate outcome = %q, want FAILED", result.Outcome)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-c"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
}

func TestFactoryControls_FanOutCapturedTurnOnceAndRetainOriginalEvidence(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeNoop},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeUnsupported},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-late"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-late", Outcome: workersessions.ControlOutcomeNoop},
		},
	})
	factoryInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ledger.Events = append(ledger.Events,
		workerSessionAssociationEvent(t, 20, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 30, "association-duplicate", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 40, "association-other-turn", "turn-replacement", "worker-replacement"),
	)
	control := factoryInstance.(factory.Service)

	paused, err := control.ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-captured", ControlID: "pause-intent"})
	if err != nil {
		t.Fatalf("ControlPause: %v", err)
	}
	if paused.Outcome != factory.ControlOutcomeAccepted || paused.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomePartial {
		t.Fatalf("pause result = %#v, want accepted partial fan-out", paused)
	}
	if got, want := workerSessionIDsFromResults(paused.WorkerSessionControl.Children), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paused worker session IDs = %v, want %v", got, want)
	}

	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 50, "association-late", "turn-captured", "worker-late"))
	repeatedPause, err := control.ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-captured", ControlID: "pause-intent"})
	if err != nil {
		t.Fatalf("repeated ControlPause: %v", err)
	}
	if repeatedPause.Outcome != factory.ControlOutcomeNoOp || !reflect.DeepEqual(repeatedPause.WorkerSessionControl, paused.WorkerSessionControl) {
		t.Fatalf("repeated pause = %#v, want retained no-op result %#v", repeatedPause, paused.WorkerSessionControl)
	}
	if got := service.callsSnapshot(); len(got) != 2 {
		t.Fatalf("calls after repeated pause = %#v, want exactly the first two child calls", got)
	}

	resumed, err := control.ControlResume(context.Background(), factory.ResumeRequest{TurnID: "turn-captured", ControlID: "resume-intent"})
	if err != nil {
		t.Fatalf("ControlResume: %v", err)
	}
	if resumed.Outcome != factory.ControlOutcomeAccepted || resumed.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomePartial {
		t.Fatalf("resume result = %#v, want accepted partial fan-out", resumed)
	}
	if got, want := workerSessionIDsFromResults(resumed.WorkerSessionControl.Children), []string{"worker-a", "worker-b", "worker-late"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resumed worker session IDs = %v, want %v", got, want)
	}

	repeatedResume, err := control.ControlResume(context.Background(), factory.ResumeRequest{TurnID: "turn-captured", ControlID: "resume-intent"})
	if err != nil {
		t.Fatalf("repeated ControlResume: %v", err)
	}
	if repeatedResume.Outcome != factory.ControlOutcomeNoOp || !reflect.DeepEqual(repeatedResume.WorkerSessionControl, resumed.WorkerSessionControl) {
		t.Fatalf("repeated resume = %#v, want retained no-op result %#v", repeatedResume, resumed.WorkerSessionControl)
	}
	// A committed control may be retried after a later lifecycle transition.
	// Its ControlID must retain the old captured target set rather than
	// including worker-late, which joined this Factory turn after pause first
	// linearized.
	retriedAfterResume, err := control.ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-captured", ControlID: "pause-intent"})
	if err != nil {
		t.Fatalf("ControlPause retry after resume: %v", err)
	}
	if retriedAfterResume.Outcome != factory.ControlOutcomeAccepted || !reflect.DeepEqual(retriedAfterResume.WorkerSessionControl, paused.WorkerSessionControl) {
		t.Fatalf("pause retry after resume = %#v, want accepted retained evidence %#v", retriedAfterResume, paused.WorkerSessionControl)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-late"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
}

func TestFactoryControls_EmptyCapturedTurnDoesNotCallWorkerSessions(t *testing.T) {
	service := newWorkerSessionControlSpy(nil)
	factoryInstance, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	paused, err := factoryInstance.(factory.Service).ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-without-children"})
	if err != nil {
		t.Fatalf("ControlPause: %v", err)
	}
	if paused.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomeNoOp || len(paused.WorkerSessionControl.Children) != 0 {
		t.Fatalf("empty fan-out result = %#v, want no-op with no children", paused.WorkerSessionControl)
	}
	if got := service.callsSnapshot(); len(got) != 0 {
		t.Fatalf("Worker Sessions calls = %#v, want none", got)
	}
}

func TestFanOutWorkerSessionControl_StopActionsReachEveryCapturedChild(t *testing.T) {
	for _, action := range []factory.WorkerSessionControlAction{
		factory.WorkerSessionControlActionCancel,
		factory.WorkerSessionControlActionTerminate,
	} {
		t.Run(string(action), func(t *testing.T) {
			boundaryFailure := errors.New("worker session boundary unavailable")
			service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
				{action: action, id: "worker-a"}: {result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied}},
				{action: action, id: "worker-b"}: {result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeNoop}},
				{action: action, id: "worker-c"}: {result: workersessions.ControlResult{DispatchID: "dispatch-c", Outcome: workersessions.ControlOutcomeFailed}, err: boundaryFailure},
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			result := fanOutWorkerSessionControl(ctx, service, capturedWorkerSessionControlTargets{
				turnID: "turn-stop", workerSessionIDs: []string{"worker-a", "worker-b", "worker-c"},
			}, action)

			if result.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
				t.Fatalf("aggregate outcome = %q, want FAILED", result.Outcome)
			}
			if got, want := service.callsSnapshot(), []workerSessionControlCall{
				{action: action, id: "worker-a"}, {action: action, id: "worker-b"}, {action: action, id: "worker-c"},
			}; !reflect.DeepEqual(got, want) {
				t.Fatalf("control calls = %#v, want %#v", got, want)
			}
			if service.observedCanceledContext() {
				t.Fatal("Worker Sessions received canceled stop fan-out context")
			}
		})
	}
}

func TestFanOutWorkerSessionControl_ContinuesPastSynchronousProductionChild(t *testing.T) {
	execution := newSynchronousFanOutExecution("dispatch-a", "dispatch-b")
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
	starts := make(chan workersessions.StartResult, 2)
	startErrs := make(chan error, 2)
	for _, identity := range []struct{ sessionID, dispatchID string }{
		{sessionID: "worker-a", dispatchID: "dispatch-a"},
		{sessionID: "worker-b", dispatchID: "dispatch-b"},
	} {
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
		}(identity.sessionID, identity.dispatchID)
	}
	<-execution.admitted("dispatch-a")
	<-execution.admitted("dispatch-b")

	result := fanOutWorkerSessionControl(context.Background(), workerSessions, capturedWorkerSessionControlTargets{
		turnID: "turn-synchronous", workerSessionIDs: []string{"worker-a", "worker-b"},
	}, factory.WorkerSessionControlActionTerminate)
	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeApplied {
		t.Fatalf("fan-out outcome = %q, want APPLIED", result.Outcome)
	}
	if got, want := workerSessionIDsFromResults(result.Children), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fan-out children = %v, want %v", got, want)
	}
	for _, wantDispatchID := range []string{"dispatch-a", "dispatch-b"} {
		if cancellation := <-execution.cancelCalls; cancellation.DispatchID != wantDispatchID {
			t.Fatalf("boundary cancellation dispatch = %q, want %q", cancellation.DispatchID, wantDispatchID)
		}
	}
	for range 2 {
		started := <-starts
		if startErr := <-startErrs; startErr != nil ||
			started.Session.State != workersessions.StateTerminated ||
			started.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled ||
			!errors.Is(started.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
			t.Fatalf("synchronous Start() result = %#v, %v, want one terminated canceled result", started, startErr)
		}
	}
}

func TestFactoryTerminate_FansOutCapturedCancelAndRetainsOriginalEvidence(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionCancel, id: "worker-a"}: {result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied}},
		{action: factory.WorkerSessionControlActionCancel, id: "worker-b"}: {result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeNoop}},
		{action: factory.WorkerSessionControlActionCancel, id: "worker-c"}: {result: workersessions.ControlResult{DispatchID: "dispatch-c", Outcome: workersessions.ControlOutcomeFailed}, err: errors.New("boundary unavailable")},
	})
	factoryInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()), withInlineDispatch(), withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ledger.Events = append(ledger.Events,
		workerSessionAssociationEvent(t, 20, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 30, "association-c", "turn-captured", "worker-c"),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	control := factoryInstance.(factory.Service)
	terminated, err := control.ControlTerminate(ctx, factory.TerminateRequest{
		TurnID: "turn-captured", ControlID: "cancel-intent", WorkerSessionAction: factory.WorkerSessionControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlTerminate: %v", err)
	}
	if terminated.Outcome != factory.ControlOutcomeAccepted || terminated.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
		t.Fatalf("terminate result = %#v, want accepted failed fan-out evidence", terminated)
	}
	if got, want := workerSessionIDsFromResults(terminated.WorkerSessionControl.Children), []string{"worker-a", "worker-b", "worker-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("controlled Worker Session IDs = %v, want %v", got, want)
	}

	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 40, "association-late", "turn-captured", "worker-late"))
	repeated, err := control.ControlTerminate(context.Background(), factory.TerminateRequest{
		TurnID: "turn-captured", ControlID: "cancel-intent", WorkerSessionAction: factory.WorkerSessionControlActionCancel,
	})
	if err != nil {
		t.Fatalf("repeated ControlTerminate: %v", err)
	}
	if repeated.Outcome != factory.ControlOutcomeNoOp || !reflect.DeepEqual(repeated.WorkerSessionControl, terminated.WorkerSessionControl) {
		t.Fatalf("repeated terminate = %#v, want retained no-op evidence %#v", repeated, terminated.WorkerSessionControl)
	}
	if got := service.callsSnapshot(); len(got) != 3 {
		t.Fatalf("Worker Sessions calls after retry = %#v, want original three calls", got)
	}
}

func TestFactoryTerminate_DefaultsToWorkerSessionTerminate(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionTerminate, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
	})
	factoryInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()), withInlineDispatch(), withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"))

	terminated, err := factoryInstance.(factory.Service).ControlTerminate(context.Background(), factory.TerminateRequest{
		TurnID: "turn-captured", ControlID: "terminate-intent",
	})
	if err != nil {
		t.Fatalf("ControlTerminate: %v", err)
	}
	if terminated.WorkerSessionControl.Action != factory.WorkerSessionControlActionTerminate {
		t.Fatalf("Worker Sessions action = %q, want TERMINATE", terminated.WorkerSessionControl.Action)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{{action: factory.WorkerSessionControlActionTerminate, id: "worker-a"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Worker Sessions calls = %#v, want %#v", got, want)
	}
}

func TestTerminateWorkerSessionControlAction_RejectsNonStopActions(t *testing.T) {
	_, err := terminateWorkerSessionControlAction(factory.WorkerSessionControlActionPause)
	if !errors.Is(err, factory.ErrInvalidLifecycleTransition) {
		t.Fatalf("pause action error = %v, want invalid lifecycle transition", err)
	}
}

func workerSessionIDsFromResults(results []factory.WorkerSessionControlChildResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.WorkerSessionID)
	}
	return ids
}

type workerSessionControlCall struct {
	action factory.WorkerSessionControlAction
	id     string
}

type workerSessionControlResponse struct {
	result workersessions.ControlResult
	err    error
}

type workerSessionControlSpy struct {
	*fakeWorkerSessionsService
	mu              sync.Mutex
	responses       map[workerSessionControlCall]workerSessionControlResponse
	calls           []workerSessionControlCall
	canceledContext bool
}

func newWorkerSessionControlSpy(responses map[workerSessionControlCall]workerSessionControlResponse) *workerSessionControlSpy {
	return &workerSessionControlSpy{
		fakeWorkerSessionsService: &fakeWorkerSessionsService{},
		responses:                 responses,
	}
}

func (s *workerSessionControlSpy) Pause(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionPause, req)
}

func (s *workerSessionControlSpy) Resume(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionResume, req)
}

func (s *workerSessionControlSpy) Cancel(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionCancel, req)
}

func (s *workerSessionControlSpy) Terminate(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionTerminate, req)
}

func (s *workerSessionControlSpy) control(ctx context.Context, action factory.WorkerSessionControlAction, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	call := workerSessionControlCall{action: action, id: req.ID}
	s.calls = append(s.calls, call)
	if ctx.Err() != nil {
		s.canceledContext = true
	}
	response := s.responses[call]
	return response.result, response.err
}

func (s *workerSessionControlSpy) callsSnapshot() []workerSessionControlCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workerSessionControlCall(nil), s.calls...)
}

func (s *workerSessionControlSpy) observedCanceledContext() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canceledContext
}

var _ workersessions.Service = (*workerSessionControlSpy)(nil)

// synchronousFanOutExecution models two Workers-admitted dispatches behind the
// real synchronous pool boundary. Each dispatch can finish only by exact
// cancellation, so the test observes the control path without sleeps or an
// artificial completion race.
type synchronousFanOutExecution struct {
	mu                     sync.Mutex
	dispatches             map[string]*synchronousFanOutDispatch
	cancelCalls            chan workers.WorkstationDispatchCancelRequest
	canceledControlContext bool
}

type synchronousFanOutDispatch struct {
	admitted     chan struct{}
	admittedOnce sync.Once
	canceled     chan struct{}
	canceledOnce sync.Once
}

var _ workers.WorkstationExecutionService = (*synchronousFanOutExecution)(nil)

func newSynchronousFanOutExecution(dispatchIDs ...string) *synchronousFanOutExecution {
	execution := &synchronousFanOutExecution{
		dispatches:  make(map[string]*synchronousFanOutDispatch),
		cancelCalls: make(chan workers.WorkstationDispatchCancelRequest, len(dispatchIDs)),
	}
	for _, dispatchID := range dispatchIDs {
		execution.dispatches[dispatchID] = &synchronousFanOutDispatch{admitted: make(chan struct{}), canceled: make(chan struct{})}
	}
	return execution
}

func (*synchronousFanOutExecution) StartWorkstationPool(context.Context, workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStarted}, nil
}

func (*synchronousFanOutExecution) StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStopped}, nil
}

func (e *synchronousFanOutExecution) DispatchWorkstation(ctx context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
	return e.DispatchWorkstationWithAdmission(ctx, request, nil)
}

func (e *synchronousFanOutExecution) DispatchWorkstationWithAdmission(
	_ context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	e.mu.Lock()
	dispatch := e.dispatches[dispatchID]
	e.mu.Unlock()
	if dispatch == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrUnknownWorkstationDispatch
	}
	if admitted != nil {
		admitted()
	}
	dispatch.admittedOnce.Do(func() { close(dispatch.admitted) })
	<-dispatch.canceled
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeFailed,
			Error:      workers.ErrWorkstationDispatchCanceled.Error(),
		},
	}, workers.ErrWorkstationDispatchCanceled
}

func (e *synchronousFanOutExecution) CancelWorkstationDispatch(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	e.mu.Lock()
	dispatch := e.dispatches[request.DispatchID]
	if ctx.Err() != nil {
		e.canceledControlContext = true
	}
	e.mu.Unlock()
	if dispatch == nil {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrUnknownWorkstationDispatch
	}
	e.cancelCalls <- request
	dispatch.canceledOnce.Do(func() { close(dispatch.canceled) })
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

func (e *synchronousFanOutExecution) admitted(dispatchID string) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dispatches[dispatchID].admitted
}

func (e *synchronousFanOutExecution) observedCanceledControlContext() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.canceledControlContext
}

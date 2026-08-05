package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionswire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
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

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

type operationsFactory struct {
	lifecycleObserverFactory

	submitResult work.WorkRequestSubmitResult
	submitErr    error
	moveResult   work.OperatorMoveResult
	moveErr      error
	subscribeErr error
	snapshotErr  error
	waitCh       chan struct{}

	wantScopeSessionID string
	wantReconnectID    string

	submitCalls    int
	moveCalls      int
	subscribeCalls int
	snapshotCalls  int
}

func (f *operationsFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	f.submitCalls++
	if f.submitErr != nil {
		return work.WorkRequestSubmitResult{}, f.submitErr
	}
	if f.submitResult.Works == nil && len(request.Works) > 0 {
		return work.WorkRequestSubmitResult{
			Works: []work.WorkRequestSubmittedWork{{
				WorkID: request.Works[0].WorkID,
			}},
		}, nil
	}
	return f.submitResult, nil
}

func (f *operationsFactory) MoveWork(_ context.Context, workID, stateName string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	f.moveCalls++
	if f.moveErr != nil {
		return work.OperatorMoveResult{}, f.moveErr
	}
	if f.moveResult.WorkID == "" {
		return work.OperatorMoveResult{WorkID: workID, ToState: stateName}, nil
	}
	return f.moveResult, nil
}

func (f *operationsFactory) SubscribeFactoryEvents(_ context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	f.subscribeCalls++
	if f.subscribeErr != nil {
		return nil, f.subscribeErr
	}
	if f.wantScopeSessionID != "" && scope.SessionID != f.wantScopeSessionID {
		return nil, errors.New("unexpected subscribe scope session")
	}
	if f.wantReconnectID != "" {
		if reconnect == nil || reconnect.AfterEventID != f.wantReconnectID {
			return nil, errors.New("unexpected subscribe reconnect cursor")
		}
	}
	return &interfaces.FactoryEventStream{
		Events:             make(chan interfaces.FactoryEvent),
		StreamGenerationID: "gen-1",
	}, nil
}

func (f *operationsFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.snapshotCalls++
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	}, nil
}

func (f *operationsFactory) WaitToComplete() <-chan struct{} {
	if f.waitCh != nil {
		return f.waitCh
	}
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestRuntimeOperations_ReportUnavailableWithoutBundle(t *testing.T) {
	t.Parallel()

	if _, err := factoryservice.SubmitWorkRequest(context.Background(), nil, work.WorkRequest{}); !errors.Is(err, factoryservice.ErrRuntimeNotAvailable) {
		t.Fatalf("SubmitWorkRequest error = %v, want ErrRuntimeNotAvailable", err)
	}
	if _, err := factoryservice.MoveWork(context.Background(), nil, "work-1", "done", work.WorkStateChangeSourceAPI, "req-1"); !errors.Is(err, factoryservice.ErrRuntimeNotAvailable) {
		t.Fatalf("MoveWork error = %v, want ErrRuntimeNotAvailable", err)
	}
	if _, err := factoryservice.SubscribeFactoryEvents(context.Background(), nil, nil, interfaces.FactoryEventReconnectScope{}); !errors.Is(err, factoryservice.ErrRuntimeNotAvailable) {
		t.Fatalf("SubscribeFactoryEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
	if _, err := factoryservice.GetEngineStateSnapshot(context.Background(), nil); !errors.Is(err, factoryservice.ErrRuntimeNotAvailable) {
		t.Fatalf("GetEngineStateSnapshot error = %v, want ErrRuntimeNotAvailable", err)
	}

	select {
	case <-factoryservice.WaitToComplete(nil):
	default:
		t.Fatal("WaitToComplete without bundle should return a closed channel")
	}
}

func newOperationsTestBundle() (*operationsFactory, *factoryservice.Bundle, chan struct{}) {
	factoryStub := &operationsFactory{
		wantScopeSessionID: "session-alpha",
		wantReconnectID:    "evt-1",
	}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})
	waitCh := make(chan struct{})
	factoryStub.waitCh = waitCh
	return factoryStub, &factoryservice.Bundle{Factory: factoryStub}, waitCh
}

func assertSubmitDelegated(t *testing.T, bundle *factoryservice.Bundle) {
	t.Helper()
	submitResult, err := factoryservice.SubmitWorkRequest(context.Background(), bundle, work.WorkRequest{
		Works: []work.Work{{WorkID: "work-1"}},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if len(submitResult.Works) != 1 || submitResult.Works[0].WorkID != "work-1" {
		t.Fatalf("submit result = %#v, want work-1", submitResult)
	}
}

func assertMoveDelegated(t *testing.T, bundle *factoryservice.Bundle) {
	t.Helper()
	moveResult, err := factoryservice.MoveWork(context.Background(), bundle, "work-1", "done", work.WorkStateChangeSourceAPI, "req-1")
	if err != nil {
		t.Fatalf("MoveWork: %v", err)
	}
	if moveResult.WorkID != "work-1" || moveResult.ToState != "done" {
		t.Fatalf("move result = %#v, want work-1 done", moveResult)
	}
}

func assertSubscribeDelegated(t *testing.T, bundle *factoryservice.Bundle) {
	t.Helper()
	stream, err := factoryservice.SubscribeFactoryEventsForSession(
		context.Background(),
		bundle,
		"session-alpha",
		&interfaces.FactoryEventReconnectCursor{AfterEventID: "evt-1"},
	)
	if err != nil {
		t.Fatalf("SubscribeFactoryEventsForSession: %v", err)
	}
	if stream == nil || stream.StreamGenerationID != "gen-1" {
		t.Fatalf("subscribe stream = %#v, want generation gen-1", stream)
	}
}

func assertSnapshotDelegated(t *testing.T, bundle *factoryservice.Bundle) {
	t.Helper()
	snapshot, err := factoryservice.GetEngineStateSnapshot(context.Background(), bundle)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusActive {
		t.Fatalf("snapshot runtime status = %q, want active", snapshot.RuntimeStatus)
	}
}

func assertWaitDelegated(t *testing.T, bundle *factoryservice.Bundle, waitCh chan struct{}) {
	t.Helper()
	if got := factoryservice.WaitToComplete(bundle); got != waitCh {
		t.Fatalf("WaitToComplete channel = %p, want %p", got, waitCh)
	}
}

func assertSingleDelegateCalls(t *testing.T, factoryStub *operationsFactory) {
	t.Helper()
	if factoryStub.submitCalls != 1 || factoryStub.moveCalls != 1 || factoryStub.subscribeCalls != 1 || factoryStub.snapshotCalls != 1 {
		t.Fatalf("factory calls = submit:%d move:%d subscribe:%d snapshot:%d, want 1 each",
			factoryStub.submitCalls, factoryStub.moveCalls, factoryStub.subscribeCalls, factoryStub.snapshotCalls)
	}
}

func TestRuntimeOperations_DelegateToHostedFactoryWithoutRootService(t *testing.T) {
	t.Parallel()

	factoryStub, bundle, waitCh := newOperationsTestBundle()
	assertSubmitDelegated(t, bundle)
	assertMoveDelegated(t, bundle)
	assertSubscribeDelegated(t, bundle)
	assertSnapshotDelegated(t, bundle)
	assertWaitDelegated(t, bundle, waitCh)
	assertSingleDelegateCalls(t, factoryStub)
}

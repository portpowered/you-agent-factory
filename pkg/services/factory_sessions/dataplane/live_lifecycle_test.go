package dataplane_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/dataplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
)

type lifecycleTestHost struct {
	factory         factory.Factory
	factoryErr      error
	stopErr         error
	stoppedSessions []string
	observations    []string
}

func (h *lifecycleTestHost) SessionFactory(_ string) (factory.Service, error) {
	if h.factoryErr != nil {
		return nil, h.factoryErr
	}
	return h.factory, nil
}

func (h *lifecycleTestHost) StopLiveSession(sessionID string) error {
	if h.stopErr != nil {
		return h.stopErr
	}
	h.stoppedSessions = append(h.stoppedSessions, sessionID)
	return nil
}

func (h *lifecycleTestHost) ObserveLiveLifecycleControl(
	_ string,
	operation factorysessionexecution.LifecycleControlKind,
	_ factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	_ factorysessionexecution.LifecycleStatus,
	err error,
) {
	if err != nil {
		h.observations = append(h.observations, string(operation)+":error")
		return
	}
	h.observations = append(h.observations, string(operation)+":"+string(outcome))
}

type lifecycleTestFactory struct {
	factoryState string
	pauseErr     error
	resumeErr    error
	pauseCalls   int
	resumeCalls  int
}

func (f *lifecycleTestFactory) Run(context.Context) error { return nil }

func (f *lifecycleTestFactory) Pause(context.Context) error {
	f.pauseCalls++
	return f.pauseErr
}

func (f *lifecycleTestFactory) Resume(context.Context) error {
	f.resumeCalls++
	return f.resumeErr
}

func (f *lifecycleTestFactory) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}

func (f *lifecycleTestFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (f *lifecycleTestFactory) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}

func (f *lifecycleTestFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

func (f *lifecycleTestFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net], error) {
	return &interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net]{
		FactoryState: f.factoryState,
	}, nil
}

func (f *lifecycleTestFactory) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func TestLiveLifecycle_ApplyControl_AcceptsRunningPause(t *testing.T) {
	t.Parallel()

	testFactory := &lifecycleTestFactory{factoryState: string(interfaces.FactoryStateRunning)}
	host := &lifecycleTestHost{factory: testFactory}
	controller := dataplane.NewLiveLifecycle(host)

	result, err := controller.ApplyControl(
		context.Background(),
		"sess-1",
		factorysessionexecution.LifecycleControlPause,
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("ApplyControl: %v", err)
	}
	if result.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", result.Status)
	}
	wantLinks := factorysessionexecution.LifecycleControlLinks{
		Session: "/factory-sessions/sess-1",
		Status:  "/factory-sessions/sess-1",
		Results: "/factory-sessions/sess-1/result",
		Events:  "/factory-sessions/sess-1/events",
	}
	if result.Links != wantLinks {
		t.Fatalf("links = %#v, want %#v", result.Links, wantLinks)
	}
	if testFactory.pauseCalls != 1 {
		t.Fatalf("pause calls = %d, want 1", testFactory.pauseCalls)
	}
}

func TestLiveLifecycle_ApplyControl_ReturnsNotFoundForMissingSession(t *testing.T) {
	t.Parallel()

	host := &lifecycleTestHost{
		factoryErr: fmt.Errorf("%w: missing", apisurface.ErrFactorySessionNotFound),
	}
	controller := dataplane.NewLiveLifecycle(host)

	_, err := controller.ApplyControl(
		context.Background(),
		"missing",
		factorysessionexecution.LifecycleControlPause,
		factorysessionexecution.ControlRequest{},
	)
	if err == nil || !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("ApplyControl error = %v, want not found", err)
	}
	if len(host.observations) != 1 || host.observations[0] != "PAUSE:error" {
		t.Fatalf("observations = %#v, want PAUSE:error", host.observations)
	}
}

func TestLiveLifecycle_CloseSession_StopsLiveSession(t *testing.T) {
	t.Parallel()

	host := &lifecycleTestHost{}
	controller := dataplane.NewLiveLifecycle(host)

	if err := controller.CloseSession("sess-1"); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if len(host.stoppedSessions) != 1 || host.stoppedSessions[0] != "sess-1" {
		t.Fatalf("stopped sessions = %#v, want sess-1", host.stoppedSessions)
	}
}

func TestLiveLifecycle_ApplyControl_AcceptsPausedResume(t *testing.T) {
	t.Parallel()

	testFactory := &lifecycleTestFactory{factoryState: string(interfaces.FactoryStatePaused)}
	host := &lifecycleTestHost{factory: testFactory}
	controller := dataplane.NewLiveLifecycle(host)

	result, err := controller.ApplyControl(
		context.Background(),
		"sess-1",
		factorysessionexecution.LifecycleControlResume,
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("ApplyControl: %v", err)
	}
	if result.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", result.Status)
	}
	if testFactory.resumeCalls != 1 {
		t.Fatalf("resume calls = %d, want 1", testFactory.resumeCalls)
	}
}

func TestLiveLifecycle_CloseSession_PropagatesStopError(t *testing.T) {
	t.Parallel()

	host := &lifecycleTestHost{stopErr: fmt.Errorf("stop failed")}
	controller := dataplane.NewLiveLifecycle(host)

	if err := controller.CloseSession("sess-1"); err == nil {
		t.Fatal("CloseSession = nil, want stop error")
	}
}

func TestLiveLifecycle_ApplyControl_ReturnsErrorWhenSnapshotUnavailable(t *testing.T) {
	t.Parallel()

	testFactory := &lifecycleTestFactory{}
	testFactory.factoryState = ""
	host := &lifecycleTestHost{
		factory: &snapshotErrorFactory{inner: testFactory},
	}
	controller := dataplane.NewLiveLifecycle(host)

	_, err := controller.ApplyControl(
		context.Background(),
		"sess-1",
		factorysessionexecution.LifecycleControlPause,
		factorysessionexecution.ControlRequest{},
	)
	if err == nil {
		t.Fatal("ApplyControl = nil, want snapshot error")
	}
}

type snapshotErrorFactory struct {
	inner *lifecycleTestFactory
}

func (f *snapshotErrorFactory) Run(context.Context) error { return f.inner.Run(context.Background()) }
func (f *snapshotErrorFactory) Pause(context.Context) error {
	return f.inner.Pause(context.Background())
}
func (f *snapshotErrorFactory) Resume(context.Context) error {
	return f.inner.Resume(context.Background())
}
func (f *snapshotErrorFactory) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return f.inner.GetFactoryEvents(context.Background())
}
func (f *snapshotErrorFactory) WaitToComplete() <-chan struct{} { return f.inner.WaitToComplete() }
func (f *snapshotErrorFactory) SubmitWorkRequest(ctx context.Context, req work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return f.inner.SubmitWorkRequest(ctx, req)
}
func (f *snapshotErrorFactory) SubscribeFactoryEvents(ctx context.Context, cursor *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return f.inner.SubscribeFactoryEvents(ctx, cursor, scope)
}
func (f *snapshotErrorFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net], error) {
	return nil, fmt.Errorf("snapshot unavailable")
}
func (f *snapshotErrorFactory) MoveWork(ctx context.Context, a, b string, source work.WorkStateChangeSource, reason string) (work.OperatorMoveResult, error) {
	return f.inner.MoveWork(ctx, a, b, source, reason)
}

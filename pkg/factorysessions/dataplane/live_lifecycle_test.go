package dataplane_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions/dataplane"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type lifecycleTestHost struct {
	factory         factory.Factory
	factoryErr      error
	stopErr         error
	stoppedSessions []string
	observations    []string
}

func (h *lifecycleTestHost) SessionFactory(_ string) (factory.Factory, error) {
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

func (f *lifecycleTestFactory) GetFactoryEvents(context.Context) ([]factoryapi.FactoryEvent, error) {
	return nil, nil
}

func (f *lifecycleTestFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (f *lifecycleTestFactory) SubmitWorkRequest(context.Context, interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	return interfaces.WorkRequestSubmitResult{}, nil
}

func (f *lifecycleTestFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

func (f *lifecycleTestFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		FactoryState: f.factoryState,
	}, nil
}

func (f *lifecycleTestFactory) MoveWork(context.Context, string, string, interfaces.WorkStateChangeSource, string) (interfaces.OperatorMoveResult, error) {
	return interfaces.OperatorMoveResult{}, nil
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

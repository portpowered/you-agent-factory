package testutil_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	petri "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestMockFactory_LiveSessionPauseResume_ReturnsTypedControlForExistingSession(t *testing.T) {
	t.Parallel()

	resumeCalls := 0
	session := &testutil.MockFactory{}
	session.PauseLiveFactorySessionFunc = func(
		context.Context,
		string,
		factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error) {
		return factorysessions.LifecycleControlResult{
			SessionID: "live-sess-001",
			Operation: factorysessions.LifecycleControlPause,
			Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
			Status:    factorysessions.LifecycleStatusPaused,
		}, nil
	}
	session.ResumeLiveFactorySessionFunc = func(
		context.Context,
		string,
		factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error) {
		resumeCalls++
		outcome := factorysessions.LifecycleControlOutcomeAccepted
		if resumeCalls > 1 {
			outcome = factorysessions.LifecycleControlOutcomeNoOp
		}
		return factorysessions.LifecycleControlResult{
			SessionID: "live-sess-001",
			Operation: factorysessions.LifecycleControlResume,
			Outcome:   outcome,
			Status:    factorysessions.LifecycleStatusRunning,
		}, nil
	}
	mock := &testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"live-sess-001": session,
		},
	}

	pause, err := mock.PauseLiveFactorySession(context.Background(), "live-sess-001", factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("PauseLiveFactorySession() error = %v", err)
	}
	if pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		pause.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("pause response = %#v, want ACCEPTED/PAUSED", pause)
	}

	resume, err := mock.ResumeLiveFactorySession(context.Background(), "live-sess-001", factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession() error = %v", err)
	}
	if resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		resume.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume response = %#v, want ACCEPTED/RUNNING", resume)
	}

	noOp, err := mock.ResumeLiveFactorySession(context.Background(), "live-sess-001", factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession() no-op error = %v", err)
	}
	if noOp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp ||
		noOp.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume no-op response = %#v, want NO_OP/RUNNING", noOp)
	}
}

func TestMockFactory_LiveSessionPauseResume_ReturnsNotFoundForMissingSession(t *testing.T) {
	t.Parallel()

	mock := &testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{}}
	_, pauseErr := mock.PauseLiveFactorySession(t.Context(), "missing-session", factorysessions.ControlRequest{})
	if !errors.Is(pauseErr, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("PauseLiveFactorySession() error = %v, want %v", pauseErr, apisurface.ErrFactorySessionNotFound)
	}
	_, resumeErr := mock.ResumeLiveFactorySession(t.Context(), "missing-session", factorysessions.ControlRequest{})
	if !errors.Is(resumeErr, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("ResumeLiveFactorySession() error = %v, want %v", resumeErr, apisurface.ErrFactorySessionNotFound)
	}
}

func TestMockFactory_LiveSessionPauseResume_ReturnsControlErrorForTerminalSession(t *testing.T) {
	t.Parallel()

	mock := &testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"live-sess-001": {
				PauseLiveFactorySessionFunc: func(
					context.Context,
					string,
					factorysessions.ControlRequest,
				) (factorysessions.LifecycleControlResult, error) {
					return factorysessions.LifecycleControlResult{}, &factorysessions.ControlError{
						Operation: factorysessions.LifecycleControlPause,
						Outcome:   factorysessions.LifecycleControlOutcomeTerminalSession,
						Status:    factorysessions.LifecycleStatusSucceeded,
						Message:   "terminal session",
					}
				},
			},
		},
	}
	_, err := mock.PauseLiveFactorySession(t.Context(), "live-sess-001", factorysessions.ControlRequest{})
	var controlErr *factorysessions.ControlError
	if !errors.As(err, &controlErr) {
		t.Fatalf("PauseLiveFactorySession() error = %T(%v), want *ControlError", err, err)
	}
	if controlErr.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("control outcome = %s, want TERMINAL_SESSION", controlErr.Outcome)
	}
}

func TestMockFactory_GetEngineStateSnapshot_ReturnsConfiguredEngineStateAndCountsCall(t *testing.T) {
	expected := &interfaces.EngineStateSnapshot[petri.PetriMarkingSnapshot, *petri.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		InFlightCount: 2,
	}
	mf := &testutil.MockFactory{EngineState: expected}

	got, err := mf.GetEngineStateSnapshot(t.Context())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if got != expected {
		t.Fatalf("GetEngineStateSnapshot() = %#v, want configured snapshot %#v", got, expected)
	}
	if mf.EngineStateSnapshotCalls != 1 {
		t.Fatalf("EngineStateSnapshotCalls = %d, want 1", mf.EngineStateSnapshotCalls)
	}
}

func TestMockFactory_GetEngineStateSnapshot_BuildsAggregateSnapshotFromConfiguredFields(t *testing.T) {
	net := &petri.Net{ID: "test-net"}
	marking := &petri.PetriMarkingSnapshot{
		Tokens: map[string]*petri.RuntimeToken{
			"tok-1": {ID: "tok-1", PlaceID: "task:init"},
		},
		PlaceTokens: map[string][]string{"task:init": {"tok-1"}},
	}
	mf := &testutil.MockFactory{
		State:   interfaces.FactoryStateRunning,
		Marking: marking,
		Net:     net,
		Uptime:  5 * time.Second,
	}

	snapshot, err := mf.GetEngineStateSnapshot(t.Context())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("GetEngineStateSnapshot() = nil, want snapshot")
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("FactoryState = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateRunning)
	}
	if snapshot.Uptime != 5*time.Second {
		t.Fatalf("Uptime = %v, want %v", snapshot.Uptime, 5*time.Second)
	}
	if snapshot.Topology != net {
		t.Fatal("Topology did not use configured net")
	}
	if snapshot.Marking.Tokens["tok-1"] == nil {
		t.Fatalf("Marking did not include configured token: %#v", snapshot.Marking.Tokens)
	}
}

func TestMockFactory_GetEngineStateSnapshot_ReturnsConfiguredError(t *testing.T) {
	wantErr := assertErr{}
	mf := &testutil.MockFactory{EngineStateSnapshotErr: wantErr}

	_, err := mf.GetEngineStateSnapshot(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetEngineStateSnapshot error = %v, want %v", err, wantErr)
	}
	if mf.EngineStateSnapshotCalls != 1 {
		t.Fatalf("EngineStateSnapshotCalls = %d, want 1", mf.EngineStateSnapshotCalls)
	}
}

func TestMockFactory_GetFactoryEvents_ReturnsCopy(t *testing.T) {
	mf := &testutil.MockFactory{
		FactoryEvents: []factoryapi.FactoryEvent{
			{
				Id:            "event-1",
				SchemaVersion: factoryapi.AgentFactoryEventV1,
				Type:          factoryapi.FactoryEventTypeRunRequest,
			},
		},
	}

	events, err := mf.GetFactoryEvents(t.Context())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	if len(events) != 1 || events[0].Id != "event-1" {
		t.Fatalf("GetFactoryEvents() = %#v, want configured event", events)
	}

	events[0].Id = "mutated"
	again, err := mf.GetFactoryEvents(t.Context())
	if err != nil {
		t.Fatalf("GetFactoryEvents second call: %v", err)
	}
	if again[0].Id != "event-1" {
		t.Fatalf("GetFactoryEvents returned mutable backing slice, got id %q", again[0].Id)
	}
}

func TestMockFactory_SubscribeFactoryEvents_ReturnsHistoryAndCapturesContext(t *testing.T) {
	mf := &testutil.MockFactory{
		FactoryEvents: []factoryapi.FactoryEvent{
			{
				Id:            "event-1",
				SchemaVersion: factoryapi.AgentFactoryEventV1,
				Type:          factoryapi.FactoryEventTypeRunRequest,
			},
		},
	}

	stream, err := mf.SubscribeFactoryEvents(t.Context(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if stream == nil {
		t.Fatal("SubscribeFactoryEvents() = nil, want stream")
	}
	if len(stream.History) != 1 || stream.History[0].Id != "event-1" {
		t.Fatalf("stream history = %#v, want configured event", stream.History)
	}
	if mf.FactoryEventStreamCtx == nil {
		t.Fatal("FactoryEventStreamCtx was not captured")
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "assert error" }

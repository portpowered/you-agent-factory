package events

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings/projections"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventHistory_RecordSessionLifecycle_EmitsReconstructableBracketSequence(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 10, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-alpha", &interfaces.FactoryConfig{
		Name: "factory-alpha",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				Dialect:    "workflow-v1",
				SourceRef:  "workflow/main.js",
				SourceHash: "sha256:source",
			},
		},
	}, 0, t0)
	history.RecordSessionLifecycleCompletion(
		"session-alpha",
		&interfaces.FactoryConfig{
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
			},
		},
		2,
		interfaces.FactoryStateCompleted,
		"",
		t0.Add(2*time.Second),
	)

	events := generatedHistoryEvents(t, history)
	if len(events) != 3 {
		t.Fatalf("events = %d, want started, result-updated, completed", len(events))
	}
	assertSessionLifecycleEventType(t, events[0], factoryapi.FactoryEventTypeSessionStarted, "factory-event/session-started")
	assertSessionLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeSessionResultUpdated, "factory-event/session-result-updated/1")
	assertSessionLifecycleEventType(t, events[2], factoryapi.FactoryEventTypeSessionCompleted, "factory-event/session-completed")

	for i, event := range events {
		if event.Context.SessionId == nil || *event.Context.SessionId != "session-alpha" {
			t.Fatalf("events[%d].context.sessionId = %#v, want session-alpha", i, event.Context.SessionId)
		}
		if event.Context.SessionSequence == nil || *event.Context.SessionSequence != i {
			t.Fatalf("events[%d].context.sessionSequence = %#v, want %d", i, event.Context.SessionSequence, i)
		}
	}

	worldState, err := projections.ReconstructCanonicalFactoryWorldState(history.CanonicalEvents(), 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || !worldState.SessionBracket.Terminal {
		t.Fatalf("session bracket = %#v, want terminal reconstructed lifecycle", worldState.SessionBracket)
	}
}

func TestFactoryEventHistory_AddEventTypeRecorder_ReplaysHistoryThenObservesCompletion(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 10, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-alpha", &interfaces.FactoryConfig{
		Name: "factory-alpha",
	}, 0, t0)

	var eventTypes []interfaces.FactoryEventType
	history.AddEventTypeRecorder(func(eventType interfaces.FactoryEventType) {
		eventTypes = append(eventTypes, eventType)
	})
	history.RecordSessionLifecycleCompletion(
		"session-alpha",
		&interfaces.FactoryConfig{},
		1,
		interfaces.FactoryStateCompleted,
		"",
		t0.Add(time.Second),
	)

	want := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionCompleted,
	}
	if len(eventTypes) != len(want) {
		t.Fatalf("event types = %v, want %v", eventTypes, want)
	}
	for index := range want {
		if eventTypes[index] != want[index] {
			t.Fatalf("event types[%d] = %q, want %q", index, eventTypes[index], want[index])
		}
	}
}

func TestFactoryEventHistory_RecordSessionLifecycle_FailedRunEmitsFailedWithPartialResult(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 11, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-beta", &interfaces.FactoryConfig{
		Name: "factory-beta",
	}, 0, t0)
	history.RecordSessionLifecycleCompletion(
		"session-beta",
		&interfaces.FactoryConfig{},
		1,
		interfaces.FactoryStateFailed,
		"dispatch failed",
		t0.Add(time.Second),
	)

	events := generatedHistoryEvents(t, history)
	if len(events) != 3 {
		t.Fatalf("events = %d, want started, failed-with-partial result, completed", len(events))
	}
	resultPayload, err := events[1].Payload.AsSessionResultUpdatedEventPayload()
	if err != nil {
		t.Fatalf("result updated payload: %v", err)
	}
	if resultPayload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFailedWithPartial {
		t.Fatalf("result status = %q, want FAILED_WITH_PARTIAL", resultPayload.ResultStatus)
	}
	completedPayload, err := events[2].Payload.AsSessionCompletedEventPayload()
	if err != nil {
		t.Fatalf("completed payload: %v", err)
	}
	if completedPayload.FailureDetail == nil || completedPayload.FailureDetail.Message != "dispatch failed" {
		t.Fatalf("completed failure detail = %#v, want dispatch failed message", completedPayload.FailureDetail)
	}
}

func TestFactoryEventHistory_RecordSessionLifecycleControl_EmitsPauseAndResume(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleControl(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Source:           "runtime",
		Tick:             3,
		Operation:        interfaces.FactorySessionLifecycleControlPause,
		Outcome:          interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		PreviousStatus:   interfaces.FactorySessionLifecycleStatusRunning,
		NewStatus:        interfaces.FactorySessionLifecycleStatusPaused,
		Reason:           "pause requested",
	}, t0)
	history.RecordSessionLifecycleControl(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Source:           "runtime",
		Tick:             4,
		Operation:        interfaces.FactorySessionLifecycleControlResume,
		Outcome:          interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		PreviousStatus:   interfaces.FactorySessionLifecycleStatusPaused,
		NewStatus:        interfaces.FactorySessionLifecycleStatusRunning,
		Reason:           "resume requested",
	}, t0.Add(time.Second))

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("events = %d, want pause and resume lifecycle controls", len(events))
	}
	assertSessionLifecycleEventType(t, events[0], factoryapi.FactoryEventTypeSessionLifecycleControl, "session-lifecycle-control/session-live/0")
	assertSessionLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeSessionLifecycleControl, "session-lifecycle-control/session-live/1")

	pausePayload, err := events[0].Payload.AsSessionLifecycleControlEventPayload()
	if err != nil {
		t.Fatalf("pause payload: %v", err)
	}
	if pausePayload.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pausePayload.PreviousStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning ||
		pausePayload.NewStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("pause payload = %#v, want RUNNING->PAUSED", pausePayload)
	}
}

func TestFactoryStateToDurableLifecycleStatus_MapsLiveFactoryStates(t *testing.T) {
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStatePaused); got != interfaces.FactorySessionLifecycleStatusPaused {
		t.Fatalf("paused = %q, want PAUSED", got)
	}
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStateCompleted); got != interfaces.FactorySessionLifecycleStatusSucceeded {
		t.Fatalf("completed = %q, want SUCCEEDED", got)
	}
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStateFailed); got != interfaces.FactorySessionLifecycleStatusFailed {
		t.Fatalf("failed = %q, want FAILED", got)
	}
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStateRunning); got != interfaces.FactorySessionLifecycleStatusRunning {
		t.Fatalf("running = %q, want RUNNING", got)
	}
}

func assertSessionLifecycleEventType(
	t *testing.T,
	event factoryapi.FactoryEvent,
	wantType factoryapi.FactoryEventType,
	wantID string,
) {
	t.Helper()
	if event.Type != wantType {
		t.Fatalf("event type = %q, want %q", event.Type, wantType)
	}
	if event.Id != wantID {
		t.Fatalf("event id = %q, want %q", event.Id, wantID)
	}
}

func TestFactoryEventHistory_RecordSessionPauseResume_EmitsReconstructableControlStatus(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-live", &interfaces.FactoryConfig{
		Name: "factory-live",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindPetri,
		},
	}, 0, t0)
	history.RecordSessionPaused(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Source:           "runtime",
		Tick:             1,
	}, t0.Add(time.Second))
	history.RecordSessionResumed(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Source:           "runtime",
		Tick:             2,
	}, t0.Add(2*time.Second))

	events := generatedHistoryEvents(t, history)
	if len(events) != 3 {
		t.Fatalf("events = %d, want started, paused, resumed", len(events))
	}
	assertSessionLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeSessionPaused, "factory-event/session-paused/1")
	assertSessionLifecycleEventType(t, events[2], factoryapi.FactoryEventTypeSessionResumed, "factory-event/session-resumed/2")

	worldState, err := projections.ReconstructCanonicalFactoryWorldState(history.CanonicalEvents(), 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil {
		t.Fatal("session bracket = nil, want pause/resume lifecycle")
	}
	if worldState.SessionBracket.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusRunning) {
		t.Fatalf("lifecycle control status = %q, want RUNNING", worldState.SessionBracket.LifecycleControlStatus)
	}
	if worldState.SessionBracket.PausedAt.IsZero() || worldState.SessionBracket.ResumedAt.IsZero() {
		t.Fatalf("paused/resumed timestamps = %#v, want both set", worldState.SessionBracket)
	}
}

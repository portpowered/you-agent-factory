package events

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryEventHistory_RecordSessionLifecycle_EmitsReconstructableBracketSequence(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 10, 0, 0, time.UTC)
	history := NewFactoryEventHistory(nil, func() time.Time { return t0 })
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

	events := history.Events()
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

	worldState, err := projections.ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || !worldState.SessionBracket.Terminal {
		t.Fatalf("session bracket = %#v, want terminal reconstructed lifecycle", worldState.SessionBracket)
	}
}

func TestFactoryEventHistory_RecordSessionLifecycle_FailedRunEmitsFailedWithPartialResult(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 11, 0, 0, time.UTC)
	history := NewFactoryEventHistory(nil, func() time.Time { return t0 })
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

	events := history.Events()
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
	if completedPayload.FailureDetail == nil || completedPayload.FailureDetail.Message == nil || *completedPayload.FailureDetail.Message != "dispatch failed" {
		t.Fatalf("completed failure detail = %#v, want dispatch failed message", completedPayload.FailureDetail)
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

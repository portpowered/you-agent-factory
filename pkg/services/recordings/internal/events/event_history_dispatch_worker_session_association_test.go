package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFactoryEventHistory_WorkerSessionWorkFactsSurviveSeedAndFreshAppend(t *testing.T) {
	t.Parallel()
	when := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return when })
	record := interfaces.FactoryDispatchRecord{DispatchID: "dispatch-selected", Dispatch: work.WorkDispatch{
		DispatchID: "dispatch-selected", TransitionID: "station",
		InputTokens: workers.InputTokens(workers.Token{ID: "token", Color: workers.Color{WorkID: "work-selected"}}),
	}}
	history.RecordWorkstationRequest(1, record, when)
	history.RecordDispatchWorkerSessionAssociationWithExecution(1, "dispatch-selected", "worker-selected", "turn-selected", recordings.DispatchWorkerSessionExecutionFacts{Model: "model-one"}, when)
	prefix := history.CanonicalEvents()
	seeded := newTestFactoryEventHistory(nil, func() time.Time { return when })
	if err := seeded.SeedCanonicalEvents(prefix); err != nil {
		t.Fatal(err)
	}
	before := seeded.CanonicalHistoryReadStats()
	facts, err := seeded.CurrentWorkerSessionWorkFacts(t.Context(), "work-selected")
	if err != nil || !facts.KnownWork || len(facts.Associations) != 1 || facts.Associations["dispatch-selected"].WorkerSessionID != "worker-selected" {
		t.Fatalf("seeded selected facts = %+v, %v", facts, err)
	}
	if facts.StreamGenerationID != seeded.StreamGenerationID() || facts.StateCursors["dispatch-selected"].StreamGenerationID != facts.StreamGenerationID {
		t.Fatal("selected facts lost the generation fence")
	}
	if seeded.CanonicalHistoryReadStats() != before {
		t.Fatal("selected projection read canonical history")
	}
	*facts.Associations["dispatch-selected"].Model = "poisoned"
	again, err := seeded.CurrentWorkerSessionWorkFacts(t.Context(), "work-selected")
	if err != nil || *again.Associations["dispatch-selected"].Model != "model-one" {
		t.Fatalf("seeded facts were not detached: %+v, %v", again, err)
	}
	record.DispatchID, record.Dispatch.DispatchID = "dispatch-next", "dispatch-next"
	seeded.RecordWorkstationRequest(2, record, when.Add(time.Second))
	seeded.RecordDispatchWorkerSessionAssociation(2, "dispatch-next", "worker-next", "turn-next", when.Add(time.Second))
	fresh, err := seeded.CurrentWorkerSessionWorkFacts(t.Context(), "work-selected")
	if err != nil || len(fresh.Associations) != 2 || fresh.Associations["dispatch-next"].WorkerSessionID != "worker-next" {
		t.Fatalf("new append did not update selected facts: %+v, %v", fresh, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := seeded.CurrentWorkerSessionWorkFacts(ctx, "work-selected"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled selected read = %v", err)
	}
	if _, err := seeded.CurrentWorkerSessionWorkFacts(t.Context(), "work-selected"); err != nil {
		t.Fatalf("cancellation poisoned next read: %v", err)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_RecordsCanonicalAssociation(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "worker-session-1", "turn-1", eventTime)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("subscribe canonical events: %v", err)
	}
	if len(stream.History) != 1 {
		t.Fatalf("canonical history count = %d, want 1", len(stream.History))
	}

	event := stream.History[0]
	if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc {
		t.Fatalf("event type = %s, want %s", event.Type, interfaces.FactoryEventTypeDispatchWorkerSessionAssoc)
	}
	if event.Context.DispatchID == nil || *event.Context.DispatchID != "dispatch-1" {
		t.Fatalf("context dispatchId = %v, want dispatch-1", event.Context.DispatchID)
	}
	if event.Context.RequestID == nil || *event.Context.RequestID != "turn-1" {
		t.Fatalf("context requestId = %v, want turn-1", event.Context.RequestID)
	}
	if !event.Context.EventTime.Equal(eventTime) {
		t.Fatalf("context eventTime = %s, want %s", event.Context.EventTime, eventTime)
	}

	var payload interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode dispatch worker session association payload: %v", err)
	}
	if payload.WorkerSessionID != "worker-session-1" {
		t.Fatalf("payload workerSessionId = %q, want worker-session-1", payload.WorkerSessionID)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociationWithExecution_RetainsReplayFactsWithoutWideningPublicPayload(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociationWithExecution(
		4,
		"dispatch-model",
		"worker-session-model",
		"turn-model",
		recordings.DispatchWorkerSessionExecutionFacts{Model: "gpt-5.6-luna", ReasoningEffort: "high"},
		eventTime,
	)

	event := history.CanonicalEvents()[0]
	var replayPayload struct {
		WorkerSessionID string `json:"workerSessionId"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
	}
	if err := json.Unmarshal(event.Payload, &replayPayload); err != nil {
		t.Fatalf("decode replay association payload: %v", err)
	}
	if replayPayload.WorkerSessionID != "worker-session-model" || replayPayload.Model != "gpt-5.6-luna" || replayPayload.ReasoningEffort != "high" {
		t.Fatalf("replay association payload = %#v, want worker session and execution facts", replayPayload)
	}

	var publicPayload interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := event.DecodePayload(&publicPayload); err != nil {
		t.Fatalf("decode public association payload: %v", err)
	}
	if publicPayload.WorkerSessionID != "worker-session-model" {
		t.Fatalf("public association payload workerSessionId = %q, want worker-session-model", publicPayload.WorkerSessionID)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_IgnoresIncompleteIdentities(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociation(4, "", "worker-session-1", "turn-1", eventTime)
	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "", "turn-1", eventTime)

	if got := len(history.CanonicalEvents()); got != 0 {
		t.Fatalf("canonical event count = %d, want 0 for incomplete association identities", got)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_NilHistoryDoesNotPanic(t *testing.T) {
	var history *FactoryEventHistory
	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "worker-session-1", "turn-1", time.Now())
}

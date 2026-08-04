package canonicalledger_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestReconstructedCanonicalLedgerRetainsDispatchWorkerSessionAssociationPairsAndPublicRoundTrip(t *testing.T) {
	t.Parallel()

	const generationID = "dispatch-worker-session-replay"
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-dispatch-worker-session-replay"}
	now := time.Date(2026, 8, 4, 18, 0, 0, 0, time.UTC)
	associations := []struct {
		dispatchID      string
		workerSessionID string
	}{
		{dispatchID: "dispatch-actual-7", workerSessionID: "worker-session-actual-11"},
		{dispatchID: "dispatch-actual-13", workerSessionID: "worker-session-actual-17"},
	}

	emitter := recordingevents.NewRuntimeLedger(nil, func() time.Time { return now }, generationID, nil)
	for index, association := range associations {
		tick := index + 1
		emitter.RecordDispatchWorkerSessionAssociation(
			tick,
			association.dispatchID,
			association.workerSessionID,
			now.Add(time.Duration(index*2)*time.Second),
		)
		emitter.AppendRecordedEvent(dispatchWorkerSessionDependentOutput(
			t,
			tick,
			association.dispatchID,
			now.Add(time.Duration(index*2+1)*time.Second),
		))
	}

	persistedLedger := recordingevents.NewRuntimeLedger(nil, func() time.Time { return now }, generationID, nil)
	persisted := wire.NewService(persistedLedger)
	for _, emitted := range emitter.CanonicalEvents() {
		sessionID := scope.FactorySessionID
		emitted.Context.SessionID = &sessionID
		event := canonical.CanonicalEventFromFactory(emitted, generationID)
		event.Scope = scope
		if _, err := persisted.Append(recordings.AppendRecordedEventRequest{Event: event}); err != nil {
			t.Fatalf("persist emitted event %q: %v", emitted.Id, err)
		}
	}

	retained := persistedLedger.CanonicalEvents()
	freshLedger := reconstructRuntimeLedger(t, retained, generationID, now)
	fresh := wire.NewService(freshLedger)
	read, err := fresh.SubscribeFrom(context.Background(), recordings.SubscribeRequest{Scope: scope})
	if err != nil {
		t.Fatalf("SubscribeFrom fresh ledger: %v", err)
	}

	loaded := make([]recordings.CanonicalEvent, 0, len(retained))
	for range retained {
		outcome := read.Subscription.Next(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent {
			t.Fatalf("fresh replay outcome = %#v, want retained canonical event", outcome)
		}
		loaded = append(loaded, outcome.Event)
	}

	if len(loaded) != len(retained) {
		t.Fatalf("fresh replay event count = %d, want %d", len(loaded), len(retained))
	}
	for index, association := range associations {
		associationIndex := index * 2
		assertReplayedDispatchWorkerSessionAssociation(
			t,
			loaded[associationIndex],
			associationIndex,
			association,
		)
		assertAssociationPrecedesDependentOutput(t, loaded[associationIndex], loaded[associationIndex+1])
	}
}

func dispatchWorkerSessionDependentOutput(
	t *testing.T,
	tick int,
	dispatchID string,
	eventTime time.Time,
) factorydefinitions.FactoryEvent {
	t.Helper()
	payload, err := json.Marshal(workers.DispatchResponseEventPayload{
		Outcome:      workers.OutcomeAccepted,
		TransitionID: "worker-session-output",
	})
	if err != nil {
		t.Fatalf("marshal dependent Worker Session output: %v", err)
	}
	return factorydefinitions.FactoryEvent{
		Context: factorydefinitions.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  eventTime,
			Tick:       tick,
		},
		Id:            "factory-event/dispatch-response/" + dispatchID,
		Payload:       payload,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          factorydefinitions.FactoryEventTypeDispatchResponse,
	}
}

func assertReplayedDispatchWorkerSessionAssociation(
	t *testing.T,
	event recordings.CanonicalEvent,
	wantSequence int,
	want struct {
		dispatchID      string
		workerSessionID string
	},
) {
	t.Helper()
	if event.Kind != recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc) ||
		event.Sequence != recordings.CanonicalEventSequence(wantSequence) ||
		event.Cursor.Sequence != recordings.CanonicalEventSequence(wantSequence) {
		t.Fatalf("replayed association envelope = %#v, want association at sequence %d", event, wantSequence)
	}

	canonicalEvent := canonical.FactoryEventFromCanonical(event)
	if canonicalEvent.Context.DispatchID == nil || *canonicalEvent.Context.DispatchID != want.dispatchID {
		t.Fatalf("replayed association dispatchId = %#v, want %q", canonicalEvent.Context.DispatchID, want.dispatchID)
	}
	var canonicalPayload factorydefinitions.DispatchWorkerSessionAssociationEventPayload
	if err := canonicalEvent.DecodePayload(&canonicalPayload); err != nil {
		t.Fatalf("decode replayed association payload: %v", err)
	}
	if canonicalPayload.WorkerSessionID != want.workerSessionID {
		t.Fatalf("replayed association workerSessionId = %q, want %q", canonicalPayload.WorkerSessionID, want.workerSessionID)
	}

	publicEvent, err := apisurface.FactoryEventToAPI(canonicalEvent)
	if err != nil {
		t.Fatalf("map replayed association to public event: %v", err)
	}
	encoded, err := json.Marshal(publicEvent)
	if err != nil {
		t.Fatalf("marshal generated public association event: %v", err)
	}
	var decoded factoryapi.FactoryEvent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated public association event: %v", err)
	}
	normalized, err := factorydefinitions.NewFactoryEvent(decoded)
	if err != nil {
		t.Fatalf("normalize generated public association event: %v", err)
	}
	if normalized.Id != canonicalEvent.Id || normalized.Context.Sequence != canonicalEvent.Context.Sequence ||
		normalized.Type != factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc ||
		normalized.Context.DispatchID == nil || *normalized.Context.DispatchID != want.dispatchID {
		t.Fatalf("normalized replayed association = %#v, want exact event and dispatch identities", normalized)
	}
	var normalizedPayload factorydefinitions.DispatchWorkerSessionAssociationEventPayload
	if err := normalized.DecodePayload(&normalizedPayload); err != nil {
		t.Fatalf("decode normalized replayed association payload: %v", err)
	}
	if normalizedPayload.WorkerSessionID != want.workerSessionID {
		t.Fatalf("normalized replayed workerSessionId = %q, want %q", normalizedPayload.WorkerSessionID, want.workerSessionID)
	}
}

func assertAssociationPrecedesDependentOutput(
	t *testing.T,
	association recordings.CanonicalEvent,
	output recordings.CanonicalEvent,
) {
	t.Helper()
	if output.Kind != recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchResponse) ||
		output.Sequence != association.Sequence+1 {
		t.Fatalf("replayed dependent output = %#v, want immediately after association %#v", output, association)
	}
	outputEvent := canonical.FactoryEventFromCanonical(output)
	associationEvent := canonical.FactoryEventFromCanonical(association)
	if associationEvent.Context.DispatchID == nil || outputEvent.Context.DispatchID == nil ||
		*outputEvent.Context.DispatchID != *associationEvent.Context.DispatchID {
		t.Fatalf(
			"replayed dependent output dispatchId = %#v, want association dispatchId %#v",
			outputEvent.Context.DispatchID,
			associationEvent.Context.DispatchID,
		)
	}
}

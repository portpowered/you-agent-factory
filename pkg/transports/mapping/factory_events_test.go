package apisurface

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventsToAPIRejectsMalformedCanonicalPayload(t *testing.T) {
	_, err := FactoryEventsToAPI([]interfaces.FactoryEvent{{
		Id:            "factory-event/run-finished",
		Payload:       json.RawMessage(`{"state":`),
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeRunResponse,
	}})
	if err == nil || !strings.Contains(err.Error(), "factory-event/run-finished") {
		t.Fatalf("malformed canonical event mapping error = %v, want event identity", err)
	}
}

func TestFactoryEventAssociationRoundTripPreservesDispatchAndWorkerSessionIDs(t *testing.T) {
	dispatchID := "dispatch-actual-7"
	eventTime := time.Date(2026, 8, 4, 16, 30, 0, 0, time.UTC)
	payload, err := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{
		WorkerSessionID: "worker-session-actual-11",
	})
	if err != nil {
		t.Fatalf("marshal canonical association payload: %v", err)
	}
	canonical := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			Sequence:   17,
			Tick:       8,
			EventTime:  eventTime,
			DispatchID: &dispatchID,
		},
		Id:            "factory-event/dispatch-worker-session-association/dispatch-actual-7",
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}

	public, err := FactoryEventToAPI(canonical)
	if err != nil {
		t.Fatalf("map association to generated public event: %v", err)
	}
	if public.Type != factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation {
		t.Fatalf("public event type = %q, want %q", public.Type, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation)
	}
	if public.Context.DispatchId == nil || *public.Context.DispatchId != dispatchID {
		t.Fatalf("public context.dispatchId = %#v, want %q", public.Context.DispatchId, dispatchID)
	}
	publicPayload, err := public.Payload.AsDispatchWorkerSessionAssociationEventPayload()
	if err != nil {
		t.Fatalf("decode generated association payload: %v", err)
	}
	if publicPayload.WorkerSessionId != "worker-session-actual-11" {
		t.Fatalf("public payload.workerSessionId = %q, want worker-session-actual-11", publicPayload.WorkerSessionId)
	}

	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatalf("marshal generated association event: %v", err)
	}
	var roundTripped factoryapi.FactoryEvent
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal generated association event: %v", err)
	}
	normalized, err := interfaces.NewFactoryEvent(roundTripped)
	if err != nil {
		t.Fatalf("normalize generated association event: %v", err)
	}
	if normalized.Id != canonical.Id || normalized.Type != canonical.Type || normalized.Context.Sequence != canonical.Context.Sequence || normalized.Context.Tick != canonical.Context.Tick || !normalized.Context.EventTime.Equal(canonical.Context.EventTime) {
		t.Fatalf("normalized association envelope = %#v, want canonical identity and ordering context", normalized)
	}
	if normalized.Context.DispatchID == nil || *normalized.Context.DispatchID != dispatchID {
		t.Fatalf("normalized context.dispatchId = %#v, want %q", normalized.Context.DispatchID, dispatchID)
	}
	var normalizedPayload interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := normalized.DecodePayload(&normalizedPayload); err != nil {
		t.Fatalf("decode normalized association payload: %v", err)
	}
	if normalizedPayload.WorkerSessionID != "worker-session-actual-11" {
		t.Fatalf("normalized payload.workerSessionId = %q, want worker-session-actual-11", normalizedPayload.WorkerSessionID)
	}
}

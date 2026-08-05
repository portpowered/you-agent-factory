package runtime

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestCaptureAssociatedWorkerSessionTargets_SelectsOneDeterministicSnapshot(t *testing.T) {
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 30, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 20, "association-duplicate", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 40, "association-next-turn", "turn-next", "worker-next"),
		directWorkerEvent(t, 50, "turn-captured", "worker-direct"),
		malformedWorkerSessionAssociationEvent("turn-captured"),
	}}

	captured := captureAssociatedWorkerSessionTargets(ledger, "turn-captured")
	if captured.turnID != "turn-captured" {
		t.Fatalf("captured turn ID = %q, want turn-captured", captured.turnID)
	}
	if got, want := captured.workerSessionIDsSnapshot(), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("captured Worker Session IDs = %v, want %v", got, want)
	}

	// This deterministic post-capture commit models an association that races
	// after the control's ledger-snapshot linearization point. A retry must
	// retain captured rather than silently reselect this later child.
	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 60, "association-late", "turn-captured", "worker-late"))
	if got, want := captured.workerSessionIDsSnapshot(), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("captured target set after later association = %v, want immutable %v", got, want)
	}
	if got, want := captureAssociatedWorkerSessionTargets(ledger, "turn-captured").workerSessionIDsSnapshot(), []string{"worker-a", "worker-b", "worker-late"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("new capture after later association = %v, want %v", got, want)
	}
}

func TestCaptureAssociatedWorkerSessionTargets_IsolatesFactorySessionLedgersAndEmptyTurns(t *testing.T) {
	currentFactorySession := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 1, "current-association", "turn-1", "worker-current"),
	}}
	otherFactorySession := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 1, "other-association", "turn-1", "worker-other"),
	}}

	if got, want := captureAssociatedWorkerSessionTargets(currentFactorySession, "turn-1").workerSessionIDsSnapshot(), []string{"worker-current"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("current Factory Session targets = %v, want %v", got, want)
	}
	if got, want := captureAssociatedWorkerSessionTargets(otherFactorySession, "turn-1").workerSessionIDsSnapshot(), []string{"worker-other"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("other Factory Session targets = %v, want %v", got, want)
	}
	if got := captureAssociatedWorkerSessionTargets(currentFactorySession, " ").workerSessionIDsSnapshot(); len(got) != 0 {
		t.Fatalf("blank turn target set = %v, want no-op empty selection", got)
	}
}

func workerSessionAssociationEvent(t *testing.T, sequence int, id, turnID, workerSessionID string) interfaces.FactoryEvent {
	t.Helper()
	payload, err := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID})
	if err != nil {
		t.Fatalf("marshal association payload: %v", err)
	}
	dispatchID := "dispatch-" + workerSessionID
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC),
			RequestID:  &turnID,
			Sequence:   sequence,
		},
		Id:            id,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}
}

func directWorkerEvent(t *testing.T, sequence int, turnID, workerSessionID string) interfaces.FactoryEvent {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"workerSessionId": workerSessionID})
	if err != nil {
		t.Fatalf("marshal direct Worker event payload: %v", err)
	}
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			EventTime: time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC),
			RequestID: &turnID,
			Sequence:  sequence,
		},
		Id:            "direct-worker-event",
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchResponse,
	}
}

func malformedWorkerSessionAssociationEvent(turnID string) interfaces.FactoryEvent {
	dispatchID := "dispatch-malformed"
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC),
			RequestID:  &turnID,
			Sequence:   55,
		},
		Id:            "association-malformed",
		Payload:       []byte(`not-json`),
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}
}

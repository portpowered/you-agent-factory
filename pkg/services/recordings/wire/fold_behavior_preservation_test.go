package wire_test

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

// Fold-behavior preservation tests construct Recordings exclusively through
// recordings/wire and exercise append and projection queries through the
// published Service root after the internal composed-root relocation.

type behavioralLedger struct {
	events []factorydefinitions.FactoryEvent
}

func (ledger *behavioralLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	out := make([]factorydefinitions.FactoryEvent, len(ledger.events))
	copy(out, ledger.events)
	return out
}

func (ledger *behavioralLedger) Subscribe(
	_ context.Context,
	_ *factorydefinitions.FactoryEventReconnectCursor,
	_ factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return factorydefinitions.FactoryEventStream{
		StreamGenerationID: ledger.StreamGenerationID(),
		History:            ledger.CanonicalEvents(),
	}, nil
}

func (ledger *behavioralLedger) StreamGenerationID() string { return "wire-fold-gen" }

func (ledger *behavioralLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *behavioralLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (ledger *behavioralLedger) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	event.Context.Sequence = len(ledger.events)
	ledger.events = append(ledger.events, event)
}

func TestWireFoldPreservesAppendAndProjectionQueryThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	ledger := &behavioralLedger{}
	service, err := recordingswire.NewService(
		ledger,
		nil,
		func(string, []byte) error { return nil },
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root recordings.Service = service
	if root == nil {
		t.Fatal("constructed service is not assignable to recordings.Service")
	}

	scope := recordings.CanonicalEventScope{FactorySessionID: "wire-fold-session"}
	event := recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID("wire-fold-event"),
		Sequence:    0,
		FactoryTick: 1,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: ledger.StreamGenerationID(),
			Sequence:           0,
		},
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunResponse),
		Payload:    `{}`,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
	}

	accepted, err := root.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append() = %v", err)
	}
	if accepted.Event.ID != event.ID {
		t.Fatalf("Append() event ID = %q, want %q", accepted.Event.ID, event.ID)
	}

	reconstructed, err := root.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        scope,
		Events:       []recordings.CanonicalEvent{accepted.Event},
		SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState() = %v", err)
	}

	dashboard, err := root.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: reconstructed.WorldState,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard() = %v", err)
	}
	if reconstructed.WorldState.SchemaVersion == "" {
		t.Fatalf("ReconstructWorldState() returned empty schema version: %#v", reconstructed.WorldState)
	}
	if reconstructed.WorldState.Scope != scope {
		t.Fatalf("ReconstructWorldState() scope = %#v, want %#v", reconstructed.WorldState.Scope, scope)
	}
	if dashboard.Data.ActiveExecutionsByDispatchID == nil {
		t.Fatalf("QuerySimpleDashboard() returned nil active executions map: %#v", dashboard.Data)
	}
}

package canonicalledger_test

import (
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

func TestAppendAcceptsPublicEmittableFactoryEventKind(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	event := scopedAppendEvent("evt-public", 0)
	event.Kind = recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunRequest)

	if _, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event}); err != nil {
		t.Fatalf("Append public emittable kind: %v", err)
	}
	if len(ledger.events) != 1 {
		t.Fatalf("ledger events = %d, want one retained public append", len(ledger.events))
	}
}

func TestAppendRejectsExcludedNonPublicVocabulariesWithoutMutation(t *testing.T) {
	t.Parallel()

	valid := scopedAppendEvent("evt-valid", 0)
	tests := map[string]recordings.CanonicalEventKind{
		"retired factory event alias": recordings.CanonicalEventKind("RUN_STARTED"),
		"contract-only public kind": recordings.CanonicalEventKind(
			factorydefinitions.FactoryEventTypeJavaScriptCheckpointRef,
		),
		"response-stream retention record": recordings.CanonicalEventKind("PROGRESS_FRAGMENT"),
	}
	for name, kind := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := &stubLedger{}
			svc := wire.NewService(ledger)
			event := valid
			event.ID = recordings.CanonicalEventID("evt-" + name)
			event.Kind = kind

			if _, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event}); !errors.Is(
				err,
				recordings.ErrInvalidAppendEvent,
			) {
				t.Fatalf("Append excluded kind error = %v, want ErrInvalidAppendEvent", err)
			}
			if len(ledger.events) != 0 {
				t.Fatalf("Append excluded kind mutated ledger: %#v", ledger.events)
			}
		})
	}
}

func TestAppendRejectsArbitraryNonInventoryKind(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	event := recordings.CanonicalEvent{
		ID:         "evt-arbitrary",
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       recordings.CanonicalEventKind("OVERFLOW_TEST"),
		Payload:    `{"retained":false}`,
	}

	if _, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event}); !errors.Is(
		err,
		recordings.ErrInvalidAppendEvent,
	) {
		t.Fatalf("Append arbitrary kind error = %v, want ErrInvalidAppendEvent", err)
	}
	if len(ledger.events) != 0 {
		t.Fatalf("Append arbitrary kind mutated ledger: %#v", ledger.events)
	}
}

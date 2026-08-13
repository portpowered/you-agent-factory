package canonicalledger_test

import (
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

func TestAppendWithValidationUsesAtomicLedgerAdmission(t *testing.T) {
	t.Parallel()

	validationErr := errors.New("lifecycle rejected event")
	ledger := &validatingLedger{stubLedger: &stubLedger{}}
	service := wire.NewService(ledger)
	event := scopedAppendEvent(
		"evt-validated",
		99,
		recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	)
	called := false
	if _, err := service.AppendWithValidation(
		recordings.AppendRecordedEventRequest{Event: event},
		func(accepted recordings.CanonicalEvent) error {
			called = true
			if accepted.Sequence != 0 {
				t.Fatalf("validation event sequence = %d, want canonical sequence 0", accepted.Sequence)
			}
			if accepted.Scope != event.Scope {
				t.Fatalf("validation event scope = %#v, want %#v", accepted.Scope, event.Scope)
			}
			return validationErr
		},
	); !errors.Is(err, validationErr) {
		t.Fatalf("AppendWithValidation rejected event error = %v, want %v", err, validationErr)
	}
	if !called {
		t.Fatal("AppendWithValidation did not invoke lifecycle validation")
	}
	if ledger.validationCalls != 1 {
		t.Fatalf("atomic ledger validation calls = %d, want 1", ledger.validationCalls)
	}
	if len(ledger.events) != 0 {
		t.Fatalf("rejected atomic append retained %d events, want 0", len(ledger.events))
	}

	accepted, err := service.AppendWithValidation(
		recordings.AppendRecordedEventRequest{Event: scopedAppendEvent("evt-accepted", 0)},
		nil,
	)
	if err != nil {
		t.Fatalf("AppendWithValidation nil validation: %v", err)
	}
	if accepted.Event.ID != "evt-accepted" || accepted.Event.Sequence != 0 {
		t.Fatalf("accepted atomic append = %#v, want evt-accepted at sequence 0", accepted.Event)
	}
	if ledger.validationCalls != 2 || len(ledger.events) != 1 {
		t.Fatalf("accepted atomic append state = calls:%d events:%d, want calls:2 events:1", ledger.validationCalls, len(ledger.events))
	}
}

func TestAppendWithValidationHandlesRetainedAndUnsupportedLedgers(t *testing.T) {
	t.Parallel()

	retainedLedger := &validatingLedger{stubLedger: &stubLedger{}}
	service := wire.NewService(retainedLedger)
	event := scopedAppendEvent("evt-retained", 0)
	first, err := service.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append initial retained event: %v", err)
	}
	retainedErr := errors.New("retained event validation failed")
	if _, err := service.AppendWithValidation(
		recordings.AppendRecordedEventRequest{Event: event},
		func(accepted recordings.CanonicalEvent) error {
			if accepted != first.Event {
				t.Fatalf("retained validation event = %#v, want %#v", accepted, first.Event)
			}
			return retainedErr
		},
	); !errors.Is(err, retainedErr) {
		t.Fatalf("retained AppendWithValidation error = %v, want %v", err, retainedErr)
	}
	if retainedLedger.validationCalls != 0 || len(retainedLedger.events) != 1 {
		t.Fatalf("retained append mutated atomic ledger: calls:%d events:%d", retainedLedger.validationCalls, len(retainedLedger.events))
	}

	plainLedger := &stubLedger{}
	if _, err := wire.NewService(plainLedger).AppendWithValidation(
		recordings.AppendRecordedEventRequest{Event: scopedAppendEvent("evt-unsupported", 0)},
		nil,
	); !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("unsupported ledger error = %v, want ErrRecordingWriteRejected", err)
	}
	if len(plainLedger.events) != 0 {
		t.Fatalf("unsupported ledger retained %d events, want 0", len(plainLedger.events))
	}

	invalid := scopedAppendEvent("evt-invalid", 0)
	invalid.Payload = "{"
	if _, err := service.AppendWithValidation(
		recordings.AppendRecordedEventRequest{Event: invalid},
		nil,
	); !errors.Is(err, recordings.ErrInvalidAppendEvent) {
		t.Fatalf("invalid AppendWithValidation error = %v, want ErrInvalidAppendEvent", err)
	}
}

type validatingLedger struct {
	*stubLedger
	validationCalls int
}

func (ledger *validatingLedger) AppendRecordedEventWithValidation(
	event factorydefinitions.FactoryEvent,
	validate func(factorydefinitions.FactoryEvent) error,
) (factorydefinitions.FactoryEvent, error) {
	ledger.validationCalls++
	event.Context.Sequence = len(ledger.events)
	if validate != nil {
		if err := validate(event); err != nil {
			return factorydefinitions.FactoryEvent{}, err
		}
	}
	ledger.events = append(ledger.events, event)
	return event, nil
}

var _ recordings.Ledger = (*validatingLedger)(nil)

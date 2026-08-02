package events

import (
	"errors"
	"testing"
)

func TestRecordKeyIsExactIdempotencyTuple(t *testing.T) {
	a := Record{
		Topic:             "factory-session/s1/response-events",
		SourceType:        "factory_session",
		SourceID:          "s1",
		SourceSequence:    1,
		SourceEventID:     "evt-1",
		Schema:            "schema.v1",
		AggregateSequence: 7,
		Generation:        1,
		Payload:           "unrelated payload difference",
	}
	b := a
	b.AggregateSequence = 99
	b.Generation = 2
	b.Payload = map[string]any{"different": true}

	if a.Key() != b.Key() {
		t.Fatalf("expected records sharing (SourceType, SourceID, SourceSequence, SourceEventID) to share a Key(); got %+v vs %+v", a.Key(), b.Key())
	}

	c := a
	c.SourceSequence = 2
	if a.Key() == c.Key() {
		t.Fatalf("expected differing SourceSequence to change Key()")
	}
}

func TestValidationErrorUnwrapsToSentinel(t *testing.T) {
	verr := &ValidationError{Field: "topic", Err: ErrInvalidTopic}

	if !errors.Is(verr, ErrInvalidTopic) {
		t.Fatalf("expected errors.Is(verr, ErrInvalidTopic) to hold")
	}
	if verr.Error() == "" {
		t.Fatalf("expected a non-empty error message")
	}
}

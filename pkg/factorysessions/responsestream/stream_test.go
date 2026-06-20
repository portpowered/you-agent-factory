package responsestream_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func TestSessionResponseStream_AppendAssignsMonotonicSequenceAndRetentionMetadata(t *testing.T) {
	start := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	stream := responsestream.NewSessionResponseStreamWithClock(
		fixedClock{now: start},
		responsestream.DefaultRetentionLimits(),
	)

	first := stream.Append(responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: "dispatch-1",
		Payload:    "phase=planning",
	})
	second := stream.Append(responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		DispatchID: "dispatch-1",
		Payload:    "partial response",
		ProviderSessionRef: &interfaces.ProviderSessionMetadata{
			Provider: "cursor",
			Kind:     "session_id",
			ID:       "sess-1",
		},
	})

	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences = [%d %d], want [1 2]", first.Sequence, second.Sequence)
	}
	if !first.RecordedAt.Equal(start) || !second.RecordedAt.Equal(start) {
		t.Fatalf("recordedAt = [%s %s], want %s", first.RecordedAt, second.RecordedAt, start)
	}
	if first.PayloadBytes != len([]byte(first.Payload)) || second.PayloadBytes != len([]byte(second.Payload)) {
		t.Fatalf("payload bytes = [%d %d], want derived from payload length", first.PayloadBytes, second.PayloadBytes)
	}

	events := stream.Events()
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("retained events = %#v, want ascending sequence order", events)
	}

	accounting := stream.RetentionAccounting()
	if accounting.EventCount != 2 {
		t.Fatalf("event count = %d, want 2", accounting.EventCount)
	}
	if accounting.TotalPayloadBytes != first.PayloadBytes+second.PayloadBytes {
		t.Fatalf("total payload bytes = %d, want %d", accounting.TotalPayloadBytes, first.PayloadBytes+second.PayloadBytes)
	}
	if !accounting.OldestRecordedAt.Equal(start) {
		t.Fatalf("oldest recordedAt = %s, want %s", accounting.OldestRecordedAt, start)
	}
}

func TestSessionResponseStream_DefaultRetentionLimitsAreDocumented(t *testing.T) {
	limits := responsestream.DefaultRetentionLimits()
	if limits.MaxBytes <= 0 || limits.MaxEvents <= 0 || limits.MaxAge <= 0 {
		t.Fatalf("default limits = %#v, want positive byte/event/age controls", limits)
	}

	stream := responsestream.NewSessionResponseStream()
	got := stream.RetentionLimits()
	if got != limits {
		t.Fatalf("stream limits = %#v, want %#v", got, limits)
	}
}

func TestSessionResponseStreamEvent_IsNotCanonicalFactoryEvent(t *testing.T) {
	t.Parallel()

	var factoryEventType factoryapi.FactoryEventType
	_ = factoryEventType

	event := responsestream.Event{
		Kind: responsestream.EventKindProgressFragment,
	}
	if string(event.Kind) == string(factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatal("internal stream kind must not alias canonical factory event types")
	}

	stream := responsestream.NewSessionResponseStreamWithClock(factory.RealClock{}, responsestream.RetentionLimits{})
	if stream.Append(event).Kind != responsestream.EventKindProgressFragment {
		t.Fatal("append must preserve internal stream event kind")
	}
}

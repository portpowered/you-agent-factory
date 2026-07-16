package responsestream_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	return c.now
}

func progressEvent(dispatchID, payload string) responsestream.Event {
	return responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: dispatchID,
		Payload:    payload,
	}
}

func TestSessionResponseStream_AppendAssignsMonotonicSequenceAndRetentionMetadata(t *testing.T) {
	start := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: start},
		responsestream.DefaultRetentionLimits(),
	)

	first, firstCompaction := stream.Append(responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: "dispatch-1",
		Payload:    "phase=planning",
	})
	second, secondCompaction := stream.Append(responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		DispatchID: "dispatch-1",
		Payload:    "partial response",
		ProviderSessionRef: &workerexecution.ProviderSessionMetadata{
			Provider: "cursor",
			Kind:     "session_id",
			ID:       "sess-1",
		},
	})
	if firstCompaction != nil || secondCompaction != nil {
		t.Fatalf("unexpected compaction = [%v %v], want nil under default limits", firstCompaction, secondCompaction)
	}

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

	stream := responsestream.NewSessionResponseStreamWithClock(platformclock.Real{}, responsestream.RetentionLimits{})
	stored, compaction := stream.Append(event)
	if compaction != nil {
		t.Fatalf("compaction = %#v, want nil with unlimited limits", compaction)
	}
	if stored.Kind != responsestream.EventKindProgressFragment {
		t.Fatal("append must preserve internal stream event kind")
	}
}

func TestSessionResponseStream_TruncatesWhenEventCountExceeded(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 2},
	)

	for i := 1; i <= 3; i++ {
		_, compaction := stream.Append(progressEvent("dispatch-"+strconv.Itoa(i), "chunk"))
		if i < 3 && compaction != nil {
			t.Fatalf("append %d compaction = %#v, want nil before limit", i, compaction)
		}
		if i == 3 {
			if compaction == nil || compaction.Reason != responsestream.CompactionReasonTruncated {
				t.Fatalf("final compaction = %#v, want truncated summary", compaction)
			}
			if compaction.FirstRetainedSequence != 2 || compaction.LastDroppedSequence != 1 {
				t.Fatalf("compaction sequences = [%d %d], want first retained 2 and last dropped 1",
					compaction.FirstRetainedSequence, compaction.LastDroppedSequence)
			}
		}
	}

	events := stream.Events()
	if len(events) != 2 || events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("retained events = %#v, want sequences [2 3]", events)
	}
}

func TestSessionResponseStream_TruncatesWhenByteLimitExceeded(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxBytes: 8},
	)

	_, firstCompaction := stream.Append(progressEvent("dispatch-1", "12345"))
	if firstCompaction != nil {
		t.Fatalf("first compaction = %#v, want nil", firstCompaction)
	}
	_, secondCompaction := stream.Append(progressEvent("dispatch-2", "67890"))
	if secondCompaction == nil || secondCompaction.Reason != responsestream.CompactionReasonTruncated {
		t.Fatalf("second compaction = %#v, want byte truncation", secondCompaction)
	}

	accounting := stream.RetentionAccounting()
	if accounting.TotalPayloadBytes > 8 {
		t.Fatalf("total payload bytes = %d, want <= 8", accounting.TotalPayloadBytes)
	}
}

func TestSessionResponseStream_EnforceRetentionAfterDispatchCompletion(t *testing.T) {
	start := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	stream := responsestream.NewSessionResponseStreamWithClock(
		clock,
		responsestream.RetentionLimits{MaxAge: time.Minute},
	)

	_, firstCompaction := stream.Append(progressEvent("dispatch-1", "old"))
	if firstCompaction != nil {
		t.Fatalf("first compaction = %#v, want nil", firstCompaction)
	}
	stream.CompleteDispatch()

	clock.now = start.Add(2 * time.Minute)
	compaction := stream.EnforceRetention()
	if compaction == nil || compaction.Reason != responsestream.CompactionReasonAgeEvicted {
		t.Fatalf("compaction = %#v, want age eviction after dispatch completion", compaction)
	}
	if events := stream.Events(); len(events) != 0 {
		t.Fatalf("retained events = %#v, want empty window after age eviction", events)
	}
}

func TestSessionResponseStream_EvictsByAge(t *testing.T) {
	start := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	stream := responsestream.NewSessionResponseStreamWithClock(
		clock,
		responsestream.RetentionLimits{MaxAge: time.Minute},
	)

	_, firstCompaction := stream.Append(progressEvent("dispatch-1", "old"))
	if firstCompaction != nil {
		t.Fatalf("first compaction = %#v, want nil", firstCompaction)
	}

	clock.now = start.Add(2 * time.Minute)
	_, secondCompaction := stream.Append(progressEvent("dispatch-1", "new"))
	if secondCompaction == nil || secondCompaction.Reason != responsestream.CompactionReasonAgeEvicted {
		t.Fatalf("second compaction = %#v, want age eviction", secondCompaction)
	}

	events := stream.Events()
	if len(events) != 1 || events[0].Payload != "new" {
		t.Fatalf("retained events = %#v, want only new event", events)
	}
}

func TestSessionResponseStream_CoalescesAdjacentProgressFragments(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 2},
	)

	stream.Append(progressEvent("dispatch-1", "alpha"))
	stream.Append(progressEvent("dispatch-1", "beta"))
	_, compaction := stream.Append(progressEvent("dispatch-2", "gamma"))
	if compaction == nil || compaction.Reason != responsestream.CompactionReasonCoalesced {
		t.Fatalf("compaction = %#v, want coalesced summary", compaction)
	}

	events := stream.Events()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2 retained events", len(events))
	}
	if events[0].Payload != "alphabeta" || events[1].Payload != "gamma" {
		t.Fatalf("payloads = [%q %q], want merged first dispatch fragments", events[0].Payload, events[1].Payload)
	}
	if events[0].Sequence >= events[1].Sequence {
		t.Fatalf("sequences = [%d %d], want ascending order", events[0].Sequence, events[1].Sequence)
	}
}

func TestSessionResponseStream_EventsAfterSlowConsumerDetectsCompaction(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 2},
	)

	stream.Append(progressEvent("dispatch-1", "one"))
	stream.Append(progressEvent("dispatch-2", "two"))
	stream.Append(progressEvent("dispatch-3", "three"))

	read := stream.EventsAfter(1)
	if !read.BehindRetainedWindow {
		t.Fatal("behind retained window = false, want true for slow consumer")
	}
	if read.Compaction == nil || read.Compaction.Reason != responsestream.CompactionReasonTruncated {
		t.Fatalf("compaction = %#v, want truncated catch-up summary", read.Compaction)
	}
	if len(read.Events) != 2 || read.Events[0].Sequence != 2 {
		t.Fatalf("catch-up events = %#v, want retained window from sequence 2", read.Events)
	}

	current := stream.EventsAfter(2)
	if current.BehindRetainedWindow || len(current.Events) != 1 || current.Events[0].Sequence != 3 {
		t.Fatalf("current read = %#v, want single event after sequence 2", current)
	}
}

func TestSessionResponseStream_RetainedEventsPreserveMonotonicSequence(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 3},
	)

	for i := 0; i < 5; i++ {
		stream.Append(progressEvent("dispatch-"+string(rune('a'+i)), "chunk"))
	}

	events := stream.Events()
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			t.Fatalf("sequences = %#v, want strictly increasing order", events)
		}
	}
}

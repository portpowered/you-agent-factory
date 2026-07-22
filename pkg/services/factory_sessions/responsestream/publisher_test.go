package responsestream_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
)

func TestPublisher_PublishIncrementsDiagnostics(t *testing.T) {
	stream := newResponseStream()
	publisher := responsestream.NewPublisher(stream, nil)

	first := publisher.Publish(responsestream.Event{
		Kind:    responsestream.EventKindProgressFragment,
		Payload: "phase=planning",
	})
	second := publisher.Publish(responsestream.Event{
		Kind:    responsestream.EventKindResponseFragment,
		Payload: "partial",
	})

	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences = [%d %d], want [1 2]", first.Sequence, second.Sequence)
	}
	diagnostics := publisher.Diagnostics()
	if diagnostics.PublishedCount != 2 {
		t.Fatalf("published count = %d, want 2", diagnostics.PublishedCount)
	}
}

func TestPublisher_ReportCompactionEmitsDiagnostics(t *testing.T) {
	var observed responsestream.CompactionSummary
	stream := newResponseStream()
	publisher := responsestream.NewPublisher(stream, func(summary responsestream.CompactionSummary) {
		observed = summary
	})

	summary := responsestream.CompactionSummary{
		Reason:                responsestream.CompactionReasonTruncated,
		DroppedSequenceCount:  2,
		FirstRetainedSequence: 3,
		LastDroppedSequence:   2,
	}
	publisher.ReportCompaction(summary)

	if observed.Reason != summary.Reason {
		t.Fatalf("observed reason = %q, want %q", observed.Reason, summary.Reason)
	}
	events := stream.Events()
	if len(events) != 1 || events[0].Kind != responsestream.EventKindCompactionSignal {
		t.Fatalf("events = %#v, want compaction signal", events)
	}
	diagnostics := publisher.Diagnostics()
	if diagnostics.CompactionCount != 1 {
		t.Fatalf("compaction count = %d, want 1", diagnostics.CompactionCount)
	}
}

func TestPublisher_RepeatedCompactionSignalsStayBounded(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 2},
	)
	publisher := responsestream.NewPublisher(stream, nil)

	for i := 0; i < 10; i++ {
		publisher.Publish(responsestream.Event{
			Kind:       responsestream.EventKindProgressFragment,
			DispatchID: "dispatch-1",
			Payload:    "chunk-" + string(rune('a'+i)),
		})
	}

	events := stream.Events()
	progressCount := 0
	signalCount := 0
	for _, event := range events {
		switch event.Kind {
		case responsestream.EventKindCompactionSignal:
			signalCount++
		default:
			progressCount++
		}
	}
	if signalCount > 1 {
		t.Fatalf("compaction signal count = %d, want at most 1", signalCount)
	}
	if progressCount > 2 {
		t.Fatalf("retained progress count = %d, want at most MaxEvents=2", progressCount)
	}
	if len(events) > 3 {
		t.Fatalf("retained event count = %d, want at most MaxEvents plus one compaction signal", len(events))
	}

	accounting := stream.RetentionAccounting()
	if accounting.EventCount != len(events) {
		t.Fatalf("accounting event count = %d, want %d retained events", accounting.EventCount, len(events))
	}
}

func TestPublisher_ReportCompactionReplacesExistingSignal(t *testing.T) {
	stream := newResponseStream()
	publisher := responsestream.NewPublisher(stream, nil)

	first := responsestream.CompactionSummary{
		Reason:                responsestream.CompactionReasonTruncated,
		DroppedSequenceCount:  1,
		FirstRetainedSequence: 2,
		LastDroppedSequence:   1,
	}
	second := responsestream.CompactionSummary{
		Reason:                responsestream.CompactionReasonCoalesced,
		DroppedSequenceCount:  1,
		FirstRetainedSequence: 4,
		LastDroppedSequence:   3,
	}
	publisher.ReportCompaction(first)
	publisher.ReportCompaction(second)

	events := stream.Events()
	if len(events) != 1 || events[0].Kind != responsestream.EventKindCompactionSignal {
		t.Fatalf("events = %#v, want single compaction signal", events)
	}
	if events[0].Compaction == nil {
		t.Fatal("compaction summary = nil, want merged summary")
	}
	if events[0].Compaction.DroppedSequenceCount != 2 {
		t.Fatalf("dropped sequence count = %d, want 2 from merged compactions", events[0].Compaction.DroppedSequenceCount)
	}
	if events[0].Sequence != 2 {
		t.Fatalf("compaction sequence = %d, want 2 after replacement", events[0].Sequence)
	}
	diagnostics := publisher.Diagnostics()
	if diagnostics.CompactionCount != 2 {
		t.Fatalf("compaction count = %d, want 2 reported compactions", diagnostics.CompactionCount)
	}
}

func TestPublisher_SecondCompactionPreservesSequenceOrder(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 1},
	)
	publisher := responsestream.NewPublisher(stream, nil)

	publisher.Publish(responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: "dispatch-1",
		Payload:    "A",
	})
	publisher.Publish(responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: "dispatch-1",
		Payload:    "B",
	})
	publisher.Publish(responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: "dispatch-1",
		Payload:    "C",
	})

	events := stream.Events()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want retained progress plus compaction signal", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			t.Fatalf("sequences = %#v, want strictly ascending order", events)
		}
	}
	if events[0].Kind != responsestream.EventKindProgressFragment || events[0].Payload != "C" {
		t.Fatalf("first event = %#v, want retained progress C", events[0])
	}
	if events[1].Kind != responsestream.EventKindCompactionSignal {
		t.Fatalf("second event = %#v, want compaction signal at tail", events[1])
	}

	behind := stream.EventsAfter(2)
	if !behind.BehindRetainedWindow {
		t.Fatal("behind retained window = false, want true after dropped progress")
	}
	if behind.FirstRetainedSequence != events[0].Sequence {
		t.Fatalf("first retained sequence = %d, want %d from oldest progress event",
			behind.FirstRetainedSequence, events[0].Sequence)
	}

	current := stream.EventsAfter(events[0].Sequence)
	if current.BehindRetainedWindow {
		t.Fatal("behind retained window = true, want false at latest progress sequence")
	}
	if len(current.Events) != 1 || current.Events[0].Kind != responsestream.EventKindCompactionSignal {
		t.Fatalf("current read = %#v, want compaction signal after latest progress", current.Events)
	}
}

func TestPublisher_PublishReportsRetentionCompaction(t *testing.T) {
	var observed responsestream.CompactionSummary
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 1},
	)
	publisher := responsestream.NewPublisher(stream, func(summary responsestream.CompactionSummary) {
		observed = summary
	})

	publisher.Publish(responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: "dispatch-1",
		Payload:    "first",
	})
	publisher.Publish(responsestream.Event{
		Kind:       responsestream.EventKindProgressFragment,
		DispatchID: "dispatch-2",
		Payload:    "second",
	})

	if observed.Reason != responsestream.CompactionReasonTruncated {
		t.Fatalf("observed reason = %q, want truncated", observed.Reason)
	}
	events := stream.Events()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want retained progress plus compaction signal", len(events))
	}
	if events[0].Payload != "second" || events[1].Kind != responsestream.EventKindCompactionSignal {
		t.Fatalf("events = %#v, want truncated progress and compaction signal", events)
	}
}

func TestPublisher_SlowSubscriberDoesNotBlockAndReceivesCompactionSignal(t *testing.T) {
	stream := responsestream.NewSessionResponseStreamWithClock(
		&fixedClock{now: time.Unix(0, 0).UTC()},
		responsestream.RetentionLimits{MaxEvents: 2},
	)
	publisher := responsestream.NewPublisher(stream, nil)

	subscription := mustSubscribe(t, stream, 0)
	defer subscription.Detach()

	first := publisher.Publish(responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		DispatchID: "dispatch-1",
		Payload:    "retained-1",
	})
	initial := mustReadNext(t, subscription, "initial")
	if len(initial.Events) != 1 || initial.Events[0].Sequence != first.Sequence {
		t.Fatalf("initial events = %#v, want first retained event", initial.Events)
	}

	publishAlternatingFragmentsAsync(t, publisher, 64)
	catchUp := mustReadNext(t, subscription, "catch-up")
	assertRetainedWindowCompaction(t, catchUp)

	diagnostics := publisher.Diagnostics()
	if diagnostics.PublishedCount != 65 {
		t.Fatalf("published count = %d, want 65", diagnostics.PublishedCount)
	}
	if diagnostics.CompactionCount == 0 || diagnostics.LastCompaction == nil {
		t.Fatalf("diagnostics = %#v, want recorded compaction", diagnostics)
	}
}

func mustSubscribe(t *testing.T, stream *factorysessions.SessionResponseStream, sequence int64) *responsestream.Subscription {
	t.Helper()

	subscription, err := stream.Subscribe(sequence)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	return subscription
}

func mustReadNext(t *testing.T, subscription *responsestream.Subscription, label string) responsestream.ReadResult {
	t.Helper()

	result, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(%s): %v", label, err)
	}
	return result
}

func publishAlternatingFragmentsAsync(t *testing.T, publisher *responsestream.Publisher, count int) {
	t.Helper()

	publishDone := make(chan struct{})
	go func() {
		for i := 0; i < count; i++ {
			kind := responsestream.EventKindProgressFragment
			if i%2 != 0 {
				kind = responsestream.EventKindResponseFragment
			}
			publisher.Publish(responsestream.Event{
				Kind:       kind,
				DispatchID: "dispatch-1",
				Payload:    "chunk-" + strconv.Itoa(i),
			})
		}
		close(publishDone)
	}()

	select {
	case <-publishDone:
	case <-time.After(time.Second):
		t.Fatal("publishing stalled behind a slow subscriber")
	}
}

func assertRetainedWindowCompaction(t *testing.T, result responsestream.ReadResult) {
	t.Helper()

	if !result.BehindRetainedWindow {
		t.Fatalf("catch-up result = %#v, want retained-window gap signal", result)
	}
	if result.Compaction == nil || result.Compaction.Reason != responsestream.CompactionReasonTruncated {
		t.Fatalf("catch-up compaction = %#v, want truncation summary", result.Compaction)
	}
	for _, event := range result.Events {
		if event.Kind != responsestream.EventKindCompactionSignal {
			continue
		}
		if event.Compaction == nil {
			t.Fatal("compaction signal missing summary")
		}
		return
	}
	t.Fatalf("catch-up events = %#v, want retained compaction signal", result.Events)
}

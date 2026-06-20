package responsestream_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
)

func TestPublisher_PublishIncrementsDiagnostics(t *testing.T) {
	stream := responsestream.NewSessionResponseStream()
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
	stream := responsestream.NewSessionResponseStream()
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
	stream := responsestream.NewSessionResponseStream()
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

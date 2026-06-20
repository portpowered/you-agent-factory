package responsestream_test

import (
	"testing"

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

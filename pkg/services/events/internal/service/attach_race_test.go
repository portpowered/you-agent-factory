package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// TestAttachSource_ConcurrentSourceAppendsForwardInDestinationCommitOrder
// proves that concurrent producers appending to an attached source topic are
// forwarded into the destination without a missing or duplicate destination
// aggregate position, and that the destination's own aggregate order agrees
// with the source's aggregate order for every forwarded record: the source
// record at source position N is always the one forwarded to whatever
// destination position corresponds to it, regardless of goroutine start
// order. Run with -race to additionally prove the nested source/destination
// locking introduced by forwarding is free of data races (see the repo-wide
// sandbox -race limitation noted in append_race_test.go/read_race_test.go).
func TestAttachSource_ConcurrentSourceAppendsForwardInDestinationCommitOrder(t *testing.T) {
	const goroutines = 50
	const perGoroutine = 10
	const total = goroutines * perGoroutine

	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/concurrent-attach/response-events")
	destination := events.Topic("chat-session/concurrent-attach/events")

	if _, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source},
		Mode:        events.AttachModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("AttachSource() error = %v", err)
	}

	var wg sync.WaitGroup
	sourcePositions := make(chan events.AggregateSequence, total)
	for g := range goroutines {
		wg.Go(func() {
			for i := range perGoroutine {
				result, err := st.Append(ctx, events.AppendRequest{
					Topic:          source,
					SourceType:     "worker.tool",
					SourceID:       events.SourceID(fmt.Sprintf("worker-%d", g)),
					SourceSequence: events.SourceSequence(i + 1),
					SourceEventID:  events.SourceEventID(fmt.Sprintf("evt-%d-%d", g, i)),
					SchemaID:       "worker.output.v1",
					Payload:        json.RawMessage(`{"ok":true}`),
				})
				if err != nil {
					t.Errorf("Append() error = %v", err)
					continue
				}
				sourcePositions <- result.Record.ID.Position
			}
		})
	}
	wg.Wait()
	close(sourcePositions)

	sourceSeen := make(map[events.AggregateSequence]bool, total)
	for pos := range sourcePositions {
		sourceSeen[pos] = true
	}
	if len(sourceSeen) != total {
		t.Fatalf("observed %d unique source positions, want %d", len(sourceSeen), total)
	}

	sourceRecords := readAll(st, ctx, t, source)
	destRecords := readAll(st, ctx, t, destination)
	if len(destRecords) != len(sourceRecords) {
		t.Fatalf("destination has %d records, want %d (one forwarded record per source commit, no missing or duplicate)", len(destRecords), len(sourceRecords))
	}

	destPositions := make(map[events.AggregateSequence]bool, total)
	for i, destRec := range destRecords {
		if destRec.ID.Position != events.AggregateSequence(i+1) {
			t.Fatalf("destination record[%d].ID.Position = %d, want %d (destination positions must be contiguous)", i, destRec.ID.Position, i+1)
		}
		if destPositions[destRec.ID.Position] {
			t.Fatalf("destination position %d observed more than once", destRec.ID.Position)
		}
		destPositions[destRec.ID.Position] = true

		srcRec := sourceRecords[i]
		if destRec.SourceType != srcRec.SourceType || destRec.SourceID != srcRec.SourceID ||
			destRec.SourceSequence != srcRec.SourceSequence || destRec.SourceEventID != srcRec.SourceEventID {
			t.Fatalf("destination record[%d] identity = %+v, want it to match source record[%d] identity %+v (destination order must agree with source commit order)",
				i, destRec.Identity(), i, srcRec.Identity())
		}
	}
}

// TestAttachSource_ConcurrentIdempotentAttachOnlyRegistersOnce proves that
// many goroutines racing an identical (Destination, Source) AttachSource
// call converge on exactly one accepted attachment and every other call
// observes AttachOutcomeAlreadyAttached, so a subsequent source commit is
// still forwarded exactly once.
func TestAttachSource_ConcurrentIdempotentAttachOnlyRegistersOnce(t *testing.T) {
	const goroutines = 50

	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/concurrent-idem/response-events")
	destination := events.Topic("chat-session/concurrent-idem/events")
	req := events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source},
		Mode:        events.AttachModeRetainedThenLive,
	}

	var wg sync.WaitGroup
	results := make(chan events.AttachSourceResult, goroutines)
	for range goroutines {
		wg.Go(func() {
			result, err := st.AttachSource(ctx, req)
			if err != nil {
				t.Errorf("AttachSource() error = %v", err)
				return
			}
			results <- result
		})
	}
	wg.Wait()
	close(results)

	accepted := 0
	for result := range results {
		if result.Outcome == events.AttachOutcomeAccepted {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted count = %d, want exactly 1 across %d concurrent racers", accepted, goroutines)
	}

	appendFixture(st, ctx, t, source, 1, "evt-1")
	destRecords := readAll(st, ctx, t, destination)
	if len(destRecords) != 1 {
		t.Fatalf("destination has %d records, want exactly 1: concurrent idempotent attaches must never register more than one forwarding path", len(destRecords))
	}
}

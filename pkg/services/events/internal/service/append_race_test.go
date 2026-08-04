package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// TestAppend_ConcurrentAppendsAssignUniqueContiguousPositions proves that
// concurrent producers appending distinct identities to the same topic never
// duplicate, skip, or reorder aggregate positions, regardless of goroutine
// start order. Run with -race to additionally prove the topic's head,
// records, and identity index are free of data races under contention.
func TestAppend_ConcurrentAppendsAssignUniqueContiguousPositions(t *testing.T) {
	const goroutines = 50
	const perGoroutine = 20
	const total = goroutines * perGoroutine

	st := New()
	ctx := context.Background()
	topic := events.Topic("chat-session/concurrent/events")

	var wg sync.WaitGroup
	positions := make(chan events.AggregateSequence, total)
	errs := make(chan error, total)

	for g := range goroutines {
		wg.Go(func() {
			for i := range perGoroutine {
				req := events.AppendRequest{
					Topic:          topic,
					SourceType:     "worker.tool",
					SourceID:       events.SourceID(fmt.Sprintf("worker-%d", g)),
					SourceSequence: events.SourceSequence(i + 1),
					SourceEventID:  events.SourceEventID(fmt.Sprintf("evt-%d-%d", g, i)),
					SchemaID:       "worker.output.v1",
					Payload:        json.RawMessage(`{"ok":true}`),
				}
				result, err := st.Append(ctx, req)
				if err != nil {
					errs <- err
					continue
				}
				positions <- result.Record.ID.Position
			}
		})
	}
	wg.Wait()
	close(positions)
	close(errs)

	for err := range errs {
		t.Fatalf("Append() error = %v", err)
	}

	seen := make(map[events.AggregateSequence]bool, total)
	for pos := range positions {
		if seen[pos] {
			t.Fatalf("position %d was assigned to more than one accepted append", pos)
		}
		seen[pos] = true
	}
	if len(seen) != total {
		t.Fatalf("observed %d unique positions, want %d", len(seen), total)
	}
	for pos := events.AggregateSequence(1); pos <= events.AggregateSequence(total); pos++ {
		if !seen[pos] {
			t.Fatalf("position %d was never assigned; positions must be contiguous", pos)
		}
	}
}

// TestAppend_ConcurrentDuplicateAppendsConvergeOnOneAcceptedRecord proves
// that many goroutines racing the identical append identity produce exactly
// one accepted outcome and every other call resolves to that same
// duplicate, never a second accepted position.
func TestAppend_ConcurrentDuplicateAppendsConvergeOnOneAcceptedRecord(t *testing.T) {
	const goroutines = 50

	st := New()
	ctx := context.Background()
	req := events.AppendRequest{
		Topic:          "chat-session/race/events",
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"ok":true}`),
	}

	var wg sync.WaitGroup
	results := make(chan events.AppendResult, goroutines)
	for range goroutines {
		wg.Go(func() {
			result, err := st.Append(ctx, req)
			if err != nil {
				t.Errorf("Append() error = %v", err)
				return
			}
			results <- result
		})
	}
	wg.Wait()
	close(results)

	accepted := 0
	ids := make(map[events.RecordID]bool)
	for result := range results {
		if result.Outcome == events.AppendOutcomeAccepted {
			accepted++
		}
		ids[result.Record.ID] = true
	}
	if accepted != 1 {
		t.Fatalf("accepted count = %d, want exactly 1 across %d concurrent racers", accepted, goroutines)
	}
	if len(ids) != 1 {
		t.Fatalf("observed %d distinct Record.IDs, want exactly 1: every racer must resolve to the same accepted record", len(ids))
	}
}

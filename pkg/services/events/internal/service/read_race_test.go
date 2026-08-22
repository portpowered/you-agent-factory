package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// TestRead_ConcurrentIndependentReadersObserveContiguousHistory proves that
// many goroutines reading from independently chosen cursors, concurrently
// with ongoing appends, never observe a duplicated or skipped position and
// never see one reader's progress affect another's. Run with -race to
// additionally prove topicState's records/head are free of data races under
// concurrent Append+Read contention.
func TestRead_ConcurrentIndependentReadersObserveContiguousHistory(t *testing.T) {
	const totalAppends = 200
	const readers = 20

	st := New()
	ctx := context.Background()
	topic := events.Topic("chat-session/concurrent-read/events")

	var appendWG sync.WaitGroup
	appendWG.Go(func() {
		for i := 1; i <= totalAppends; i++ {
			req := events.AppendRequest{
				Topic:          topic,
				SourceType:     "worker.tool",
				SourceID:       "worker-1",
				SourceSequence: events.SourceSequence(i),
				SourceEventID:  events.SourceEventID(fmt.Sprintf("evt-%d", i)),
				SchemaID:       "worker.output.v1",
				Payload:        json.RawMessage(`{"ok":true}`),
			}
			if _, err := st.Append(ctx, req); err != nil {
				t.Errorf("Append() error = %v", err)
				return
			}
		}
	})

	var readerWG sync.WaitGroup
	errs := make(chan error, readers)
	for range readers {
		readerWG.Go(func() {
			var from events.AggregateSequence
			seen := 0
			for seen < totalAppends {
				result, err := st.Read(ctx, events.ReadRequest{
					Topic: topic,
					From:  events.Cursor{Topic: topic, Position: from},
					Limit: 7,
				})
				if err != nil {
					errs <- err
					return
				}
				if err := result.Validate(); err != nil {
					errs <- fmt.Errorf("Read().Validate() = %w", err)
					return
				}
				switch result.Outcome {
				case events.ReadOutcomeProgress:
					for _, rec := range result.Records {
						want := from + 1
						if rec.ID.Position != want {
							errs <- fmt.Errorf("expected contiguous position %d, got %d", want, rec.ID.Position)
							return
						}
						from = rec.ID.Position
						seen++
					}
				case events.ReadOutcomeAtHead:
					// nothing new yet; retry.
				default:
					errs <- fmt.Errorf("unexpected Outcome = %v", result.Outcome)
					return
				}
			}
		})
	}

	appendWG.Wait()
	readerWG.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("reader error: %v", err)
	}
}

package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// Read serves a bounded slice of req.Topic's aggregate ordering, starting
// after req.From. A canceled context or a malformed request is rejected
// before any observable state or log side effect: this Store never
// registers a topic in response to a rejected Read, and the terminal outcome
// log fires only once a well-formed request has actually been evaluated
// against topic state. Once Store.Close has taken effect, Read is rejected
// with an events.ErrOperationFailed-wrapped error instead (checked per-topic,
// so a topic created after Close observes the same rejection as one that
// existed before it), with the same no-log-on-rejection behavior.
//
// This Store treats every well-formed Topic as known and lazily materializes
// its (empty) state on first use, the same way Append does; it never returns
// ErrUnknownTopic or ErrUnresolvableCursor. A cursor naming a position ahead
// of the topic's live head is reported as the explicit, successful
// ReadOutcomeInvalidCursor rather than an operation-failure error, since a
// purely in-memory, single-instance store has no independent way to
// distinguish a stale/foreign cursor from any other not-yet-resolvable
// position.
func (st *Store) Read(ctx context.Context, req events.ReadRequest) (events.ReadResult, error) {
	if err := ctx.Err(); err != nil {
		return events.ReadResult{}, err
	}
	if err := req.Validate(); err != nil {
		return events.ReadResult{}, err
	}

	ts := st.topic(req.Topic)
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		return events.ReadResult{}, errClosed
	}
	result := ts.readLocked(req)
	ts.mu.Unlock()

	st.logReadOutcome(req, result)
	return result, nil
}

// readLocked evaluates req against ts's current aggregate ordering. Callers
// hold ts.mu.
func (ts *topicState) readLocked(req events.ReadRequest) events.ReadResult {
	topic := req.Topic
	from := req.From.Position
	head := ts.head
	earliest := ts.earliestLocked()

	switch {
	case from == head:
		return events.ReadResult{
			Outcome:  events.ReadOutcomeAtHead,
			Next:     events.Cursor{Topic: topic, Position: head},
			Retained: events.RetainedRange{Topic: topic, Earliest: earliest, Head: head},
		}
	case from > head:
		return events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}
	case earliest > 1 && from+1 < earliest:
		// from+1 == earliest is not a gap: that cursor asks to start reading
		// after from, so its first record is exactly the earliest retained
		// position, which is still available. Only a from that skips past
		// earliest (from+1 < earliest) has genuinely lost history.
		return events.ReadResult{
			Outcome: events.ReadOutcomeGap,
			Gap:     &events.GapFacts{Topic: topic, Requested: from, EarliestRetained: earliest, Head: head},
		}
	}

	startIndex := int(from + 1 - earliest)
	available := ts.records[startIndex:]
	if req.Limit > 0 && len(available) > req.Limit {
		available = available[:req.Limit]
	}

	records := make([]events.Record, len(available))
	for i, rec := range available {
		records[i] = rec.Detached()
	}
	last := records[len(records)-1]

	return events.ReadResult{
		Records:  records,
		Next:     events.Cursor{Topic: topic, Position: last.ID.Position},
		Retained: events.RetainedRange{Topic: topic, Earliest: earliest, Head: head},
		Outcome:  events.ReadOutcomeProgress,
	}
}

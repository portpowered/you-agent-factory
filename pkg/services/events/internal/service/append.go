package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// Append accepts req into its topic's aggregate ordering, assigning the next
// unique, contiguous position in commit order, or resolves to the original
// accepted Record when req repeats an already-accepted
// (sourceType, sourceID, sourceSequence, sourceEventID) identity on the same
// topic. A canceled context or a malformed request is rejected before any
// aggregate state changes and before the accepted/duplicate outcome log is
// emitted: the deferred log call only reports a real accepted or duplicate
// outcome when err is nil. Every returned Record owns detached payload
// bytes, so caller mutation of req.Payload after this call, or of one
// returned Record's Payload, can never alter the Store's retained copy or a
// later duplicate/read/delivery of the same record.
func (st *Store) Append(ctx context.Context, req events.AppendRequest) (result events.AppendResult, err error) {
	defer func() {
		st.logAppendOutcome(req, result, err)
	}()

	if err = ctx.Err(); err != nil {
		return events.AppendResult{}, err
	}
	if err = req.Validate(); err != nil {
		return events.AppendResult{}, err
	}

	detached := req.Detached()
	identity := detached.Identity()
	st.logAppendIntent(detached)

	ts := st.topic(detached.Topic)
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if existing, ok := ts.identity[identity]; ok {
		result = events.AppendResult{Record: existing.Detached(), Outcome: events.AppendOutcomeDuplicate}
		return result, nil
	}

	ts.head++
	stored := events.Record{
		ID:             events.RecordID{Topic: detached.Topic, Position: ts.head},
		SourceType:     detached.SourceType,
		SourceID:       detached.SourceID,
		SourceSequence: detached.SourceSequence,
		SourceEventID:  detached.SourceEventID,
		SchemaID:       detached.SchemaID,
		Payload:        detached.Payload,
	}.Detached()
	ts.records = append(ts.records, stored)
	ts.identity[identity] = stored
	if len(ts.records) > st.maxRetainedPerTopic {
		ts.records = ts.records[1:]
	}

	result = events.AppendResult{Record: stored.Detached(), Outcome: events.AppendOutcomeAccepted}
	return result, nil
}

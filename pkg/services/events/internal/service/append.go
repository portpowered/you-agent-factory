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
// outcome when err is nil. Once Store.Close has taken effect, Append is
// rejected with events.ErrClosed instead (checked per-topic, so a topic
// created after Close observes the same rejection as one that existed
// before it), also before any aggregate state change or intent log. Every
// returned Record owns detached payload bytes, so caller mutation of
// req.Payload after this call, or of one returned Record's Payload, can
// never alter the Store's retained copy or a later duplicate/read/delivery
// of the same record.
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

	ts := st.topic(detached.Topic)
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		err = events.ErrClosed
		return events.AppendResult{}, err
	}
	st.logAppendIntent(detached)
	stored, outcome := ts.commitLocked(st, detached.Topic, detached, identity)
	ts.mu.Unlock()

	// commitLocked's returned Record shares backing storage with ts's own
	// retained/identity copy (and, for a forwarding destination, with every
	// other attachment's view of the same commit): detach once more here so
	// a caller mutating this specific returned Record can never corrupt the
	// Store's canonical copy or another observer's independent view of it.
	result = events.AppendResult{Record: stored.Detached(), Outcome: outcome}
	return result, nil
}

// commitLocked stores req as the next accepted record for ts (assigning the
// next aggregate position), or resolves to the existing accepted Record when
// req's identity was already accepted. It evicts the oldest retained record
// once the topic exceeds the store's retention cap, pruning that evicted
// record's entry from ts.identity in the same step so idempotency state
// stays bounded by the same explicit retention policy as the retained
// records themselves: once a record's position has been evicted, repeating
// its identity is accepted as a new record rather than resolved as a
// duplicate. It then notifies both live subscribers and any topics attached
// to ts as a forwarding destination under this same lock, so no observer can
// ever see one commit before another and no aggregate position is skipped or
// duplicated across a retained-then-live or attachment forwarding handoff.
// Callers hold ts.mu; req must already carry a detached Payload (see
// AppendRequest.Detached and Record.Detached), since commitLocked does not
// clone it a second time.
func (ts *topicState) commitLocked(st *Store, topic events.Topic, req events.AppendRequest, identity events.AppendIdentity) (events.Record, events.AppendOutcome) {
	if existing, ok := ts.identity[identity]; ok {
		return existing.Detached(), events.AppendOutcomeDuplicate
	}

	ts.head++
	stored := events.Record{
		ID:             events.RecordID{Topic: topic, Position: ts.head},
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
		SchemaID:       req.SchemaID,
		Payload:        req.Payload,
	}.Detached()
	ts.records = append(ts.records, stored)
	ts.identity[identity] = stored
	if len(ts.records) > st.maxRetainedPerTopic {
		evicted := ts.records[0]
		ts.records = ts.records[1:]
		delete(ts.identity, evicted.Identity())
	}
	ts.notifySubscribersLocked(st, topic, stored)
	ts.notifyAttachmentsLocked(st, stored)

	return stored, events.AppendOutcomeAccepted
}

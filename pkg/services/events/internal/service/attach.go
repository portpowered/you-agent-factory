package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// attachmentForward is one active forwarding registration from a topic (as
// an attachment source) into destination's aggregate ordering. startAt
// records the resolved starting cursor AttachSource reported when the
// attachment was first accepted, so a later idempotent AttachSource call for
// the same (Destination, Source) pair returns that original value rather
// than whatever the repeated call's own request happened to ask for.
type attachmentForward struct {
	startAt events.Cursor
}

// AttachSource registers req.Destination to receive req.Source's records
// from req.StartAt onward (AttachModeRetainedThenLive) or from the source's
// current live head onward (AttachModeLiveOnly, which never replays retained
// history). A canceled context, a malformed/self/incompatible-cursor
// request, a StartAt naming a not-yet-existing position beyond the source's
// live head, or an already-closed source topic is rejected before any
// attachment is registered or record forwarded. An equivalent
// (Destination, Source) attachment already registered is idempotent: it
// returns the original AttachmentID, Outcome, and StartAt without creating a
// second forwarding path or re-forwarding anything.
//
// Once accepted, AttachSource itself synchronously forwards every currently
// retained source record after StartAt (RetainedThenLive only) before
// returning, and registers the destination so every later source commit is
// forwarded the moment it commits -- both phases run under the source
// topic's own lock, so no source commit can ever be observed out of order
// or missed across the retained-to-live handoff. A StartAt naming a position
// the source topic has already evicted is not itself a rejection (unlike a
// StartAt beyond the live head): AttachSource reuses the same retained-range
// resolution Read/Subscribe use, reports the resulting GapFacts once via a
// safe structured log, and resumes forwarding from the earliest still
// retained record, mirroring Subscribe's own gap recovery rather than
// Read's stricter invalid-cursor outcome, since there is no per-attachment
// consumer to hand a Gap outcome to the way Subscribe's Delivery does.
//
// Forwarded records preserve the source record's identity, schema, and
// detached payload; only the destination's own aggregate position differs.
// A destination is itself eligible to be a further attachment source, so
// forwarding chains recurse; AttachSource only rejects the direct
// Destination == Source case, not a longer forwarding cycle, so attaching
// topics in a cycle can deadlock and is out of this Store's supported usage.
func (st *Store) AttachSource(ctx context.Context, req events.AttachSourceRequest) (result events.AttachSourceResult, err error) {
	defer func() {
		st.logAttachOutcome(req, result, err)
	}()

	if err = ctx.Err(); err != nil {
		return events.AttachSourceResult{}, err
	}
	if err = req.Validate(); err != nil {
		return events.AttachSourceResult{}, err
	}
	st.logAttachIntent(req)

	id := events.AttachmentID{Destination: req.Destination, Source: req.Source}
	srcTS := st.topic(req.Source)

	srcTS.mu.Lock()
	if existing, ok := srcTS.attachments[req.Destination]; ok {
		srcTS.mu.Unlock()
		result = events.AttachSourceResult{ID: id, Outcome: events.AttachOutcomeAlreadyAttached, StartAt: existing.startAt}
		return result, nil
	}
	if srcTS.closed {
		srcTS.mu.Unlock()
		err = events.ErrOperationFailed
		return events.AttachSourceResult{}, err
	}

	var catchup []events.Record
	var gap *events.GapFacts
	var startAt events.Cursor
	switch req.Mode {
	case events.AttachModeLiveOnly:
		startAt = events.Cursor{Topic: req.Source, Position: srcTS.head}
	default: // events.AttachModeRetainedThenLive; req.Validate already rejected any other mode
		if req.StartAt.Position > srcTS.head {
			srcTS.mu.Unlock()
			err = events.ErrUnresolvableCursor
			return events.AttachSourceResult{}, err
		}
		catchup, gap = srcTS.catchupLocked(req.Source, req.StartAt.Position)
		startAt = req.StartAt
	}

	for _, rec := range catchup {
		st.forwardRecord(req.Destination, rec)
	}
	srcTS.attachments[req.Destination] = &attachmentForward{startAt: startAt}
	srcTS.mu.Unlock()

	if gap != nil {
		st.logAttachGap(req, gap)
	}
	result = events.AttachSourceResult{ID: id, Outcome: events.AttachOutcomeAccepted, StartAt: startAt}
	return result, nil
}

// forwardRecord commits rec (already accepted on some other topic) into
// destination's own aggregate ordering, preserving its source identity,
// schema, and payload while letting destination assign its own aggregate
// position, or resolving to the already-forwarded record via destination's
// own identity index if this exact (sourceType, sourceID, sourceSequence,
// sourceEventID) tuple already reached it. Callers may already hold another
// topic's lock; forwardRecord acquires only destination's own lock, and (via
// commitLocked's own notifyAttachmentsLocked call) recurses into any further
// topics attached to destination, so a multi-hop forwarding chain commits
// and delivers in one uninterrupted call chain.
func (st *Store) forwardRecord(destination events.Topic, rec events.Record) events.Record {
	destTS := st.topic(destination)
	destTS.mu.Lock()
	stored, _ := destTS.commitLocked(st, destination, events.AppendRequest{
		Topic:          destination,
		SourceType:     rec.SourceType,
		SourceID:       rec.SourceID,
		SourceSequence: rec.SourceSequence,
		SourceEventID:  rec.SourceEventID,
		SchemaID:       rec.SchemaID,
		Payload:        rec.Payload,
	}, rec.Identity())
	destTS.mu.Unlock()
	return stored
}

// notifyAttachmentsLocked forwards rec (the record ts just committed) to
// every topic currently attached to ts as a destination. Callers hold ts.mu;
// forwardRecord takes only each destination's own lock, never ts's, so this
// never contends with itself across distinct destinations.
func (ts *topicState) notifyAttachmentsLocked(st *Store, rec events.Record) {
	for dest := range ts.attachments {
		st.forwardRecord(dest, rec)
	}
}

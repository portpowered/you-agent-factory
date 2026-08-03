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
// the source topic has already evicted is rejected with ErrUnresolvableCursor
// before any record is forwarded or attachment registered, exactly like a
// StartAt beyond the live head: unlike Subscribe (which has a per-consumer
// Delivery to report a recovered Gap through), AttachSource has no outcome
// vocabulary for "some requested history was silently lost," so it never
// forwards a partial backlog.
//
// Forwarded records preserve the source record's identity, schema, and
// detached payload; only the destination's own aggregate position differs.
// A destination is itself eligible to be a further attachment source, so
// forwarding chains recurse; AttachSource rejects both the direct
// Destination == Source case (via req.Validate) and any longer forwarding
// cycle (Source indirectly reachable from Destination through already
// registered attachments) with ErrSelfAttachment before registering
// anything, so no accepted attachment graph can ever deadlock Append's
// nested destination-lock forwarding.
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

	// attachMu serializes the whole call against every other AttachSource,
	// so the reachability check below and the registration it guards form
	// one atomic step against the attachment graph: no concurrent AttachSource
	// call can register a complementary edge that only individually looked
	// acyclic.
	st.attachMu.Lock()
	defer st.attachMu.Unlock()

	if st.reachable(req.Destination, req.Source) {
		err = events.ErrSelfAttachment
		return events.AttachSourceResult{}, err
	}

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
		// req.StartAt.Position+1 == earliest is not evicted: that cursor's
		// first forwarded record is exactly the still-retained earliest
		// position (see topicState.readLocked's identical boundary). Only a
		// StartAt that skips past earliest has genuinely lost history.
		if earliest := srcTS.earliestLocked(); earliest > 1 && req.StartAt.Position+1 < earliest {
			srcTS.mu.Unlock()
			err = events.ErrUnresolvableCursor
			return events.AttachSourceResult{}, err
		}
		catchup, _ = srcTS.catchupLocked(req.Source, req.StartAt.Position)
		startAt = req.StartAt
	}

	for _, rec := range catchup {
		st.forwardRecord(req.Destination, rec)
	}
	srcTS.attachments[req.Destination] = &attachmentForward{startAt: startAt}
	srcTS.mu.Unlock()

	result = events.AttachSourceResult{ID: id, Outcome: events.AttachOutcomeAccepted, StartAt: startAt}
	return result, nil
}

// reachable reports whether target is reachable from start by following
// currently registered attachment edges (start -> ... -> target) forward.
// It locks at most one topic at a time -- never two simultaneously -- so the
// traversal itself can never deadlock against Append's nested
// source-then-destination forwarding locks; callers hold st.attachMu so the
// attachment graph cannot change out from under the traversal.
func (st *Store) reachable(start, target events.Topic) bool {
	visited := map[events.Topic]bool{start: true}
	queue := []events.Topic{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		ts := st.topic(cur)
		ts.mu.Lock()
		next := make([]events.Topic, 0, len(ts.attachments))
		for dest := range ts.attachments {
			next = append(next, dest)
		}
		ts.mu.Unlock()

		for _, dest := range next {
			if dest == target {
				return true
			}
			if !visited[dest] {
				visited[dest] = true
				queue = append(queue, dest)
			}
		}
	}
	return false
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

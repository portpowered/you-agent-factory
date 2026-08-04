package service

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// sourceKey narrows the four-part Events idempotency identity down to the
// (SourceType, SourceID) pair PublishRecord tracks ordering against: every
// record sharing one sourceKey is one source's own observation stream, and
// must commit in non-decreasing SourceSequence order within it.
type sourceKey struct {
	sourceType events.SourceType
	sourceID   events.SourceID
}

// publication is one Worker Session's own publication window: the
// mutual-exclusion boundary that makes "opening record, then every accepted
// source-native record in source order, then exactly one terminal record"
// hold even under concurrent PublishRecord callers and a racing terminal
// append. mu serializes every record this session ever commits through
// appendDraft -- publishOpeningRecord, PublishRecord, and
// publishTerminalRecord alike -- so at most one commit for this session is
// ever in flight at a time, and the two lifecycle boundaries (open, then
// closed) are observed by every publish attempt under the same lock that
// guards them. lastSequence and open are owned entirely by that lock; a
// publication is never read or written without holding mu.
type publication struct {
	mu sync.Mutex
	// open is true only between a successfully committed opening record and
	// the start of the terminal-record commit attempt. PublishRecord rejects
	// every call observed while open is false, whether that is because the
	// session was only ever Reserved, its opening record has not yet
	// committed, or its terminal record has already started committing.
	open bool
	// lastSequence is the highest SourceSequence already accepted for each
	// (SourceType, SourceID) this session has published, used to reject a
	// record whose SourceSequence regresses behind one already committed.
	lastSequence map[sourceKey]events.SourceSequence
	// accepted holds every full Events idempotency identity this session has
	// already committed. It lets an exact retry of a previously accepted
	// identity reach Events (and resolve to the original record as a
	// duplicate) even after a later SourceSequence has advanced lastSequence
	// past it -- Events itself retains identities permanently for dedup, so a
	// retry must stay idempotent regardless of publication order since. Only
	// an identity never accepted before is subject to the out-of-order
	// rejection below.
	accepted map[events.AppendIdentity]struct{}
}

// publicationFor returns the publication registered for id, or nil if id was
// never reserved. The returned pointer is stable for id's lifetime: callers
// serialize on its own mu rather than r.mu, so looking it up is a brief,
// independent critical section.
func (r *registry) publicationFor(id string) *publication {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.publications[id]
}

// PublishRecord validates req, then appends req.Draft, detached, as a
// source-native Worker record onto workersessions.Topic(req.SessionID) using
// req's complete Events idempotency identity, through the same appendDraft
// helper publishOpeningRecord uses. PublishRecord requires an established
// publication window: req.SessionID must have committed its opening record
// and must not yet have started committing its terminal record, and it
// enforces that accepted records for one (SourceType, SourceID) commit in
// non-decreasing SourceSequence order, rejecting a call that would regress
// behind one already accepted -- unless the call's full identity was itself
// already accepted, in which case it always reaches Events and resolves to
// the original record as a duplicate. Beyond that ordering and window
// enforcement, PublishRecord relies on Events for aggregate order, duplicate resolution,
// cursors, reads, and subscriptions. An invalid Draft, an unopened or closed
// publication window, an out-of-order SourceSequence, a malformed Events
// identity, or a rejected Events append is returned unchanged, and no record
// is committed.
func (r *registry) PublishRecord(ctx context.Context, req workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "invalid")
		return workersessions.PublishRecordResult{}, err
	}

	pub := r.publicationFor(req.SessionID)
	if pub == nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "not_found")
		return workersessions.PublishRecordResult{}, workersessions.ErrSessionNotFound
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()

	if !pub.open {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "publication_not_open")
		return workersessions.PublishRecordResult{}, workersessions.ErrPublicationNotOpen
	}

	key := sourceKey{sourceType: req.SourceType, sourceID: req.SourceID}
	identity := events.AppendIdentity{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	_, alreadyAccepted := pub.accepted[identity]
	if last := pub.lastSequence[key]; !alreadyAccepted && req.SourceSequence < last {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "out_of_order")
		return workersessions.PublishRecordResult{}, workersessions.ErrOutOfOrderPublication
	}

	appendResult, err := r.appendDraft(ctx, workersessions.Topic(req.SessionID), identity, req.SchemaID, req.Draft)
	if err != nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "append_failed")
		return workersessions.PublishRecordResult{}, err
	}
	pub.accepted[identity] = struct{}{}
	if req.SourceSequence > pub.lastSequence[key] {
		pub.lastSequence[key] = req.SourceSequence
	}

	outcome := workersessions.PublishOutcomeAccepted
	if appendResult.Outcome == events.AppendOutcomeDuplicate {
		outcome = workersessions.PublishOutcomeDuplicate
	}
	r.logger.Info(
		"worker session publish record",
		"sessionID", req.SessionID,
		"outcome", publishOutcomeLabel(outcome),
		"aggregate_sequence", uint64(appendResult.Record.ID.Position),
	)
	return workersessions.PublishRecordResult{
		SessionID:         req.SessionID,
		AggregateSequence: appendResult.Record.ID.Position,
		Outcome:           outcome,
	}, nil
}

func publishOutcomeLabel(outcome workersessions.PublishOutcome) string {
	switch outcome {
	case workersessions.PublishOutcomeAccepted:
		return "accepted"
	case workersessions.PublishOutcomeDuplicate:
		return "duplicate"
	default:
		return "unspecified"
	}
}

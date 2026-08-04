package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// PublishRecord validates req, then appends req.Draft, detached, as a
// source-native Worker record onto workersessions.Topic(req.SessionID) using
// req's complete Events idempotency identity, through the same appendDraft
// helper publishOpeningRecord uses. PublishRecord performs no registry
// lookup or mutation: it neither requires nor reads r.sessions, since
// Topic's naming is a pure function of req.SessionID and Events alone owns
// aggregate order, duplicate resolution, cursors, reads, and subscriptions
// for the resulting record. An invalid Draft, a malformed Events identity,
// or a rejected Events append is returned unchanged, and no record is
// committed.
func (r *registry) PublishRecord(ctx context.Context, req workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "invalid")
		return workersessions.PublishRecordResult{}, err
	}

	identity := events.AppendIdentity{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	appendResult, err := r.appendDraft(ctx, workersessions.Topic(req.SessionID), identity, req.SchemaID, req.Draft)
	if err != nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "append_failed")
		return workersessions.PublishRecordResult{}, err
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

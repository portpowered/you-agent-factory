package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// AdvanceStreamHead advances req.SessionID's StreamHead to
// req.AggregateSequence under an optimistic-version guard. When StreamHead
// already stands at or beyond req.AggregateSequence -- because this is a
// retry of an advancement that already committed, or a stale call for a
// position an intervening advancement already passed -- AdvanceStreamHead
// reconciles idempotently: it reports AdvanceStreamHeadOutcomeAlreadyCurrent
// and leaves the stored session, including its Version, byte-for-byte
// unchanged, without consulting ExpectedVersion at all. This mirrors
// BindFactorySession's "already at target value" precedent: an
// already-satisfied request converges on the committed result instead of
// depending on a version that may have already moved past ExpectedVersion
// precisely because the prior call succeeded. Only when the position is
// genuinely new does AdvanceStreamHead consult ExpectedVersion, reporting
// *ConflictError on a stale mismatch and exposing no partially committed
// head update in that case: the session is either left completely unchanged
// (conflict) or fully updated (StreamHead, Version, UpdatedAt together),
// never one without the others. A successful advancement mutates only those
// three fields, leaving SelectedTarget, TargetEpisode, ActiveTurnID, and
// every attachment/control/episode record untouched.
func (s *Store) AdvanceStreamHead(_ context.Context, req chatsessions.AdvanceStreamHeadRequest) (result chatsessions.AdvanceStreamHeadResult, err error) {
	s.logStart("AdvanceStreamHead", req.SessionID)
	defer func() {
		s.logOutcome("AdvanceStreamHead", req.SessionID, err,
			"source_type", string(req.SourceType),
			"source_id", string(req.SourceID),
			"source_sequence", uint64(req.SourceSequence),
			"source_event_id", string(req.SourceEventID),
			"aggregate_sequence", uint64(req.AggregateSequence),
			"version", result.Session.Version,
			"outcome", advanceStreamHeadOutcomeLabel(result.Outcome))
	}()

	if err := req.Validate(); err != nil {
		return chatsessions.AdvanceStreamHeadResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.AdvanceStreamHeadResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}

	if record.session.StreamHead >= uint64(req.AggregateSequence) {
		return chatsessions.AdvanceStreamHeadResult{
			Session: record.session,
			Outcome: chatsessions.AdvanceStreamHeadOutcomeAlreadyCurrent,
		}, nil
	}

	if req.ExpectedVersion != record.session.Version {
		return chatsessions.AdvanceStreamHeadResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}

	updated := record.session
	updated.StreamHead = uint64(req.AggregateSequence)
	updated.Version++
	updated.UpdatedAt = s.now()
	if err := updated.Validate(); err != nil {
		return chatsessions.AdvanceStreamHeadResult{}, err
	}

	record.session = updated
	s.sessions[req.SessionID] = record

	return chatsessions.AdvanceStreamHeadResult{
		Session: updated,
		Outcome: chatsessions.AdvanceStreamHeadOutcomeAdvanced,
	}, nil
}

func advanceStreamHeadOutcomeLabel(outcome chatsessions.AdvanceStreamHeadOutcome) string {
	switch outcome {
	case chatsessions.AdvanceStreamHeadOutcomeAdvanced:
		return "advanced"
	case chatsessions.AdvanceStreamHeadOutcomeAlreadyCurrent:
		return "already_current"
	default:
		return "unspecified"
	}
}

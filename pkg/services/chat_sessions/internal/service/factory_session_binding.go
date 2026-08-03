package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// BindFactorySession commits req.FactorySessionID onto req.SessionID's
// TargetEpisode numbered req.Episode, but only when req.SessionID exists and
// req.ExpectedVersion still matches the session's current version. Because
// every mutation that could move the session's active turn or target episode
// (AdvanceTurn releasing the active turn, a later StartTurn, or SetTarget
// opening a new episode) also advances Session.Version, a matching
// ExpectedVersion already guarantees req.TurnID is still the session's active
// turn and req.Episode is still its open episode; the explicit turn/episode
// lookups below are defense-in-depth, not a second independent guard, and
// report the same *ConflictError a version mismatch would.
//
// A repeat call carrying the exact FactorySessionID the episode already
// carries is idempotent and succeeds without mutation regardless of
// ExpectedVersion, so a retried or concurrently-raced binding attempt for the
// identity that already won converges on that one committed value instead of
// failing. A call whose episode already carries a *different*
// FactorySessionID reports *chatsessions.FactorySessionConflictError and
// never overwrites the committed identity. In every failure case the stored
// session and episode history are left byte-for-byte unchanged, and this
// method only ever mutates the current (last) episode, so prior episode
// history is never touched.
func (s *Store) BindFactorySession(_ context.Context, req chatsessions.BindFactorySessionRequest) (result chatsessions.BindFactorySessionResult, err error) {
	s.logStart("BindFactorySession", req.SessionID)
	defer func() {
		s.logOutcome("BindFactorySession", req.SessionID, err, "version", result.Session.Version, "target_episode", req.Episode)
	}()
	if req.FactorySessionID == "" {
		return chatsessions.BindFactorySessionResult{}, &chatsessions.ValidationError{
			Value: "BindFactorySessionRequest", Field: "FactorySessionID", Err: chatsessions.ErrRequiredValue,
		}
	}
	if req.TurnID == "" {
		return chatsessions.BindFactorySessionResult{}, &chatsessions.ValidationError{
			Value: "BindFactorySessionRequest", Field: "TurnID", Err: chatsessions.ErrRequiredValue,
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.BindFactorySessionResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}

	idx := len(record.episodes) - 1
	current := record.episodes[idx]
	if current.Number == req.Episode && current.FactorySessionID == req.FactorySessionID {
		return chatsessions.BindFactorySessionResult{Session: record.session}, nil
	}
	if current.Number == req.Episode && current.FactorySessionID != "" {
		return chatsessions.BindFactorySessionResult{}, &chatsessions.FactorySessionConflictError{
			SessionID: req.SessionID, Episode: req.Episode,
			Bound: current.FactorySessionID, Attempted: req.FactorySessionID,
		}
	}

	if req.ExpectedVersion != record.session.Version {
		return chatsessions.BindFactorySessionResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}
	if record.session.ActiveTurnID != req.TurnID {
		return chatsessions.BindFactorySessionResult{}, &chatsessions.ConflictError{
			Value: "Turn", ID: req.TurnID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}
	turn, ok := record.turns[req.TurnID]
	if !ok || turn.Episode != req.Episode || current.Number != req.Episode {
		return chatsessions.BindFactorySessionResult{}, &chatsessions.ConflictError{
			Value: "TargetEpisode", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}

	updatedEpisode := current
	updatedEpisode.FactorySessionID = req.FactorySessionID
	updatedEpisode.PendingFactorySessionID = ""
	if err := updatedEpisode.Validate(); err != nil {
		return chatsessions.BindFactorySessionResult{}, err
	}

	updatedSession := record.session
	updatedSession.Version++
	updatedSession.UpdatedAt = s.now()
	if err := updatedSession.Validate(); err != nil {
		return chatsessions.BindFactorySessionResult{}, err
	}

	record.episodes[idx] = updatedEpisode
	record.session = updatedSession
	s.sessions[req.SessionID] = record

	return chatsessions.BindFactorySessionResult{Session: updatedSession}, nil
}

// RecordPendingFactorySession records (or, with a blank FactorySessionID,
// clears) the current episode's PendingFactorySessionID under the same
// session/episode/turn/version guard BindFactorySession uses, but never
// advances Session.Version: this is incidental reconciliation bookkeeping a
// retry can observe through the episode snapshot, not a state transition
// other guarded callers need to see as invalidating their own
// ExpectedVersion. A repeat call carrying the exact value already recorded is
// a no-op success.
func (s *Store) RecordPendingFactorySession(_ context.Context, req chatsessions.RecordPendingFactorySessionRequest) (result chatsessions.RecordPendingFactorySessionResult, err error) {
	s.logStart("RecordPendingFactorySession", req.SessionID)
	defer func() {
		s.logOutcome("RecordPendingFactorySession", req.SessionID, err, "version", result.Session.Version, "target_episode", req.Episode)
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.RecordPendingFactorySessionResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}

	idx := len(record.episodes) - 1
	current := record.episodes[idx]
	if current.Number == req.Episode && current.PendingFactorySessionID == req.FactorySessionID {
		return chatsessions.RecordPendingFactorySessionResult{Session: record.session}, nil
	}

	if req.ExpectedVersion != record.session.Version {
		return chatsessions.RecordPendingFactorySessionResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}
	if record.session.ActiveTurnID != req.TurnID {
		return chatsessions.RecordPendingFactorySessionResult{}, &chatsessions.ConflictError{
			Value: "Turn", ID: req.TurnID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}
	turn, ok := record.turns[req.TurnID]
	if !ok || turn.Episode != req.Episode || current.Number != req.Episode {
		return chatsessions.RecordPendingFactorySessionResult{}, &chatsessions.ConflictError{
			Value: "TargetEpisode", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}

	updatedEpisode := current
	updatedEpisode.PendingFactorySessionID = req.FactorySessionID
	if err := updatedEpisode.Validate(); err != nil {
		return chatsessions.RecordPendingFactorySessionResult{}, err
	}

	record.episodes[idx] = updatedEpisode
	s.sessions[req.SessionID] = record

	return chatsessions.RecordPendingFactorySessionResult{Session: record.session}, nil
}

package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// StartTurn admits a new Turn against the session's current target episode
// under an optimistic-version guard. It reports *NotFoundError for an
// unknown SessionID, *ConflictError when ExpectedVersion no longer matches
// the session's current version (checked before the busy check, matching
// SetTarget's ordering), and *BusyError while a non-terminal turn is already
// active -- in every failure case no turn is created and the stored session
// and turn state are left byte-for-byte unchanged. On success it moves a
// CREATED session to ACTIVE on its first turn and leaves an already-ACTIVE
// session's State unchanged.
func (s *Store) StartTurn(_ context.Context, req chatsessions.StartTurnRequest) (result chatsessions.StartTurnResult, err error) {
	s.logStart("StartTurn", req.SessionID)
	defer func() {
		s.logOutcome("StartTurn", req.SessionID, err, "version", result.Session.Version, "turn_id", result.Turn.ID)
	}()
	if err := req.RequestID.Validate(); err != nil {
		return chatsessions.StartTurnResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.StartTurnResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	if req.ExpectedVersion != record.session.Version {
		return chatsessions.StartTurnResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}
	if active, ok := record.activeTurnValue(); ok {
		return chatsessions.StartTurnResult{}, &chatsessions.BusyError{
			Value: "Session", ID: req.SessionID,
			ActiveTurnID: active.ID, ActiveTurnState: active.State,
		}
	}

	turn := chatsessions.Turn{
		ID:        s.newID(),
		Episode:   record.session.TargetEpisode,
		State:     chatsessions.TurnStateAdmitted,
		RequestID: req.RequestID,
	}
	if err := turn.Validate(); err != nil {
		return chatsessions.StartTurnResult{}, err
	}

	updated := record.session
	if updated.State == chatsessions.SessionStateCreated {
		if err := updated.State.CanTransitionTo(chatsessions.SessionStateActive); err != nil {
			return chatsessions.StartTurnResult{}, err
		}
		updated.State = chatsessions.SessionStateActive
	}
	updated.ActiveTurnID = turn.ID
	updated.Version++
	updated.UpdatedAt = s.now()
	if err := updated.Validate(); err != nil {
		return chatsessions.StartTurnResult{}, err
	}

	record.turns[turn.ID] = turn
	record.lastTurnID = turn.ID
	record.session = updated
	s.sessions[req.SessionID] = record

	return chatsessions.StartTurnResult{Session: updated, Turn: turn}, nil
}

// AdvanceTurn moves one Turn to Next, enforcing the TurnState transition
// table. It reports *NotFoundError when SessionID or TurnID does not
// identify an existing turn and the turn's own *TransitionError when Next is
// not a legal transition from its current state; on either failure the
// stored turn and session are left byte-for-byte unchanged. Reaching a
// terminal Next records that terminal state on the turn (assigning it a
// distinct, non-zero TerminalSequence from the session's private turn
// sequence counter), clears the session's ActiveTurnID, and advances the
// session's version so later guarded callers observe the release.
func (s *Store) AdvanceTurn(_ context.Context, req chatsessions.AdvanceTurnRequest) (result chatsessions.AdvanceTurnResult, err error) {
	s.logStart("AdvanceTurn", req.SessionID)
	defer func() {
		s.logOutcome("AdvanceTurn", req.SessionID, err, "turn_id", result.Turn.ID, "state", string(result.Turn.State))
	}()
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.AdvanceTurnResult{}, &chatsessions.NotFoundError{Value: "Turn", ID: req.TurnID}
	}
	turn, ok := record.turns[req.TurnID]
	if !ok {
		return chatsessions.AdvanceTurnResult{}, &chatsessions.NotFoundError{Value: "Turn", ID: req.TurnID}
	}
	if err := turn.State.CanTransitionTo(req.Next); err != nil {
		return chatsessions.AdvanceTurnResult{}, err
	}

	updated := turn
	updated.State = req.Next
	nextTurnSequence := record.turnSequence
	if req.Next.IsTerminal() {
		nextTurnSequence++
		updated.TerminalSequence = nextTurnSequence
	}
	if err := updated.Validate(); err != nil {
		return chatsessions.AdvanceTurnResult{}, err
	}

	updatedSession := record.session
	releasesSession := req.Next.IsTerminal() && record.session.ActiveTurnID == req.TurnID
	if releasesSession {
		updatedSession.ActiveTurnID = ""
		updatedSession.Version++
		updatedSession.UpdatedAt = s.now()
		if err := updatedSession.Validate(); err != nil {
			return chatsessions.AdvanceTurnResult{}, err
		}
	}

	record.turns[req.TurnID] = updated
	record.turnSequence = nextTurnSequence
	if releasesSession {
		record.session = updatedSession
	}
	s.sessions[req.SessionID] = record

	return chatsessions.AdvanceTurnResult{Turn: updated}, nil
}

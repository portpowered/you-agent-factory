package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// SetTarget changes a session's selected target under an optimistic-version
// guard, closing the currently open TargetEpisode and opening the next
// consecutively numbered one at the new target. It reports *NotFoundError
// for an unknown SessionID, *ConflictError when ExpectedVersion no longer
// matches the session's current version, and *BusyError while a non-terminal
// turn is active -- in every failure case, no episode is created and the
// stored session and episode history are left byte-for-byte unchanged.
func (s *Store) SetTarget(_ context.Context, req chatsessions.SetTargetRequest) (result chatsessions.SetTargetResult, err error) {
	s.logStart("SetTarget", req.SessionID)
	defer func() {
		s.logOutcome("SetTarget", req.SessionID, err, "version", result.Session.Version, "target_episode", result.Session.TargetEpisode)
	}()
	if err := req.RequestID.Validate(); err != nil {
		return chatsessions.SetTargetResult{}, err
	}
	if err := req.Target.Validate(); err != nil {
		return chatsessions.SetTargetResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.SetTargetResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	if req.ExpectedVersion != record.session.Version {
		return chatsessions.SetTargetResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}
	if record.activeTurn != nil {
		return chatsessions.SetTargetResult{}, &chatsessions.BusyError{
			Value: "Session", ID: req.SessionID,
			ActiveTurnID: record.activeTurn.ID, ActiveTurnState: record.activeTurn.State,
		}
	}

	now := s.now()
	priorIdx := len(record.episodes) - 1
	closed, err := chatsessions.CloseTargetEpisode(record.episodes[priorIdx], now)
	if err != nil {
		return chatsessions.SetTargetResult{}, err
	}
	next, err := chatsessions.OpenNextTargetEpisode(closed, req.Target, "", now)
	if err != nil {
		return chatsessions.SetTargetResult{}, err
	}

	updated := record.session
	updated.SelectedTarget = req.Target
	updated.TargetEpisode = next.Number
	updated.Version++
	updated.UpdatedAt = now
	if err := updated.Validate(); err != nil {
		return chatsessions.SetTargetResult{}, err
	}

	record.episodes[priorIdx] = closed
	record.episodes = append(record.episodes, next)
	record.session = updated
	s.sessions[req.SessionID] = record

	return chatsessions.SetTargetResult{Session: updated}, nil
}

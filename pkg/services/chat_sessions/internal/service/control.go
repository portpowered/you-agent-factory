package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// RequestControl atomically captures the session's current active turn,
// target episode, and expected version as a new REQUESTED ControlIntent
// under an optimistic-version guard. It reports *ValidationError wrapping
// ErrUnsupportedControlAction for a declared-but-not-yet-executable Action,
// *NotFoundError when there is no active turn to target, and *ConflictError
// when ExpectedVersion no longer matches the session's current version -- in
// every failure case no control intent is created and the stored session and
// control state are left byte-for-byte unchanged. RequestID is stored as the
// intent's own map key, so structurally distinct identities (differing
// ConnectionID, Kind, or a bare TransportUUID) can never retrieve, advance,
// overwrite, or deduplicate one another even when their JSON-RPC id tokens
// happen to match.
func (s *Store) RequestControl(_ context.Context, req chatsessions.RequestControlRequest) (chatsessions.RequestControlResult, error) {
	if err := req.RequestID.Validate(); err != nil {
		return chatsessions.RequestControlResult{}, err
	}
	if err := req.Action.Validate(); err != nil {
		return chatsessions.RequestControlResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.RequestControlResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	if record.activeTurn == nil {
		return chatsessions.RequestControlResult{}, &chatsessions.NotFoundError{Value: "Turn", ID: ""}
	}
	if req.ExpectedVersion != record.session.Version {
		return chatsessions.RequestControlResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}

	intent := chatsessions.ControlIntent{
		RequestID:       req.RequestID,
		SessionID:       req.SessionID,
		TurnID:          record.activeTurn.ID,
		TargetEpisode:   record.session.TargetEpisode,
		ExpectedVersion: req.ExpectedVersion,
		Action:          req.Action,
		State:           chatsessions.ControlIntentStateRequested,
		RequestedAt:     s.now(),
	}
	if err := intent.Validate(); err != nil {
		return chatsessions.RequestControlResult{}, err
	}

	record.controls[req.RequestID] = intent
	s.sessions[req.SessionID] = record

	return chatsessions.RequestControlResult{Intent: intent}, nil
}

// AdvanceControl moves one ControlIntent to Next, enforcing the
// ControlIntentState transition table. It reports *NotFoundError when the
// intent identified by SessionID and RequestID does not exist and
// *TransitionError when Next is not a legal transition from the intent's
// current state; on either failure the stored intent is left byte-for-byte
// unchanged. Once an intent is COMMITTED, its terminal outcome is always
// resolved with chatsessions.ResolveControlIntentOutcome against the
// intent's own captured TurnID and the session's live active turn --
// caller-supplied Next is never consulted for that resolution -- so a
// delayed advancement can never retarget a captured intent to a later turn.
func (s *Store) AdvanceControl(_ context.Context, req chatsessions.AdvanceControlRequest) (chatsessions.AdvanceControlResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.AdvanceControlResult{}, &chatsessions.NotFoundError{Value: "ControlIntent", ID: req.SessionID}
	}
	intent, ok := record.controls[req.RequestID]
	if !ok {
		return chatsessions.AdvanceControlResult{}, &chatsessions.NotFoundError{Value: "ControlIntent", ID: req.SessionID}
	}

	next := req.Next
	if intent.State == chatsessions.ControlIntentStateCommitted {
		capturedTurn := record.turns[intent.TurnID]
		outcome, err := chatsessions.ResolveControlIntentOutcome(intent.TurnID, capturedTurn.State, record.session.ActiveTurnID)
		if err != nil {
			return chatsessions.AdvanceControlResult{}, err
		}
		next = outcome
	}
	if err := intent.State.CanTransitionTo(next); err != nil {
		return chatsessions.AdvanceControlResult{}, err
	}

	updated := intent
	updated.State = next
	if err := updated.Validate(); err != nil {
		return chatsessions.AdvanceControlResult{}, err
	}

	record.controls[req.RequestID] = updated
	s.sessions[req.SessionID] = record

	return chatsessions.AdvanceControlResult{Intent: updated}, nil
}

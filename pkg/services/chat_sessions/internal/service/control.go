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
// happen to match. Reusing an identity that already identifies a requested
// intent is treated as an idempotent retry: the existing intent is returned
// unchanged rather than recapturing the (possibly now different) active turn,
// target episode, or version -- an exact identity can never overwrite or
// retarget an already-captured, immutable intent, including one requested
// against an earlier turn that has since terminated and been replaced.
func (s *Store) RequestControl(_ context.Context, req chatsessions.RequestControlRequest) (result chatsessions.RequestControlResult, err error) {
	s.logStart("RequestControl", req.SessionID)
	defer func() {
		s.logOutcome("RequestControl", req.SessionID, err,
			"request_kind", string(req.RequestID.Kind), "action", string(req.Action),
			"turn_id", result.Intent.TurnID, "target_episode", result.Intent.TargetEpisode)
	}()
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
	if existing, exists := record.controls[req.RequestID]; exists {
		return chatsessions.RequestControlResult{Intent: existing}, nil
	}
	if record.session.ActiveTurnID == "" {
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
		TurnID:          record.session.ActiveTurnID,
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
// intent's own captured TurnID, the captured turn's live State, and the
// session's lastTurnID -- caller-supplied Next is never consulted for that
// resolution, so a delayed advancement can never retarget a captured intent
// to a later turn. lastTurnID (not session.ActiveTurnID) is the comparison
// point: ActiveTurnID clears to blank the instant the captured turn
// terminates, which would make every resolution SUPERSEDED and the NOOP
// outcome unreachable; lastTurnID keeps identifying the captured turn until
// a newer one is actually admitted, so a cancel racing a same-turn terminal
// resolves NOOP while a control racing a since-admitted newer turn resolves
// SUPERSEDED.
func (s *Store) AdvanceControl(_ context.Context, req chatsessions.AdvanceControlRequest) (result chatsessions.AdvanceControlResult, err error) {
	s.logStart("AdvanceControl", req.SessionID)
	defer func() {
		s.logOutcome("AdvanceControl", req.SessionID, err,
			"request_kind", string(req.RequestID.Kind), "state", string(result.Intent.State))
	}()
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
		outcome, err := chatsessions.ResolveControlIntentOutcome(intent.TurnID, capturedTurn.State, record.lastTurnID)
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

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
// ControlIntentState transition table. A terminal resolution always uses the
// immutable captured turn, never a caller-selected successor. A completed
// CLOSE additionally atomically terminalizes its captured turn (when still
// active), target episode, and Chat Session, and detaches every delivery
// attachment after the transport has already closed the exact captured Factory
// Session. The one Store lock makes that lifecycle commit indivisible: no
// observer can see a completed close intent alongside an open Chat Session,
// episode, or live attachment.
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

	next, err := resolveControlOutcome(record, intent, req.Next)
	if err != nil {
		return chatsessions.AdvanceControlResult{}, err
	}
	if err := intent.State.CanTransitionTo(next); err != nil {
		return chatsessions.AdvanceControlResult{}, err
	}

	updated := intent
	updated.State = next
	if err := updated.Validate(); err != nil {
		return chatsessions.AdvanceControlResult{}, err
	}
	if intent.Action == chatsessions.ControlActionClose && next == chatsessions.ControlIntentStateCompleted {
		record, err = s.completeCloseIntent(record, intent)
		if err != nil {
			return chatsessions.AdvanceControlResult{}, err
		}
	}

	record.controls[req.RequestID] = updated
	s.sessions[req.SessionID] = record

	return chatsessions.AdvanceControlResult{Intent: updated}, nil
}

// resolveControlOutcome keeps all terminal intent resolution tied to the
// immutable captured turn. CLOSE differs only when that same turn has already
// terminalized: closing its Factory Session still has a useful lifecycle
// effect, so the intent completes rather than becoming a CANCEL-style NOOP.
// A later turn remains SUPERSEDED for every action and can never be reached.
func resolveControlOutcome(record sessionRecord, intent chatsessions.ControlIntent, requested chatsessions.ControlIntentState) (chatsessions.ControlIntentState, error) {
	if intent.State != chatsessions.ControlIntentStateCommitted {
		return requested, nil
	}
	capturedTurn, ok := record.turns[intent.TurnID]
	if !ok {
		return "", &chatsessions.NotFoundError{Value: "Turn", ID: intent.TurnID}
	}
	outcome, err := chatsessions.ResolveControlIntentOutcome(intent.TurnID, capturedTurn.State, record.lastTurnID)
	if err != nil {
		return "", err
	}
	if intent.Action == chatsessions.ControlActionClose && outcome == chatsessions.ControlIntentStateNoop {
		return chatsessions.ControlIntentStateCompleted, nil
	}
	return outcome, nil
}

// completeCloseIntent prepares the one atomic Chat lifecycle transition that
// accompanies a successful captured Factory Session close. It changes no
// stored state until every turn, episode, and session candidate validates.
func (s *Store) completeCloseIntent(record sessionRecord, intent chatsessions.ControlIntent) (sessionRecord, error) {
	if record.lastTurnID != intent.TurnID {
		return record, &chatsessions.ConflictError{Value: "Turn", ID: intent.TurnID, Expected: intent.ExpectedVersion, Actual: record.session.Version}
	}
	if record.session.ActiveTurnID != "" && record.session.ActiveTurnID != intent.TurnID {
		return record, &chatsessions.ConflictError{Value: "Turn", ID: intent.TurnID, Expected: intent.ExpectedVersion, Actual: record.session.Version}
	}
	if record.session.State == chatsessions.SessionStateClosed {
		return record, nil
	}
	idx := len(record.episodes) - 1
	if record.episodes[idx].Number != intent.TargetEpisode {
		return record, &chatsessions.ConflictError{Value: "TargetEpisode", ID: intent.SessionID, Expected: intent.ExpectedVersion, Actual: record.session.Version}
	}

	now := s.now()
	closedEpisode, err := chatsessions.CloseTargetEpisode(record.episodes[idx], now)
	if err != nil {
		return record, err
	}
	updatedTurn, updatedSequence, err := closeCapturedTurn(record, intent.TurnID)
	if err != nil {
		return record, err
	}
	updatedSession := record.session
	if err := updatedSession.State.CanTransitionTo(chatsessions.SessionStateClosed); err != nil {
		return record, err
	}
	updatedSession.State = chatsessions.SessionStateClosed
	updatedSession.ActiveTurnID = ""
	updatedSession.Version++
	updatedSession.UpdatedAt = now
	if err := updatedSession.Validate(); err != nil {
		return record, err
	}

	record.episodes[idx] = closedEpisode
	record.turns[intent.TurnID] = updatedTurn
	record.turnSequence = updatedSequence
	record.session = updatedSession
	record.attachments = detachedAttachments(record.attachments)
	return record, nil
}

// detachedAttachments makes a detached copy of every attachment so a failed
// close validation cannot mutate the record still held by the Store. A closed
// Chat Session has no live delivery owner, while the preserved IDs and cursors
// continue to support process-local retained-history inspection.
func detachedAttachments(attachments map[string]chatsessions.Attachment) map[string]chatsessions.Attachment {
	detached := make(map[string]chatsessions.Attachment, len(attachments))
	for id, attachment := range attachments {
		attachment.Detached = true
		detached[id] = attachment
	}
	return detached
}

// closeCapturedTurn terminalizes a still-active captured turn as CANCELED.
// A turn that already reached a terminal outcome retains that truthful state;
// close only clears the session lifecycle around it.
func closeCapturedTurn(record sessionRecord, turnID string) (chatsessions.Turn, uint64, error) {
	turn, ok := record.turns[turnID]
	if !ok {
		return chatsessions.Turn{}, record.turnSequence, &chatsessions.NotFoundError{Value: "Turn", ID: turnID}
	}
	if turn.State.IsTerminal() {
		return turn, record.turnSequence, nil
	}
	if err := turn.State.CanTransitionTo(chatsessions.TurnStateCanceled); err != nil {
		return chatsessions.Turn{}, record.turnSequence, err
	}
	turn.State = chatsessions.TurnStateCanceled
	sequence := record.turnSequence + 1
	turn.TerminalSequence = sequence
	if err := turn.Validate(); err != nil {
		return chatsessions.Turn{}, record.turnSequence, err
	}
	return turn, sequence, nil
}

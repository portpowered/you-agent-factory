package stdio

import (
	"context"
	"errors"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
)

// reconcileCanceledCloseAfterBindFailure preserves a canceled prompt result
// only when its initial bind raced a successful CLOSE of that same lifecycle.
// All other bind errors remain authoritative.
func (s *Server) reconcileCanceledCloseAfterBindFailure(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	outcome factorysessions.InvocationResult,
	liveDelivered bool,
	bindErr error,
) (dispatchOutcome, bool) {
	if outcome.Status != factorysessions.InvocationTerminalStatusCanceled {
		return dispatchOutcome{}, false
	}
	closedVersion, closed := s.closedCapturedTurnAfterBindConflict(ctx, startResult, bindErr)
	if !closed {
		return dispatchOutcome{}, false
	}
	return dispatchOutcome{
		outcome:        protocol.MapFactoryInvocationOutcome(outcome),
		terminal:       factoryInvocationTurnState(outcome.Status),
		sessionVersion: closedVersion,
		liveDelivered:  liveDelivered,
	}, true
}

// closedCapturedTurnAfterBindConflict recognizes the one expected bind race
// between a canceled first invocation and a successful CLOSE. The regular
// BindFactorySession guard must still reject the stale active turn; this
// transport-level reconciliation only accepts that rejection after a fresh
// snapshot proves the same captured turn and episode have already been
// terminalized by a closed Chat Session. It never converts a general bind,
// version, or replacement conflict into a successful prompt outcome.
func (s *Server) closedCapturedTurnAfterBindConflict(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	bindErr error,
) (uint64, bool) {
	var conflictErr *chatsessions.ConflictError
	if !errors.As(bindErr, &conflictErr) ||
		conflictErr.Value != "Turn" ||
		conflictErr.ID != startResult.Turn.ID {
		return 0, false
	}
	current, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: startResult.Session.ID})
	if err != nil ||
		current.Session.State != chatsessions.SessionStateClosed ||
		current.Session.ActiveTurnID != "" ||
		current.MostRecentTurnID != startResult.Turn.ID ||
		current.Episode.Number != startResult.Episode.Number ||
		current.Episode.State != chatsessions.TargetEpisodeStateClosed {
		return 0, false
	}
	return current.Session.Version, true
}

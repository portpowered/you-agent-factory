package stdio

import (
	"context"
	"encoding/json"
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

var errSessionCloseUnavailable = errors.New("acp: session/close collaborators are not configured")
var errSessionCloseTargetUnavailable = errors.New("acp: session/close target execution is unavailable")
var errSessionCloseControlInapplicable = errors.New("acp: session/close captured control is no longer applicable")
var errSessionCloseUnexpectedIntent = errors.New("acp: session/close received an unexpected control intent")

// handleSessionClose validates one ACP request, commits its immutable CLOSE
// intent, and responds only after the exact captured Factory Session has
// closed and Chat Sessions has atomically terminalized the captured lifecycle.
func (s *Server) handleSessionClose(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	if s.chatSessions == nil || s.factoryTarget == nil {
		return nil, classifyDependencyFailure(errSessionCloseUnavailable)
	}
	var req acpsdk.CloseSessionRequest
	if err := json.Unmarshal(env.Params, &req); err != nil {
		return nil, protocol.SafeReject(err)
	}
	params, err := session.ValidateClose(req)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}
	requestID, err := chatRequestIdentity(env.Identity)
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	if err := s.applySessionClose(ctx, string(params.SessionID), requestID); err != nil {
		return nil, classifySessionCloseFailure(err)
	}
	result, err := json.Marshal(acpsdk.CloseSessionResponse{})
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	return result, nil
}

// applySessionClose performs the ordered CLOSE path. The first read captures
// its version and target through RequestControl; the second verifies the same
// captured turn remains most recent before the Factory effect. A terminal
// captured turn remains a valid close target, but a successor never does.
func (s *Server) applySessionClose(ctx context.Context, sessionID string, requestID chatsessions.RequestIdentity) error {
	current, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: sessionID})
	if err != nil {
		return err
	}
	if current.Session.State == chatsessions.SessionStateClosed {
		return nil
	}
	if targetFactorySessionID(current.Episode) == "" {
		return errSessionCloseTargetUnavailable
	}

	intent, err := s.commitSessionClose(ctx, current, requestID)
	if err != nil {
		return err
	}
	if intent.State == chatsessions.ControlIntentStateCompleted {
		return nil
	}

	current, err = s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: intent.SessionID})
	if err != nil {
		return err
	}
	if current.Session.State == chatsessions.SessionStateClosed {
		return nil
	}
	if current.Episode.Number != intent.TargetEpisode || current.MostRecentTurnID != intent.TurnID {
		s.resolveSessionControlIntent(ctx, intent)
		return errSessionCloseControlInapplicable
	}
	factorySessionID := targetFactorySessionID(current.Episode)
	if factorySessionID == "" {
		return errSessionCloseTargetUnavailable
	}
	if err := s.factoryTarget.TerminateFactorySession(ctx, factorySessionID, factorysessions.ControlRequest{
		RequestID: factoryTerminateRequestID(intent.RequestID),
		Reason:    "acp session/close",
		TurnID:    intent.TurnID,
	}); err != nil {
		return err
	}
	resolved, err := s.chatSessions.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: intent.SessionID,
		RequestID: intent.RequestID,
		Next:      chatsessions.ControlIntentStateCompleted,
	})
	if err != nil {
		return err
	}
	if resolved.Intent.State != chatsessions.ControlIntentStateCompleted {
		return errSessionCloseControlInapplicable
	}
	return nil
}

// commitSessionClose moves a newly requested CLOSE intent to COMMITTED before
// any Factory effect. A committed redelivery retries the same immutable
// target; a completed redelivery is the deterministic idempotent success.
func (s *Server) commitSessionClose(
	ctx context.Context,
	current chatsessions.GetSessionResult,
	requestID chatsessions.RequestIdentity,
) (chatsessions.ControlIntent, error) {
	requested, err := s.chatSessions.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID:       requestID,
		SessionID:       current.Session.ID,
		ExpectedVersion: current.Session.Version,
		Action:          chatsessions.ControlActionClose,
	})
	if err != nil {
		return chatsessions.ControlIntent{}, err
	}
	intent := requested.Intent
	if intent.Action != chatsessions.ControlActionClose || intent.SessionID != current.Session.ID {
		return chatsessions.ControlIntent{}, errSessionCloseUnexpectedIntent
	}
	switch intent.State {
	case chatsessions.ControlIntentStateRequested:
		committed, err := s.chatSessions.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
			SessionID: intent.SessionID,
			RequestID: intent.RequestID,
			Next:      chatsessions.ControlIntentStateCommitted,
		})
		if err != nil {
			return chatsessions.ControlIntent{}, err
		}
		return committed.Intent, nil
	case chatsessions.ControlIntentStateCommitted, chatsessions.ControlIntentStateCompleted:
		return intent, nil
	default:
		return chatsessions.ControlIntent{}, errSessionCloseControlInapplicable
	}
}

// classifySessionCloseFailure maps caller-addressable lifecycle rejections to
// invalid params while preserving the bounded internal-error behavior for a
// missing collaborator or downstream Factory Sessions failure.
func classifySessionCloseFailure(cause error) *acpsdk.RequestError {
	if errors.Is(cause, errSessionCloseTargetUnavailable) ||
		errors.Is(cause, errSessionCloseControlInapplicable) ||
		errors.Is(cause, errSessionCloseUnexpectedIntent) {
		return protocol.SafeReject(cause)
	}
	return classifyTurnAdmissionFailure(cause)
}

package stdio

import (
	"context"
	"encoding/json"
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// errSessionReconnectUnavailable marks a "session/load" or "session/resume"
// call this Server was never constructed with the Chat Sessions collaborator
// to serve. It never reaches a client verbatim -- classifyDependencyFailure
// maps it to a bounded internal-error response the same way it maps every
// other dependency failure.
var errSessionReconnectUnavailable = errors.New("acp: session/load and session/resume collaborators are not configured")

// handleSessionLoad executes one "session/load" request: reconnect this
// connection's delivery cursor to sessionID, an existing Chat Session, via
// reconnectSession. Per the pinned ACP SDK, session/load's own response
// carries no prior message history -- a client observes any catch-up the
// same way a live client does, through "session/update" notifications once
// this (or a later) "session/prompt" call streams from wherever this
// reconnect resumed (see ensureAttachment's Resume semantics in
// prompt_stream.go).
func (s *Server) handleSessionLoad(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	params, err := session.ValidateLoadSession(env.Params)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}
	attachment, rpcErr := s.reconnectSession(ctx, env, params.SessionID, params.ResumeAttachmentID)
	if rpcErr != nil {
		return nil, rpcErr
	}

	result, err := json.Marshal(acpsdk.LoadSessionResponse{Meta: session.AttachmentResumeMetadata(attachment.ID)})
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	return result, nil
}

// handleSessionResume executes one "session/resume" request: the same
// reconnect-only-the-delivery-cursor effect as handleSessionLoad, for a
// client that implements resume without full session loading (see
// acpsdk.ResumeSessionRequest's own doc comment -- "resumes an existing
// session without returning previous messages").
func (s *Server) handleSessionResume(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	params, err := session.ValidateResumeSession(env.Params)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}
	attachment, rpcErr := s.reconnectSession(ctx, env, params.SessionID, params.ResumeAttachmentID)
	if rpcErr != nil {
		return nil, rpcErr
	}

	result, err := json.Marshal(acpsdk.ResumeSessionResponse{Meta: session.AttachmentResumeMetadata(attachment.ID)})
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	return result, nil
}

// reconnectSession is the shared effect behind handleSessionLoad and
// handleSessionResume: confirm sessionID still identifies an existing Chat
// Session, then eagerly create a fresh delivery attachment or reactivate the
// exact detached attachment named by resumeAttachmentID. It records the
// result in this connection's attachmentCache so a later "session/prompt" on
// the same connection reuses it through ensureAttachment's cache-hit path
// instead of attaching again. An omitted identity always creates a fresh
// consumer; it never guesses among detached attachments. An unknown sessionID
// (*chatsessions.NotFoundError) classifies as a bounded invalid-params rejection via
// classifyTurnAdmissionFailure, matching how admitPromptTurn classifies the
// identical GetSession failure for "session/prompt".
func (s *Server) reconnectSession(
	ctx context.Context,
	env envelope.Envelope,
	sessionID session.SessionID,
	resumeAttachmentID string,
) (chatsessions.Attachment, *acpsdk.RequestError) {
	if s.chatSessions == nil {
		return chatsessions.Attachment{}, classifyDependencyFailure(errSessionReconnectUnavailable)
	}

	connectionID, ok := env.Identity.ConnectionID()
	if !ok {
		return chatsessions.Attachment{}, protocol.SafeReject(errors.New("acp: request identity has no connection"))
	}

	if _, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: string(sessionID)}); err != nil {
		return chatsessions.Attachment{}, classifyTurnAdmissionFailure(err)
	}

	cache := attachmentCacheFromContext(ctx)
	cache.setResumeAttachmentID(string(sessionID), resumeAttachmentID)
	attachment, attached, err := s.ensureAttachment(ctx, string(connectionID), string(sessionID))
	if err != nil {
		return chatsessions.Attachment{}, classifyDependencyFailure(err)
	}
	if !attached {
		return chatsessions.Attachment{}, classifyDependencyFailure(errSessionReconnectUnavailable)
	}
	return attachment, nil
}

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

// handleSessionLoad executes one "session/load" request. It attaches a fresh
// delivery cursor for a newly loading connection, then replays every retained
// Chat Session update in aggregate order before returning LoadSessionResponse.
// The pinned ACP SDK deliberately carries session history as session/update
// notifications, not in LoadSessionResponse itself. A retry on the same
// connection reuses its attachment and therefore continues from its
// acknowledged cursor without redelivering an update already observed.
func (s *Server) handleSessionLoad(ctx context.Context, env envelope.Envelope) (json.RawMessage, *acpsdk.RequestError) {
	params, err := session.ValidateLoadSession(env.Params)
	if err != nil {
		return nil, protocol.SafeReject(err)
	}
	if attachmentCacheFromContext(ctx) == nil {
		ctx = contextWithAttachmentCache(ctx, &attachmentCache{})
	}
	attachment, sessionVersion, rpcErr := s.loadSession(ctx, env, params.SessionID)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if _, err := s.streamTurnUpdates(contextWithHistoryReplay(ctx), attachment.ConnectionID, string(params.SessionID), sessionVersion, promptNotifierFromContext(ctx)); err != nil {
		return nil, classifyDependencyFailure(err)
	}

	result, err := json.Marshal(acpsdk.LoadSessionResponse{Meta: session.AttachmentResumeMetadata(attachment.ID)})
	if err != nil {
		return nil, classifyDependencyFailure(err)
	}
	return result, nil
}

// loadSession creates the fresh attachment session/load needs to replay
// retained history, or returns the attachment already created by a retry on
// this connection. ResumeAttachmentID is intentionally not consulted here:
// ACP session/load is the full-history operation, while session/resume owns
// the cursor-preserving reconnect behavior below. The attachment cache lives
// only for one connection, so a later connection always begins its load at
// the retained stream's head-zero position.
func (s *Server) loadSession(
	ctx context.Context,
	env envelope.Envelope,
	sessionID session.SessionID,
) (chatsessions.Attachment, uint64, *acpsdk.RequestError) {
	if s.chatSessions == nil {
		return chatsessions.Attachment{}, 0, classifyDependencyFailure(errSessionReconnectUnavailable)
	}

	connectionID, ok := env.Identity.ConnectionID()
	if !ok {
		return chatsessions.Attachment{}, 0, protocol.SafeReject(errors.New("acp: request identity has no connection"))
	}

	current, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: string(sessionID)})
	if err != nil {
		return chatsessions.Attachment{}, 0, classifyTurnAdmissionFailure(err)
	}

	cache := attachmentCacheFromContext(ctx)
	if attachment, attached := cache.get(string(sessionID)); attached &&
		(current.Session.State != chatsessions.SessionStateClosed || cache.wasLoaded(string(sessionID))) {
		return attachment, current.Session.Version, nil
	}
	attached, err := s.chatSessions.Attach(ctx, chatsessions.AttachRequest{
		SessionID:    string(sessionID),
		ConnectionID: string(connectionID),
		Interactive:  true,
	})
	if err != nil {
		return chatsessions.Attachment{}, 0, classifyDependencyFailure(err)
	}
	cache.set(string(sessionID), attached.Attachment)
	cache.markLoaded(string(sessionID))
	return attached.Attachment, current.Session.Version, nil
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

// reconnectSession is session/resume's cursor-preserving reconnect effect:
// it confirms sessionID still identifies an existing Chat Session, then
// reactivates the exact detached attachment named by resumeAttachmentID. It
// records the result in this connection's attachmentCache so a later
// "session/prompt" on the same connection reuses it through
// ensureAttachment's cache-hit path instead of attaching again. An omitted
// identity creates a fresh consumer; it never guesses among detached
// attachments. An unknown sessionID (*chatsessions.NotFoundError) classifies
// as a bounded invalid-params rejection via classifyTurnAdmissionFailure,
// matching how admitPromptTurn classifies the identical GetSession failure for
// "session/prompt".
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

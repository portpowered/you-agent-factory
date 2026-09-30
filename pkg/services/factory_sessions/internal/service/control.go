package service

import (
	"context"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// Control applies live closure through the owner attached to the canonical
// session record. Other controls retain their existing Assembly route while
// their owner methods are folded into the process root.
func (r *Root) Control(ctx context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	if r == nil || r.Assembly == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("Factory Sessions process root is required")
	}
	if request.Mode != factorysessions.SessionOperationModeLive {
		return r.Assembly.Control(ctx, request)
	}
	sessionID := strings.TrimSpace(request.SessionID)
	if sessionID == "" {
		return factorysessions.SessionControlResult{}, &factorysessions.DetachedRequestError{Field: "sessionId", Message: "session id is required"}
	}
	session := r.Resolve(sessionID)
	if session == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	if request.Operation != factorysessions.SessionControlClose {
		return r.Assembly.ApplyLiveControl(ctx, request)
	}
	if err := r.Assembly.CloseSession(ctx, sessionID); err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	return factorysessions.SessionControlResult{
		SessionID: sessionID, Mode: request.Mode, Operation: request.Operation, Closed: true,
	}, nil
}

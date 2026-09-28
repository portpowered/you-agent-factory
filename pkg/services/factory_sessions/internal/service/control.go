package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// Control applies live closure through the owner attached to the canonical
// session record. Other controls retain their existing Assembly route while
// their owner methods are folded into the process root.
func (r *Root) Control(ctx context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	if r == nil || r.Assembly == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("Factory Sessions process root is required")
	}
	if request.Mode != factorysessions.SessionOperationModeLive || request.Operation != factorysessions.SessionControlClose {
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
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("%w: session owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	owner, ok := bound.Owner.(interface {
		CloseOwnedSession(context.Context, string) error
	})
	if !ok {
		return factorysessions.SessionControlResult{}, fmt.Errorf("%w: session close is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	closeErr := owner.CloseOwnedSession(ctx, sessionID)
	if bound.Activation != nil {
		closeErr = errors.Join(closeErr, bound.Activation.Close(ctx))
	}
	if closeErr != nil {
		return factorysessions.SessionControlResult{}, closeErr
	}
	return factorysessions.SessionControlResult{
		SessionID: sessionID, Mode: request.Mode, Operation: request.Operation, Closed: true,
	}, nil
}

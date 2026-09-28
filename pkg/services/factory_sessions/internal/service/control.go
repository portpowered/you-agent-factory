package service

import (
	"context"
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
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("%w: session owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	if request.Operation != factorysessions.SessionControlClose {
		owner, ok := bound.Owner.(interface {
			ApplyOwnedControl(context.Context, string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
		})
		if !ok {
			return factorysessions.SessionControlResult{}, fmt.Errorf("%w: session control is unavailable", factorysessions.ErrRuntimeNotAvailable)
		}
		control := request.Control
		if control.RequestID == "" {
			control.RequestID = strings.TrimSpace(request.Correlation.RequestID)
		}
		if control.TurnID == "" {
			control.TurnID = strings.TrimSpace(request.Correlation.TurnID)
		}
		key := ""
		if control.RequestID != "" {
			key = string(request.Operation) + ":" + control.RequestID + ":" + control.TurnID
		}
		return bound.ApplyControlOnce(key, func() (factorysessions.SessionControlResult, error) {
			applied, err := owner.ApplyOwnedControl(ctx, sessionID, factorysessions.LifecycleControlKind(request.Operation), control)
			if err != nil {
				return factorysessions.SessionControlResult{}, err
			}
			return factorysessions.SessionControlResult{
				SessionID: sessionID, Mode: request.Mode, Operation: request.Operation,
				Outcome: applied.Outcome, Status: applied.Status, Links: applied.Links,
			}, nil
		})
	}
	if err := r.Assembly.CloseSession(ctx, sessionID); err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	return factorysessions.SessionControlResult{
		SessionID: sessionID, Mode: request.Mode, Operation: request.Operation, Closed: true,
	}, nil
}

package service

import (
	"context"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// ApplyLiveControl routes cancellation through the independent scope authority.
// Other controls retain their opening-owner bridge and shared record fence.
func (a *Assembly) ApplyLiveControl(ctx context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	if a == nil {
		return factorysessions.SessionControlResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	id := strings.TrimSpace(request.SessionID)
	if id == "" {
		return factorysessions.SessionControlResult{}, &factorysessions.DetachedRequestError{Field: "sessionId", Message: "session id is required"}
	}
	session := a.Resolve(id)
	if session == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, id)
	}

	if request.Operation == factorysessions.SessionControlCancel {
		control := request.Control
		if control.RequestID == "" {
			control.RequestID = strings.TrimSpace(request.Correlation.RequestID)
		}
		if control.TurnID == "" {
			control.TurnID = strings.TrimSpace(request.Correlation.TurnID)
		}
		applied, err := a.scopeControl.CancelLiveFactorySession(ctx, id, control)
		if err != nil {
			return factorysessions.SessionControlResult{}, err
		}
		return factorysessions.SessionControlResult{
			SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: request.Operation,
			Outcome: applied.Outcome, Status: applied.Status, Links: applied.Links,
		}, nil
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("%w: session owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
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
		applied, err := owner.ApplyOwnedControl(ctx, id, factorysessions.LifecycleControlKind(request.Operation), control)
		if err != nil {
			return factorysessions.SessionControlResult{}, err
		}
		return factorysessions.SessionControlResult{
			SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: request.Operation,
			Outcome: applied.Outcome, Status: applied.Status, Links: applied.Links,
		}, nil
	})
}

func (a *Assembly) applyLiveControlLegacy(ctx context.Context, sessionID string, operation factorysessions.SessionControlOperation, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	result, err := a.ApplyLiveControl(ctx, factorysessions.SessionControlRequest{
		SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive,
		Operation: operation, Control: control,
	})
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return factorysessions.LifecycleControlResult{
		SessionID: result.SessionID, Operation: factorysessions.LifecycleControlKind(result.Operation),
		Outcome: result.Outcome, Status: result.Status, Links: result.Links,
	}, nil
}

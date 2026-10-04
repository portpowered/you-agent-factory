package service

import (
	"context"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// CloseSession closes one canonical record in retryable phases. The registry
// retains the record until termination, activation cleanup, and retirement
// all succeed, so a later call can resume an incomplete close.
func (a *Assembly) CloseSession(ctx context.Context, sessionID string) error {
	if a == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return fmt.Errorf("factory session ID is required")
	}
	session := a.Resolve(id)
	if session == nil {
		return nil
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return fmt.Errorf("%w: session owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	owner, ok := bound.Owner.(interface {
		PrepareOwnedSessionClose(context.Context, *livesession.LiveSession) error
		RetireOwnedSession(context.Context, *livesession.LiveSession) error
	})
	if !ok {
		return fmt.Errorf("%w: session close is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	if err := owner.PrepareOwnedSessionClose(ctx, session); err != nil {
		return err
	}
	if bound.Activation != nil {
		if err := bound.Activation.Close(ctx); err != nil {
			return err
		}
	}
	return owner.RetireOwnedSession(ctx, session)
}

package service

import (
	"context"
	"errors"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// CloseOwnedSession retires the session owned by this runtime directly from
// the canonical registry, without looking up a per-session gateway.
func (fs *SessionRuntime) CloseOwnedSession(ctx context.Context, sessionID string) error {
	if fs == nil {
		return fmt.Errorf("Factory Session runtime is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID)
	if err != nil {
		return err
	}
	if runtime := runtimebinding.ServiceForSession(session); runtime != nil {
		_, err := runtime.ControlTerminate(ctx, factoryruntime.TerminateRequest{Reason: "factory session closed"})
		if err != nil && !errors.Is(err, factoryruntime.ErrAlreadyStopped) && !errors.Is(err, factoryruntime.ErrNotRunning) {
			return fmt.Errorf("terminate live Factory Session: %w", err)
		}
	}
	return fs.stopFactorySession(sessionID)
}

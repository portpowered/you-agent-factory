package service

import (
	"context"
	"errors"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// PrepareOwnedSessionClose terminates the session without removing its record.
// The record remains available when later cleanup needs a retry.
func (fs *SessionRuntime) PrepareOwnedSessionClose(ctx context.Context, sessionID string) error {
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
	// Termination can publish RUN_RESPONSE before the hosted run appends its
	// final session events. Join the run before activation cleanup closes
	// recording; retirement owns the runtime stop and artifact finalization.
	if handle := runtimebinding.HandleFromSession(session); handle != nil {
		select {
		case <-handle.RunDoneCh():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// RetireOwnedSession removes the canonical record only after all other
// shutdown effects have succeeded.
func (fs *SessionRuntime) RetireOwnedSession(ctx context.Context, sessionID string) error {
	if fs == nil {
		return fmt.Errorf("Factory Session runtime is required")
	}
	err := runtimebinding.StopSession(fs.sessionState, &fs.runtimeState, sessionID, func(runtimebinding.RuntimeHandle) error {
		return fs.scopeControl.StopLiveSession(ctx, sessionID)
	})
	if err == nil && fs.releaseWorkAdmissionProjection != nil {
		fs.releaseWorkAdmissionProjection(sessionID)
	}
	return err
}

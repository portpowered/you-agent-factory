package service

import (
	"context"
	"errors"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// PrepareOwnedSessionClose terminates the session without removing its record.
// The record remains available when later cleanup needs a retry.
func (a *Assembly) scopedPrepareOwnedSessionClose(fs *SessionRuntime, ctx context.Context, session *livesession.LiveSession) error {
	if fs == nil {
		return fmt.Errorf("Factory Session runtime is required")
	}
	if err := ctx.Err(); err != nil {
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
func (a *Assembly) scopedRetireOwnedSession(fs *SessionRuntime, ctx context.Context, session *livesession.LiveSession) error {
	if fs == nil {
		return fmt.Errorf("Factory Session runtime is required")
	}
	err := runtimebinding.CleanupSessionGeneration(session, func(runtimebinding.RuntimeHandle) error {
		return a.scopeControl.StopLiveGeneration(ctx, session)
	})
	if err != nil {
		return err
	}
	if err := a.scopeActivation.Retire(ctx, SessionScope{Session: session}); err != nil {
		return err
	}
	successor := a.state.Resolve(session.ID)
	if successor == nil {
		successor = runtimebinding.NextLiveSession(a.state, session.ID)
	}
	fs.runtimeState.RetireActive(session.ID, runtimebinding.HandleFromSession(session), successor)
	a.retireWorkAdmissionProjection(session.ID, session.Runtime, runtimebinding.BundleFromSession(session))
	return nil
}

func (fs *SessionRuntime) PrepareOwnedSessionClose(ctx context.Context, session *livesession.LiveSession) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedPrepareOwnedSessionClose(fs, ctx, session)
}

func (fs *SessionRuntime) RetireOwnedSession(ctx context.Context, session *livesession.LiveSession) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedRetireOwnedSession(fs, ctx, session)
}

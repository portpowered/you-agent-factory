package service

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
)

// These addressed legacy operations remain the T17 opening compatibility bridge.
func (a *Assembly) scopeActivateSessionEditableFactory(fs *SessionRuntime, ctx context.Context, session *livesession.LiveSession, sessionID, sessionRootDir, factoryDir, name, runtimeName string) error {
	return ActivateSessionRuntime(ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName,
		fs.buildReplacementFactoryRuntime, fs.requireIdleRuntimeForSession, fs.ReplaceSessionRuntime)
}

func (a *Assembly) scopeSwapPersistedNamedFactoryRuntime(fs *SessionRuntime, ctx context.Context, sessionID string, session *livesession.LiveSession, persistRoot, folderPath, factoryDir, name string) error {
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, folderPath, factoryDir, sessionID)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %w", interfaces.ErrInvalidNamedFactory, name, err)
	}
	return ApplyNamedReplacement(
		ctx,
		sessionID,
		session,
		runtimebinding.HandleFromSession(session) != nil,
		persistRoot,
		name,
		replacement,
		fs.requireIdleRuntimeForSession,
		fs.requireIdleRuntime,
		fs.ReplaceSessionRuntime,
		func(rootDir, name string, replacement runtimeports.RuntimeInstance) error {
			return ActivateStartupRuntime(
				rootDir, name, replacement, &fs.runtimeState, fs.syncActiveSessionDir,
				a.namedPaths.WriteCurrentPointer,
			)
		},
		a.namedPaths.WriteCurrentPointer,
	)
}

func (fs *SessionRuntime) activateSessionEditableFactory(ctx context.Context, session *livesession.LiveSession, sessionID, sessionRootDir, factoryDir, name, runtimeName string) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeActivateSessionEditableFactory(fs, ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName)
}

func (fs *SessionRuntime) swapPersistedNamedFactoryRuntime(ctx context.Context, sessionID string, session *livesession.LiveSession, persistRoot, folderPath, factoryDir, name string) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeSwapPersistedNamedFactoryRuntime(fs, ctx, sessionID, session, persistRoot, folderPath, factoryDir, name)
}

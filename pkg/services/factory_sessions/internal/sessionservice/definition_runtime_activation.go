package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// NamedFactoryActivationPaths resolves persistence and runtime roots for one
// named-definition activation.
func NamedFactoryActivationPaths(factoryRoot, configuredRoot string, session *livesession.LiveSession) (persistRoot, folderPath string) {
	persistRoot = strings.TrimSpace(factoryRoot)
	if persistRoot == "" {
		persistRoot = strings.TrimSpace(configuredRoot)
	}
	folderPath = persistRoot
	if session == nil {
		return persistRoot, folderPath
	}
	persistRoot = logicaltarget.SessionFactoryPersistRoot(factoryRoot, session)
	if sessionFolder := strings.TrimSpace(session.FolderPath); sessionFolder != "" {
		folderPath = sessionFolder
	} else {
		folderPath = persistRoot
	}
	return persistRoot, folderPath
}

// RequireIdleBeforeNamedActivation selects session-scoped or process-scoped
// idle validation according to whether a live runtime exists.
func RequireIdleBeforeNamedActivation(
	ctx context.Context,
	sessionID string,
	session *livesession.LiveSession,
	liveRuntime bool,
	requireSessionIdle func(context.Context, string) error,
	requireRuntimeIdle func(context.Context) error,
) error {
	if session != nil && liveRuntime {
		if requireSessionIdle == nil {
			return fmt.Errorf("session runtime idle validator is required")
		}
		return requireSessionIdle(ctx, sessionID)
	}
	if requireRuntimeIdle == nil {
		return fmt.Errorf("factory runtime idle validator is required")
	}
	return requireRuntimeIdle(ctx)
}

// ActivateSessionRuntime builds, validates, and installs one persisted
// definition replacement using the canonical ordering and error policy.
func ActivateSessionRuntime(
	ctx context.Context,
	session *livesession.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
	build func(context.Context, string, string, string) (factory.HostedInstance, error),
	requireIdle func(context.Context, string) error,
	replace func(context.Context, *livesession.LiveSession, string, factory.HostedInstance) error,
) error {
	if build == nil || requireIdle == nil || replace == nil {
		return fmt.Errorf("factory runtime activation dependencies are required")
	}
	replacement, err := build(ctx, sessionRootDir, factoryDir, sessionID)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %w", factorydefinitions.ErrInvalidNamedFactory, name, err)
	}
	if err := requireIdle(ctx, sessionID); err != nil {
		return err
	}
	return replace(ctx, session, runtimeName, replacement)
}

// ApplyNamedReplacement installs a built named Factory definition using the
// canonical live-session or process-runtime path.
func ApplyNamedReplacement(
	ctx context.Context,
	sessionID string,
	session *livesession.LiveSession,
	liveRuntime bool,
	persistRoot string,
	name string,
	replacement factory.HostedInstance,
	requireSessionIdle func(context.Context, string) error,
	requireRuntimeIdle func(context.Context) error,
	replaceSession func(context.Context, *livesession.LiveSession, string, factory.HostedInstance) error,
	activateWithoutLiveRuntime func(string, string, factory.HostedInstance) error,
	writeCurrent factorydefinitions.CurrentFactoryPointerWriter,
) error {
	if writeCurrent == nil {
		return fmt.Errorf("current Factory pointer writer is required")
	}
	if session != nil && liveRuntime {
		if requireSessionIdle == nil || replaceSession == nil {
			return fmt.Errorf("live session replacement dependencies are required")
		}
		if err := requireSessionIdle(ctx, sessionID); err != nil {
			return err
		}
		if err := writeCurrent(persistRoot, name); err != nil {
			return err
		}
		return replaceSession(ctx, session, name, replacement)
	}
	if requireRuntimeIdle == nil || activateWithoutLiveRuntime == nil {
		return fmt.Errorf("factory runtime replacement dependencies are required")
	}
	if err := requireRuntimeIdle(ctx); err != nil {
		return err
	}
	return activateWithoutLiveRuntime(persistRoot, name, replacement)
}

// ActivateStartupRuntime persists the named Factory pointer and selects its
// already-built bundle for the next process runtime start.
func ActivateStartupRuntime(
	persistRoot string,
	name string,
	replacement factory.HostedInstance,
	runtimeState *runtimebinding.State,
	syncDirectory func(factory.HostedInstance),
	writeCurrent factorydefinitions.CurrentFactoryPointerWriter,
) error {
	if runtimeState == nil {
		return fmt.Errorf("factory runtime state is required")
	}
	if writeCurrent == nil {
		return fmt.Errorf("current Factory pointer writer is required")
	}
	if err := writeCurrent(persistRoot, name); err != nil {
		return err
	}
	runtimeState.SetStartup(replacement)
	if syncDirectory != nil {
		syncDirectory(replacement)
	}
	return nil
}

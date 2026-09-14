package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
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
	build func(context.Context, string, string, string) (runtimeports.RuntimeInstance, error),
	requireIdle func(context.Context, string) error,
	replace func(context.Context, *livesession.LiveSession, string, runtimeports.RuntimeInstance) error,
) error {
	_, err := ActivateSessionRuntimeWithResult(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		factoryDir,
		name,
		runtimeName,
		build,
		requireIdle,
		replace,
	)
	return err
}

// ActivateSessionRuntimeWithResult builds, validates, and installs one
// persisted definition replacement, returning the exact loaded source that
// was handed to the live runtime. The source is validated before replacement
// so a successful result can be used for response assembly without a
// failure-prone post-swap read.
func ActivateSessionRuntimeWithResult(
	ctx context.Context,
	session *livesession.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
	build func(context.Context, string, string, string) (runtimeports.RuntimeInstance, error),
	requireIdle func(context.Context, string) error,
	replace func(context.Context, *livesession.LiveSession, string, runtimeports.RuntimeInstance) error,
) (factorydefinitions.LoadedFactorySource, error) {
	if build == nil || requireIdle == nil || replace == nil {
		return nil, fmt.Errorf("factory runtime activation dependencies are required")
	}
	replacement, err := build(ctx, sessionRootDir, factoryDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: build replacement factory %q: %w", factorydefinitions.ErrInvalidNamedFactory, name, err)
	}
	if replacement == nil {
		return nil, fmt.Errorf("replacement Factory Runtime is unavailable")
	}
	loadedConfig := replacement.LoadedRuntimeConfig()
	loaded, ok := loadedConfig.(factorydefinitions.LoadedFactorySource)
	if !ok || loaded == nil {
		return nil, fmt.Errorf("replacement Factory Runtime loaded source is required")
	}
	activationSource, ok := loaded.(factorydefinitions.LoadedFactoryActivationSource)
	if !ok || activationSource.FactoryActivationProvenance() == nil {
		return nil, fmt.Errorf("replacement Factory Runtime activation provenance is required")
	}
	if err := requireIdle(ctx, sessionID); err != nil {
		return nil, err
	}
	if err := replace(ctx, session, runtimeName, replacement); err != nil {
		return nil, err
	}
	return loaded, nil
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
	replacement runtimeports.RuntimeInstance,
	requireSessionIdle func(context.Context, string) error,
	requireRuntimeIdle func(context.Context) error,
	replaceSession func(context.Context, *livesession.LiveSession, string, runtimeports.RuntimeInstance) error,
	activateWithoutLiveRuntime func(string, string, runtimeports.RuntimeInstance) error,
	readCurrent factorydefinitions.CurrentFactoryPointerReader,
	writeCurrent factorydefinitions.CurrentFactoryPointerWriter,
	removeCurrent func(string) error,
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
		restorePointer, err := writeCurrentFactoryPointerTransaction(
			persistRoot,
			name,
			readCurrent,
			writeCurrent,
			removeCurrent,
		)
		if err != nil {
			return err
		}
		if err := replaceSession(ctx, session, name, replacement); err != nil {
			if restoreErr := restorePointer(); restoreErr != nil {
				return errors.Join(err, fmt.Errorf("restore current Factory pointer: %w", restoreErr))
			}
			return err
		}
		return nil
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
	replacement runtimeports.RuntimeInstance,
	runtimeState *runtimebinding.State,
	syncDirectory func(runtimeports.RuntimeInstance),
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

type currentFactoryPointerState struct {
	name   string
	exists bool
}

func writeCurrentFactoryPointerTransaction(
	rootDir string,
	name string,
	read factorydefinitions.CurrentFactoryPointerReader,
	write factorydefinitions.CurrentFactoryPointerWriter,
	remove func(string) error,
) (func() error, error) {
	if read == nil {
		return nil, fmt.Errorf("current Factory pointer reader is required")
	}
	state, err := readCurrentFactoryPointerState(read, rootDir)
	if err != nil {
		return nil, err
	}
	return writeCurrentFactoryPointerAfterState(state, rootDir, name, write, remove)
}

func readCurrentFactoryPointerState(
	read factorydefinitions.CurrentFactoryPointerReader,
	rootDir string,
) (currentFactoryPointerState, error) {
	name, err := read(rootDir)
	if err == nil {
		return currentFactoryPointerState{name: name, exists: true}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return currentFactoryPointerState{}, nil
	}
	return currentFactoryPointerState{}, err
}

func writeCurrentFactoryPointerAfterState(
	state currentFactoryPointerState,
	rootDir string,
	name string,
	write factorydefinitions.CurrentFactoryPointerWriter,
	remove func(string) error,
) (func() error, error) {
	if write == nil {
		return nil, fmt.Errorf("current Factory pointer writer is required")
	}
	restore := func() error {
		if state.exists {
			return write(rootDir, state.name)
		}
		if remove == nil {
			return fmt.Errorf("current Factory pointer remover is required to restore an absent pointer")
		}
		return remove(rootDir)
	}
	if err := write(rootDir, name); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return nil, errors.Join(err, fmt.Errorf("restore current Factory pointer after write failure: %w", restoreErr))
		}
		return nil, err
	}
	return restore, nil
}

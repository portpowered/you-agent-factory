package service

import (
	"context"
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

type DefinitionHostCallbacks struct {
	PersistRootDir                          func() string
	WorkstationLoader                       func() interfaces.WorkstationLoader
	CurrentRuntimeConfig                    func() interfaces.LoadedFactorySource
	WorkflowID                              func() string
	RequireSession                          func(string) (*livesession.LiveSession, error)
	SessionRuntimeConfig                    func(string) (interfaces.LoadedFactorySource, error)
	SessionFactoryPersistRoot               func(*livesession.LiveSession) string
	ValidateEditableFactorySnapshot         func(context.Context, *interfaces.FactorySnapshot) error
	GetCurrentFactorySnapshotForSession     func(context.Context, string) (*interfaces.FactorySnapshot, error)
	WithActivationLock                      func(func() error) error
	RequireIdleRuntimeForSession            func(context.Context, string) error
	ActivateSessionEditableFactory          func(context.Context, *livesession.LiveSession, string, string, string, string, string) error
	ReplaceFactoryLayoutAtDir               func(string, *interfaces.PreparedFactoryLayoutPayload) (*interfaces.FactorySplitLayoutReplaceResult, error)
	SaveNow                                 func() time.Time
	RunSessionID                            func() string
	SessionForActivation                    func(string) *livesession.LiveSession
	NamedFactoryActivationPaths             func(*livesession.LiveSession) (string, string)
	RequireIdleBeforeNamedFactoryActivation func(context.Context, string, *livesession.LiveSession) error
	SwapPersistedNamedFactoryRuntime        func(context.Context, string, *livesession.LiveSession, string, string, string, string) error
}

// AttachFactoryDefinitionService installs the Wire-constructed definition
// service used by the Factory Session runtime.
func (h *SessionRuntime) AttachFactoryDefinitionService(service interfaces.Service) interfaces.Service {
	if h != nil && service != nil {
		h.definitions = service
	}
	return service
}

// DefinitionCallbacks exposes bounded Factory Session callbacks for composition
// with a Factory Definition implementation at the initializer boundary.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func DefinitionCallbacks(runtime *SessionRuntime) DefinitionHostCallbacks {
	dependencies := DefinitionHostCallbacks{}
	if runtime == nil {
		return dependencies
	}
	dependencies.PersistRootDir = func() string {
		rootDir := runtime.factoryRootDir
		if rootDir == "" {
			rootDir = runtime.dir
		}
		return rootDir
	}
	dependencies.WorkstationLoader = func() interfaces.WorkstationLoader {
		return runtime.workstationLoader
	}
	dependencies.CurrentRuntimeConfig = runtime.currentRuntimeConfig
	dependencies.WorkflowID = func() string { return runtime.workflowID }
	dependencies.RequireSession = func(sessionID string) (*livesession.LiveSession, error) {
		return runtimebinding.RequireLiveSession(runtime.sessionState, sessionID)
	}
	dependencies.SessionRuntimeConfig = func(sessionID string) (interfaces.LoadedFactorySource, error) {
		return runtimebinding.RuntimeConfigForSession(runtime.sessionState, sessionID)
	}
	dependencies.SessionFactoryPersistRoot = func(session *livesession.LiveSession) string {
		return logicaltarget.SessionFactoryPersistRoot(runtime.factoryRootDir, session)
	}
	dependencies.ValidateEditableFactorySnapshot = func(ctx context.Context, snapshot *interfaces.FactorySnapshot) error {
		if runtime.editableFactoryValidator == nil {
			return fmt.Errorf("editable Factory validator is required")
		}
		return runtime.editableFactoryValidator(ctx, snapshot, dependencies.WorkstationLoader())
	}
	dependencies.GetCurrentFactorySnapshotForSession = func(ctx context.Context, sessionID string) (*interfaces.FactorySnapshot, error) {
		definitions := runtime.requireDefinitions()
		if definitions == nil {
			return nil, fmt.Errorf("factory definition service is required")
		}
		current, err := definitions.GetCurrentFactoryForSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if current.Snapshot == nil {
			return nil, fmt.Errorf("current factory snapshot is unavailable")
		}
		return current.Snapshot, nil
	}
	dependencies.WithActivationLock = runtime.sessionState.WithActivationLock
	dependencies.RequireIdleRuntimeForSession = runtime.requireIdleRuntimeForSession
	dependencies.ActivateSessionEditableFactory = func(
		ctx context.Context,
		session *livesession.LiveSession,
		sessionID, sessionRootDir, factoryDir, name, runtimeName string,
	) error {
		return ActivateSessionRuntime(
			ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName,
			runtime.buildReplacementFactoryRuntime,
			runtime.requireIdleRuntimeForSession,
			runtime.ReplaceSessionRuntime,
		)
	}
	dependencies.SaveNow = func() time.Time {
		return runtime.clock.Now().UTC()
	}
	dependencies.RunSessionID = runtime.runSessionID
	dependencies.SessionForActivation = runtime.sessionState.Resolve
	dependencies.NamedFactoryActivationPaths = func(session *livesession.LiveSession) (string, string) {
		return NamedFactoryActivationPaths(runtime.factoryRootDir, runtime.dir, session)
	}
	dependencies.RequireIdleBeforeNamedFactoryActivation = func(ctx context.Context, sessionID string, session *livesession.LiveSession) error {
		return RequireIdleBeforeNamedActivation(
			ctx, sessionID, session, runtimebinding.HandleFromSession(session) != nil,
			runtime.requireIdleRuntimeForSession, runtime.requireIdleRuntime,
		)
	}
	dependencies.SwapPersistedNamedFactoryRuntime = func(
		ctx context.Context,
		sessionID string,
		session *livesession.LiveSession,
		persistRoot, folderPath, factoryDir, name string,
	) error {
		replacement, err := runtime.buildReplacementFactoryRuntime(ctx, folderPath, factoryDir, sessionID)
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
			runtime.requireIdleRuntimeForSession,
			runtime.requireIdleRuntime,
			runtime.ReplaceSessionRuntime,
			func(rootDir, name string, replacement factory.HostedInstance) error {
				return ActivateStartupRuntime(
					rootDir, name, replacement, &runtime.runtimeState, runtime.syncActiveSessionDir,
					runtime.namedPaths.WriteCurrentPointer,
				)
			},
			runtime.namedPaths.WriteCurrentPointer,
		)
	}
	return dependencies
}

func (h *SessionRuntime) requireDefinitions() interfaces.Service {
	if h == nil {
		return nil
	}
	return h.definitions
}

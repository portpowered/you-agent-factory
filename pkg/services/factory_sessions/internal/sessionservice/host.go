package service

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
)

// Host exposes composition-root seams required by the session gateway.
type Host interface {
	controlplane.OpenControlHost
	controlplane.LiveReadHost
	controlplane.SyncPreflightHost
	controlplane.ResultReadHost
	controlplane.DurableLifecycleHost
	OpenLiveSessionForTarget(context.Context, factorysessions.Target) (string, error)
	SessionFactory(string) (factory.Service, error)
	StopLiveSession(string) error
	ObserveLiveLifecycleControl(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error)
}

type dependencyHost struct {
	discoverTargets               func(string) ([]factorysessions.Target, error)
	initializeFactoryScaffold     func(string) error
	openLiveSessionForTarget      func(context.Context, factorysessions.Target) (string, error)
	requireSession                func(string) (*livesession.LiveSession, error)
	listLiveSessionIDs            func() []string
	getLiveSession                func(string) *livesession.LiveSession
	buildSessionProjectionContext func(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error)
	resolveSyncPreflightTarget    func(string, *interfaces.FactorySessionLogicalResolveHint) (controlplane.SyncPreflightTarget, error)
	backendScopeID                func() string
	logicalSessionKeyID           func(*livesession.LiveSession) string
	streamGenerationID            func(*livesession.LiveSession) string
	liveSessionEvents             func(*livesession.LiveSession) []interfaces.FactoryEvent
	sessionFactory                func(string) (factory.Service, error)
	stopLiveSession               func(string) error
	observeLiveLifecycleControl   func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error)
	durableExecution              func() durableexecution.Service
	javaScriptCheckpointStore     func(*livesession.LiveSession) factory.JavaScriptCheckpointStore
	directoryInspection           roles.DirectoryInspection
	resolveSessionFolder          func(string) (string, error)
	selectTarget                  func([]factorysessions.Target, *factorysessions.TargetRef) (*factorysessions.Target, error)
}

func (h dependencyHost) ResolveSessionFolder(folderPath string) (string, error) {
	if h.resolveSessionFolder == nil {
		return "", fmt.Errorf("Factory Session folder resolver is required")
	}
	return h.resolveSessionFolder(folderPath)
}

func (h dependencyHost) SelectTarget(targets []factorysessions.Target, ref *factorysessions.TargetRef) (*factorysessions.Target, error) {
	if h.selectTarget == nil {
		return nil, fmt.Errorf("Factory Session target selector is required")
	}
	return h.selectTarget(targets, ref)
}

func (h dependencyHost) DiscoverTargets(folderPath string) ([]factorysessions.Target, error) {
	if h.discoverTargets == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.discoverTargets(folderPath)
}

func (h dependencyHost) InitializeFactoryScaffold(factoryDir string) error {
	if h.initializeFactoryScaffold == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.initializeFactoryScaffold(factoryDir)
}

func (h dependencyHost) ValidateInitNewFactoryNestedDir(resolvedFolder string) error {
	return logicaltarget.ValidateInitNewFactoryNestedDir(resolvedFolder, h.directoryInspection)
}

func (h dependencyHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if h.openLiveSessionForTarget == nil {
		return "", fmt.Errorf("factory service is required")
	}
	return h.openLiveSessionForTarget(ctx, target)
}

func (h dependencyHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if h.requireSession == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.requireSession(sessionID)
}

func (h dependencyHost) ListLiveSessionIDs() []string {
	if h.listLiveSessionIDs == nil {
		return nil
	}
	return h.listLiveSessionIDs()
}

func (h dependencyHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	if h.getLiveSession == nil {
		return nil
	}
	return h.getLiveSession(sessionID)
}

func (h dependencyHost) BuildSessionProjectionContext(ctx context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	if h.buildSessionProjectionContext == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory service is required")
	}
	return h.buildSessionProjectionContext(ctx, session)
}

func (h dependencyHost) ResolveSyncPreflightTarget(
	sessionID string,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (controlplane.SyncPreflightTarget, error) {
	if h.resolveSyncPreflightTarget == nil {
		return controlplane.SyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	return h.resolveSyncPreflightTarget(sessionID, logicalResolve)
}

func (h dependencyHost) BackendScopeID() string {
	if h.backendScopeID == nil {
		return ""
	}
	return h.backendScopeID()
}

func (h dependencyHost) LogicalSessionKeyID(session *livesession.LiveSession) string {
	if h.logicalSessionKeyID == nil {
		return ""
	}
	return h.logicalSessionKeyID(session)
}

func (h dependencyHost) StreamGenerationID(session *livesession.LiveSession) string {
	if h.streamGenerationID == nil {
		return ""
	}
	return h.streamGenerationID(session)
}

func (h dependencyHost) LiveSessionEvents(session *livesession.LiveSession) []interfaces.FactoryEvent {
	if h.liveSessionEvents == nil {
		return nil
	}
	return h.liveSessionEvents(session)
}

func (h dependencyHost) SessionFactory(sessionID string) (factory.Service, error) {
	if h.sessionFactory == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.sessionFactory(sessionID)
}

func (h dependencyHost) StopLiveSession(sessionID string) error {
	if h.stopLiveSession == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.stopLiveSession(sessionID)
}

func (h dependencyHost) ObserveLiveLifecycleControl(
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	control factorysessions.ControlRequest,
	outcome factorysessions.LifecycleControlOutcome,
	status factorysessions.LifecycleStatus,
	err error,
) {
	if h.observeLiveLifecycleControl != nil {
		h.observeLiveLifecycleControl(sessionID, operation, control, outcome, status, err)
	}
}

func (h dependencyHost) DurableExecution() durableexecution.Service {
	if h.durableExecution == nil {
		return nil
	}
	return h.durableExecution()
}

func (h dependencyHost) JavaScriptCheckpointStore(session *livesession.LiveSession) factory.JavaScriptCheckpointStore {
	if h.javaScriptCheckpointStore == nil {
		return nil
	}
	return h.javaScriptCheckpointStore(session)
}

var _ Host = dependencyHost{}

type LegacyHost interface {
	Host
	stream.Host
}

var _ factorysessions.Service = (*Service)(nil)

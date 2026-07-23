// Session host adaptation is implemented once for all transports.
package service

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// newSessionHost combines canonical state-derived callbacks with the few
// process-specific operations needed by the Session gateway.
func newSessionHost(
	state *sessionruntime.Service,
	discoverTargets func(string) ([]factorysessions.Target, error),
	initializeFactoryScaffold func(string) error,
	openLiveSessionForTarget func(context.Context, factorysessions.Target) (string, error),
	buildSessionProjectionContext func(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error),
	resolveSyncPreflightTarget func(string, *interfaces.FactorySessionLogicalResolveHint) (controlplane.SyncPreflightTarget, error),
	backendScopeID func() string,
	logicalSessionKeyID func(*livesession.LiveSession) string,
	streamGenerationID func(*livesession.LiveSession) string,
	stopLiveSession func(string) error,
	observeLiveLifecycleControl func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error),
	durableExecution func() factorysessions.ExecutionService,
	newJavaScriptCheckpointStore factory.JavaScriptCheckpointStoreFactory,
	directoryInspection roles.DirectoryInspection,
	resolveSessionFolder func(string) (string, error),
	selectTarget func([]factorysessions.Target, *factorysessions.TargetRef) (*factorysessions.Target, error),
) Host {
	host := dependencyHost{
		discoverTargets: discoverTargets, initializeFactoryScaffold: initializeFactoryScaffold,
		openLiveSessionForTarget:      openLiveSessionForTarget,
		buildSessionProjectionContext: buildSessionProjectionContext,
		resolveSyncPreflightTarget:    resolveSyncPreflightTarget,
		backendScopeID:                backendScopeID, logicalSessionKeyID: logicalSessionKeyID,
		streamGenerationID: streamGenerationID,
		stopLiveSession:    stopLiveSession, observeLiveLifecycleControl: observeLiveLifecycleControl,
		durableExecution: durableExecution, directoryInspection: directoryInspection,
		resolveSessionFolder: resolveSessionFolder,
		selectTarget:         selectTarget,
	}
	if state != nil {
		host.requireSession = func(sessionID string) (*livesession.LiveSession, error) {
			return runtimebinding.RequireLiveSession(state, sessionID)
		}
		host.listLiveSessionIDs = func() []string {
			if state.Registry() == nil {
				return nil
			}
			return state.Registry().IDs()
		}
		host.getLiveSession = state.Resolve
		host.liveSessionEvents = runtimebinding.CanonicalEventsFromSession
		host.sessionFactory = func(sessionID string) (factory.Service, error) {
			return runtimebinding.FactoryForSession(state, sessionID)
		}
		host.javaScriptCheckpointStore = func(session *livesession.LiveSession) factory.JavaScriptCheckpointStore {
			if session == nil {
				return nil
			}
			if session.JavaScriptCheckpoints == nil && newJavaScriptCheckpointStore != nil {
				session.JavaScriptCheckpoints = newJavaScriptCheckpointStore()
			}
			return session.JavaScriptCheckpoints
		}
	}
	return host
}

// Session host adaptation is implemented once for all transports.
package service

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// newSessionHost combines canonical state-derived callbacks with the few
// process-specific operations needed by the Session gateway.
func newSessionHost(
	state *sessionruntime.Service,
	buildSessionProjectionContext func(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error),
	resolveSyncPreflightTarget func(string, *interfaces.FactorySessionLogicalResolveHint) (controlplane.SyncPreflightTarget, error),
	backendScopeID func() string,
	logicalSessionKeyID func(*livesession.LiveSession) string,
	streamGenerationID func(*livesession.LiveSession) string,
	workerSessionsObservation func(string) workersessions.ObservationService,
	stopLiveSession func(string) error,
	observeLiveLifecycleControl func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error),
	durableExecution func() durableexecution.Service,
	newJavaScriptCheckpointStore factory.JavaScriptCheckpointStoreFactory,
) Host {
	host := dependencyHost{
		buildSessionProjectionContext: buildSessionProjectionContext,
		resolveSyncPreflightTarget:    resolveSyncPreflightTarget,
		backendScopeID:                backendScopeID, logicalSessionKeyID: logicalSessionKeyID,
		streamGenerationID:        streamGenerationID,
		workerSessionsObservation: workerSessionsObservation,
		stopLiveSession:           stopLiveSession, observeLiveLifecycleControl: observeLiveLifecycleControl,
		durableExecution: durableExecution,
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
			return sessionCheckpointStore(session, newJavaScriptCheckpointStore)
		}
	}
	return host
}

func sessionCheckpointStore(session *livesession.LiveSession, create factory.JavaScriptCheckpointStoreFactory) factory.JavaScriptCheckpointStore {
	if session == nil {
		return nil
	}
	if session.JavaScriptCheckpoints == nil && create != nil {
		session.JavaScriptCheckpoints = create()
	}
	return session.JavaScriptCheckpoints
}

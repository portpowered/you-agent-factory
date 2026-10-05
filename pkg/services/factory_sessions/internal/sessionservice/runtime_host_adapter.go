// Session host adaptation is implemented once for all transports.
package service

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"go.uber.org/zap"
)

// sessionLifecycleReader retains keyed facts and the existing lifecycle edge,
// without retaining a SessionRuntime or re-entering its gateway. Active and
// RuntimeRecord/Run access remain compatibility bridges owned by T15.
type sessionLifecycleReader struct {
	state            *sessionruntime.Service
	active           *runtimebinding.State
	lifecycle        factory.RuntimeLifecycle
	releaseAdmission func(string)
	logger           *zap.Logger
}

func (r sessionLifecycleReader) StopLiveSession(sessionID string) error {
	err := runtimebinding.StopSession(r.state, r.active, sessionID, func(handle factory.RuntimeRun) error {
		if handle == nil {
			return nil
		}
		if r.lifecycle == nil {
			return fmt.Errorf("factory runtime lifecycle service is required")
		}
		return r.lifecycle.Stop(handle)
	})
	if err == nil && r.releaseAdmission != nil {
		r.releaseAdmission(sessionID)
	}
	return err
}

func (r sessionLifecycleReader) ObserveLiveLifecycleControl(
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	control factorysessions.ControlRequest,
	outcome factorysessions.LifecycleControlOutcome,
	status factorysessions.LifecycleStatus,
	err error,
) {
	runtimebinding.ObserveLifecycleControl(r.logger, r.state, sessionID, operation, control, outcome, status, err)
}

func (r sessionLifecycleReader) WorkerSessionsObservationForSession(sessionID string) workersessions.ObservationService {
	var runtime factory.Service
	if r.state != nil {
		if instance, err := runtimebinding.BundleForSession(r.state, sessionID); err == nil && instance != nil {
			runtime = instance.RuntimeService()
		}
	}
	if runtime == nil {
		if instance := runtimebinding.CurrentBundle(r.state, r.active); instance != nil {
			runtime = instance.RuntimeService()
		}
		if runtime == nil && r.state != nil {
			runtime = runtimebinding.ServiceForLiveRuntime(r.state.CurrentRuntime())
		}
	}
	provider, _ := runtime.(interface {
		WorkerSessionsObservationForSession(string) workersessions.ObservationService
	})
	if provider == nil {
		return nil
	}
	return provider.WorkerSessionsObservationForSession(sessionID)
}

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
	durableExecution durableexecution.Service,
	newJavaScriptCheckpointStore factory.JavaScriptCheckpointStoreFactory,
) Host {
	host := dependencyHost{
		buildSessionProjectionContext: buildSessionProjectionContext,
		resolveSyncPreflightTarget:    resolveSyncPreflightTarget,
		backendScopeID:                backendScopeID, logicalSessionKeyID: logicalSessionKeyID,
		streamGenerationID:        streamGenerationID,
		workerSessionsObservation: workerSessionsObservation,
		stopLiveSession:           stopLiveSession, observeLiveLifecycleControl: observeLiveLifecycleControl,
		durableExecution: func() durableexecution.Service { return durableExecution },
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

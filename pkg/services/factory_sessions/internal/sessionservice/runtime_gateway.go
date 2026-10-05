package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"go.uber.org/zap"
)

type sessionGateway interface {
	roles.SessionGateway
	factorysessions.LiveControlService
	factorysessions.LiveLifecycleControlService
	JavaScriptCheckpointStore(*livesession.LiveSession) factoryruntime.JavaScriptCheckpointStore
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
}

// ResolveFactorySessionRuntimeScope resolves a public Factory Session selector
// to its canonical identity and default-session status without building a full
// read projection. Both facts are needed to scope Worker Session observations.
func (a *Assembly) ResolveFactorySessionRuntimeScope(sessionID string) (string, bool, error) {
	session := a.Resolve(sessionID)
	if session == nil {
		return "", false, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	return livesession.CanonicalID(session), session.IsDefault, nil
}

// ObserveForSession routes a status read through the live-runtime capability
// bound to the requested Factory Session.
func (s *Service) ObserveForSession(
	ctx context.Context,
	sessionID string,
	request factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	if s == nil || s.host == nil {
		return factoryruntime.ObserveResult{}, fmt.Errorf("Factory Sessions live runtime gateway is required")
	}
	runtime, err := s.host.SessionFactory(sessionID)
	if err != nil {
		return factoryruntime.ObserveResult{}, err
	}
	return runtime.Observe(ctx, request)
}

// WorkerSessionsObservationForSession resolves the opened runtime behind the
// requested Factory Session and forwards its detached Worker Sessions read
// projection. The HTTP process is built once, so session-scoped reads must not
// retain the observation service from the session that happened to start it.
func (s *Service) WorkerSessionsObservationForSession(factorySessionID string) workersessions.ObservationService {
	if s == nil || s.host == nil {
		return nil
	}
	provider, _ := s.host.(interface {
		WorkerSessionsObservationForSession(string) workersessions.ObservationService
	})
	if provider == nil {
		return nil
	}
	return provider.WorkerSessionsObservationForSession(factorySessionID)
}

// WorkerSessionsObservationForSession reads the observation service retained
// with the requested live Factory Session. The process assembly has no host:
// each opened session owns its own runtime bundle and Worker Sessions view.
func (a *Assembly) WorkerSessionsObservationForSession(factorySessionID string) workersessions.ObservationService {
	session := a.Resolve(factorySessionID)
	if state := runtimebinding.SessionStateFrom(session); state != nil {
		return state.WorkerSessionsObservation()
	}
	return nil
}

// SubscribeFactoryEventsForSession routes session-scoped observation through
// the Factory Sessions gateway.
func (s *Service) SubscribeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) (*interfaces.FactoryEventStream, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("Factory Sessions gateway is required")
	}
	scopeSessionID := strings.TrimSpace(sessionID)
	var resolvedSession *livesession.LiveSession
	if session, resolveErr := s.host.RequireSession(sessionID); resolveErr == nil && session != nil {
		resolvedSession = session
		scopeSessionID = livesession.EventScopeID(session)
	}
	runtime, err := s.host.SessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	legacyRuntime, ok := runtimebinding.WorkAndEventIngressForService(runtime)
	if !ok {
		return nil, fmt.Errorf("Factory Runtime event subscription is required until Recordings migration")
	}
	stream, err := legacyRuntime.SubscribeFactoryEvents(
		ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: scopeSessionID},
	)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	if stream != nil {
		stream.BackendScopeID = strings.TrimSpace(s.host.BackendScopeID())
		if resolvedSession != nil {
			stream.LogicalSessionKeyID = strings.TrimSpace(s.host.LogicalSessionKeyID(resolvedSession))
			stream.FactorySessionID = livesession.CanonicalID(resolvedSession)
		}
	}
	return stream, nil
}

// ProbeFactoryEventsForSession validates a reconnect cursor while retaining
// ownership of the short-lived subscription and its cancellation lifecycle.
func (s *Service) ProbeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) error {
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err := s.SubscribeFactoryEventsForSession(probeCtx, sessionID, reconnect)
	return err
}

func (a *Assembly) ListSessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	scope := request.Scope
	if scope == "" {
		scope = factorysessions.DefaultSessionListScope
	}
	request.Scope = scope
	result := factorysessions.ListSessionsResult{Scope: scope}
	if shouldIncludeRecordedHistory(scope, request.ExcludeRecordedHistory) {
		if a == nil || a.recordedHistory == nil {
			if scope == factorysessions.SessionListScopeHistory {
				return factorysessions.ListSessionsResult{}, fmt.Errorf("recorded session inventory is required")
			}
		} else {
			recorded, err := a.recordedHistory.ListSessions(ctx, request)
			if err != nil {
				return factorysessions.ListSessionsResult{}, err
			}
			result.RecordedSessions = recorded.RecordedSessions
		}
	}
	if scope == factorysessions.SessionListScopeHistory {
		return result, nil
	}
	if a == nil {
		return factorysessions.ListSessionsResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	if scope == factorysessions.SessionListScopeLive || scope == factorysessions.SessionListScopeAll {
		result.LiveSessions = a.listLiveSessions()
	}
	if scope == factorysessions.SessionListScopePersisted || scope == factorysessions.SessionListScopeAll {
		durable, err := a.listPersistedSessions(ctx, request)
		if err != nil {
			return factorysessions.ListSessionsResult{}, err
		}
		result.DurableSessions = durable
	}
	return result, nil
}

func (a *Assembly) listLiveSessions() []factorysessions.LiveSessionSummary {
	var sessions []factorysessions.LiveSessionSummary
	for _, id := range a.ListLiveSessionIDs() {
		session := a.Resolve(id)
		if session == nil {
			continue
		}
		placement := session.Placement()
		sessions = append(sessions, factorysessions.LiveSessionSummary{
			ID: livesession.CanonicalID(session), FactoryDir: session.FactoryDir,
			FolderPath: placement.FolderPath, Project: placement.Project, IsDefault: session.IsDefault,
		})
	}
	return sessions
}

func (a *Assembly) listPersistedSessions(ctx context.Context, request factorysessions.ListSessionsRequest) ([]factorysessions.DurableSessionListSummary, error) {
	if a.SessionGateway == nil {
		return nil, factorysessions.ErrExecutionServiceNotConfigured
	}
	durable, err := a.SessionGateway.ListSessions(ctx, factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopePersisted, Filters: request.Filters,
		ExcludeRecordedHistory: true,
	})
	if err != nil && !(request.Scope == factorysessions.SessionListScopeAll && errors.Is(err, factorysessions.ErrExecutionServiceNotConfigured)) {
		return nil, err
	}
	return durable.DurableSessions, nil
}

func shouldIncludeRecordedHistory(scope factorysessions.SessionListScope, excluded bool) bool {
	return scope == factorysessions.SessionListScopeHistory ||
		(scope == factorysessions.SessionListScopeAll && !excluded)
}

// ReadDurableFactorySessionEventStream reads and materializes one finite
// durable event stream behind the Factory Sessions boundary.
func (s *Service) ReadDurableFactorySessionEventStream(
	ctx context.Context,
	sessionID string,
	reconnect factorysessions.EventReconnectRequest,
) (*interfaces.FactoryEventStream, error) {
	if s == nil || s.durable == nil {
		return nil, factorysessions.ErrExecutionServiceNotConfigured
	}
	result, err := s.durable.ReadEvents(ctx, sessionID, reconnect)
	if err != nil {
		return nil, err
	}
	return factorysessions.MaterializeEventReadStream(result), nil
}

// ProbeDurableFactorySessionEvents validates one finite durable reconnect
// cursor without constructing a stream for a transport to discard.
func (s *Service) ProbeDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	reconnect factorysessions.EventReconnectRequest,
) error {
	if s == nil || s.durable == nil {
		return factorysessions.ErrExecutionServiceNotConfigured
	}
	_, err := s.durable.ReadEvents(ctx, sessionID, reconnect)
	return err
}

var _ roles.SessionGateway = (*Service)(nil)

// AttachSessionGateway installs the Wire-constructed gateway used by all
// SessionRuntime operations. It returns the same gateway for provider chaining.
func (fs *SessionRuntime) AttachSessionGateway(gateway *Service) *Service {
	if fs != nil && gateway != nil {
		fs.sessionGateway = gateway
	}
	return gateway
}

// Gateway returns the single gateway attached to this Factory Session runtime.
func (fs *SessionRuntime) Gateway() roles.SessionGateway {
	return fs.requireSessionGateway()
}

// ReconnectCursorValidator exposes the injected Recordings capability to the
// gateway constructor without exposing the concrete ledger implementation.
func (fs *SessionRuntime) ReconnectCursorValidator() factorysessions.ReconnectCursorValidator {
	if fs == nil {
		return nil
	}
	return fs.reconnectCursorValidator
}

// SessionServiceHost constructs keyed gateway reads and lifecycle effects from
// explicit collaborators. Active record facts remain the T15 compatibility bridge.
func SessionServiceHost(
	state *sessionruntime.Service,
	active *runtimebinding.State,
	control SessionScopeControl,
	releaseAdmission func(string),
	durable durableexecution.Service,
	backendScope string,
	identityService identity.Service,
	clock factoryruntime.Clock,
	projector factoryruntime.WorldStateProjector,
	checkpoints factoryruntime.JavaScriptCheckpointStoreFactory,
	logger *zap.Logger,
) Host {
	routing := sessionIdentityReader{state: state, active: active, backendScope: backendScope, identity: identityService}
	projection := sessionProjectionReader{state: state, backendScope: backendScope, identity: identityService, clock: clock, projector: projector, checkpoints: checkpoints}
	lifecycleReader := sessionLifecycleReader{state: state, active: active, control: control, releaseAdmission: releaseAdmission, logger: logger}
	return keyedSessionHost{
		sessionIdentityReader:   routing,
		sessionProjectionReader: projection,
		sessionLifecycleReader:  lifecycleReader,
		state:                   state, durable: durable,
	}
}

func (fs *SessionRuntime) requireSessionGateway() sessionGateway {
	if fs == nil {
		return nil
	}
	return fs.sessionGateway
}

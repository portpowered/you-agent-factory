package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"go.uber.org/zap"
)

type sessionGateway interface {
	factorysessions.Service
	factorysessions.LiveControlService
	JavaScriptCheckpointStore(*livesession.LiveSession) factoryruntime.JavaScriptCheckpointStore
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
}

// ResolveFactorySessionRuntimeID resolves a public Factory Session selector
// to the canonical runtime identity without building the full read projection.
// Transport adapters use this narrow capability for identity-only routing.
func (a *Assembly) ResolveFactorySessionRuntimeID(sessionID string) (string, error) {
	session := a.Resolve(sessionID)
	if session == nil {
		return "", fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	return livesession.CanonicalID(session), nil
}

// ObserveForSession routes a status read through the live-runtime capability
// bound to the requested Factory Session.
func (s *Service) ObserveForSession(
	ctx context.Context,
	sessionID string,
	request factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	if s == nil || s.liveRuntime == nil {
		return factoryruntime.ObserveResult{}, fmt.Errorf("Factory Sessions live runtime gateway is required")
	}
	return s.liveRuntime.Observe(ctx, sessionID, request)
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
	runtime, err := s.host.SessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	legacyRuntime, ok := runtimebinding.WorkAndEventIngressForService(runtime)
	if !ok {
		return nil, fmt.Errorf("Factory Runtime event subscription is required until Recordings migration")
	}
	stream, err := legacyRuntime.SubscribeFactoryEvents(
		ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID},
	)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	if stream != nil {
		stream.BackendScopeID = strings.TrimSpace(s.host.BackendScopeID())
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
		if scope == factorysessions.SessionListScopeHistory && (a == nil || a.recordedSessionInventory == nil) {
			return factorysessions.ListSessionsResult{}, fmt.Errorf("recorded session inventory is required")
		}
		recorded, err := a.listRecordedSessions()
		if err != nil {
			return factorysessions.ListSessionsResult{}, err
		}
		result.RecordedSessions = recorded
	}
	if scope == factorysessions.SessionListScopeHistory {
		return result, nil
	}
	return a.mergeDetachedSessionList(ctx, request, result)
}

func shouldIncludeRecordedHistory(scope factorysessions.SessionListScope, excluded bool) bool {
	return scope == factorysessions.SessionListScopeHistory ||
		(scope == factorysessions.SessionListScopeAll && !excluded)
}

func (a *Assembly) listRecordedSessions() ([]factorysessions.RecordedSessionListSummary, error) {
	if a == nil || a.recordedSessionInventory == nil {
		return nil, nil
	}
	root, err := a.recordingRoot()
	if err != nil {
		return nil, err
	}
	listed, err := a.recordedSessionInventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{
		RecordingRoot: root,
	})
	if err != nil {
		return nil, fmt.Errorf("list recorded Factory Sessions: %w", err)
	}
	result := make([]factorysessions.RecordedSessionListSummary, 0, len(listed.Sessions))
	for _, session := range listed.Sessions {
		result = append(result, factorysessions.RecordedSessionListSummary{
			SessionID:         session.FactorySessionID,
			Source:            factorysessions.RecordedSessionListSourceHistory,
			ArtifactReference: session.ArtifactReference,
			Format:            string(session.Format),
		})
	}
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].SessionID != result[right].SessionID {
			return result[left].SessionID < result[right].SessionID
		}
		return result[left].ArtifactReference < result[right].ArtifactReference
	})
	return result, nil
}

func (a *Assembly) mergeDetachedSessionList(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
	result factorysessions.ListSessionsResult,
) (factorysessions.ListSessionsResult, error) {
	owners := a.detachedOwners()
	if len(owners) == 0 {
		if a != nil && a.recordedSessionInventory != nil && request.Scope == factorysessions.SessionListScopeAll {
			return result, nil
		}
		return factorysessions.ListSessionsResult{}, factorysessions.ErrDetachedServiceUnavailable
	}
	seenLive := make(map[string]struct{})
	seenDurable := make(map[string]struct{})
	for _, owner := range owners {
		listed, err := owner.ListSessions(ctx, request)
		if err != nil {
			return factorysessions.ListSessionsResult{}, err
		}
		if result.Scope == "" {
			result.Scope = listed.Scope
		}
		appendUniqueLiveSessions(&result, seenLive, listed.LiveSessions)
		appendUniqueDurableSessions(&result, seenDurable, listed.DurableSessions)
	}
	return result, nil
}

func appendUniqueLiveSessions(
	result *factorysessions.ListSessionsResult,
	seen map[string]struct{},
	sessions []factorysessions.LiveSessionSummary,
) {
	for _, session := range sessions {
		if _, exists := seen[session.ID]; exists {
			continue
		}
		seen[session.ID] = struct{}{}
		result.LiveSessions = append(result.LiveSessions, session)
	}
}

func appendUniqueDurableSessions(
	result *factorysessions.ListSessionsResult,
	seen map[string]struct{},
	sessions []factorysessions.DurableSessionListSummary,
) {
	for _, session := range sessions {
		if _, exists := seen[session.SessionID]; exists {
			continue
		}
		seen[session.SessionID] = struct{}{}
		result.DurableSessions = append(result.DurableSessions, session)
	}
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

func (fs *SessionRuntime) durableExecutionService() durableexecution.Service {
	if fs == nil {
		return nil
	}
	return fs.durableExecution
}

func (fs *SessionRuntime) observeLiveLifecycleControl(
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	control factorysessions.ControlRequest,
	outcome factorysessions.LifecycleControlOutcome,
	status factorysessions.LifecycleStatus,
	err error,
) {
	if fs == nil {
		return
	}
	runtimebinding.ObserveLifecycleControl(fs.logger, fs.sessionState, sessionID, operation, control, outcome, status, err)
}

var _ factorysessions.Service = (*Service)(nil)

func newSessionGatewayService(fs *SessionRuntime) *Service {
	if fs == nil || fs.sessionState == nil {
		return nil
	}
	state := fs.sessionState
	streams := state.ResponseStreams()
	return NewWithResponseService(
		SessionServiceHost(fs),
		state,
		sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle),
		streams,
		fs.ReconnectCursorValidator(),
		fs.sessionResultProjection,
		state.ResponseEventService(),
	)
}

// AttachSessionGateway installs the Wire-constructed gateway used by all
// SessionRuntime operations. It returns the same gateway for provider chaining.
func (fs *SessionRuntime) AttachSessionGateway(gateway *Service) *Service {
	if fs != nil && gateway != nil {
		fs.sessionGateway = gateway
	}
	return gateway
}

// Gateway returns the single gateway attached to this Factory Session runtime.
func (fs *SessionRuntime) Gateway() factorysessions.Service {
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

// SessionServiceHost exposes the runtime's lifecycle callbacks to the bounded
// Session gateway.
func SessionServiceHost(runtime *SessionRuntime) Host {
	initializeFactoryScaffold := func(factoryDir string) error {
		if runtime == nil {
			return sessionvalidation.New(
				factorysessions.ValidationReasonUnreadable,
				"folderPath",
				fmt.Errorf("initialize factory scaffold: initializer is required"),
			)
		}
		initialize := runtime.factoryScaffoldInitializer
		if initialize == nil {
			return sessionvalidation.New(
				factorysessions.ValidationReasonUnreadable,
				"folderPath",
				fmt.Errorf("initialize factory scaffold: initializer is required"),
			)
		}
		if err := initialize(factoryDir); err != nil {
			return sessionvalidation.New(
				factorysessions.ValidationReasonUnreadable,
				"folderPath",
				fmt.Errorf("initialize factory scaffold: %w", err),
			)
		}
		return nil
	}
	if runtime == nil {
		return newSessionHost(
			nil, nil, initializeFactoryScaffold, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		)
	}
	discoverTargets := func(folderPath string) ([]factorysessions.Target, error) {
		return runtime.identity.Discover(context.Background(), identity.DiscoverRequest{
			FolderPath: folderPath, WorkstationLoader: runtime.workstationLoader,
			LoadFactory: runtime.loadFactory, Logger: runtime.logger,
		})
	}
	resolveSessionFolder := func(folderPath string) (string, error) {
		return runtime.identity.ResolveFolder(folderPath)
	}
	resolveSyncPreflightTarget := func(
		sessionID string,
		logicalResolve *interfaces.FactorySessionLogicalResolveHint,
	) (controlplane.SyncPreflightTarget, error) {
		target, err := runtime.resolveSessionSyncPreflightTarget(sessionID, logicalResolve)
		return controlplane.SyncPreflightTarget{
			Session: target.session, Remapped: target.remapped, Unresolved: target.unresolved,
		}, err
	}
	backendScopeID := func() string {
		return runtimebinding.BackendScopeID(runtime.backendScopeID, nil)
	}
	logicalSessionKeyID := func(session *livesession.LiveSession) string {
		if session == nil {
			return ""
		}
		resolved, err := runtime.identity.Normalize(context.Background(), identity.NormalizeRequest{
			BackendScopeID: backendScopeID(), FolderPath: session.FolderPath, Target: session.Target,
		})
		if err != nil {
			return ""
		}
		return resolved.LogicalSessionKeyID
	}
	streamGenerationID := func(session *livesession.LiveSession) string {
		return runtimebinding.StreamGenerationID(session)
	}
	return newSessionHost(
		runtime.sessionState,
		discoverTargets,
		initializeFactoryScaffold,
		runtime.openFactorySessionForTarget,
		runtime.buildSessionProjectionContext,
		resolveSyncPreflightTarget,
		backendScopeID,
		logicalSessionKeyID,
		streamGenerationID,
		runtime.WorkerSessionsObservationForSession,
		runtime.stopFactorySession,
		runtime.observeLiveLifecycleControl,
		runtime.durableExecutionService,
		runtime.newJavaScriptCheckpointStore,
		runtime.directoryInspection,
		resolveSessionFolder,
		runtime.identity.Select,
	)
}

func (fs *SessionRuntime) requireSessionGateway() sessionGateway {
	if fs == nil {
		return newSessionGatewayService(nil)
	}
	if fs.sessionGateway == nil {
		fs.sessionGateway = newSessionGatewayService(fs)
	}
	return fs.sessionGateway
}

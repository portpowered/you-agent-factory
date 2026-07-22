package service

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/observations"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/identity"
	"go.uber.org/zap"
)

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
	stream, err := runtime.SubscribeFactoryEvents(
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

// GetEngineStateSnapshotForSession returns one live session's runtime snapshot.
func (s *Service) GetEngineStateSnapshotForSession(
	ctx context.Context,
	sessionID string,
) (*factoryruntime.StateSnapshot, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("Factory Sessions gateway is required")
	}
	return s.liveRuntime.Snapshot(ctx, sessionID)
}

func (fs *SessionRuntime) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	return fs.requireSessionGateway().SubscribeSessionResponseStream(sessionID, dispatchID, afterSequence)
}

func (fs *SessionRuntime) SessionResponseStreamDispatchIDs(sessionID string) ([]string, error) {
	return fs.requireSessionGateway().SessionResponseStreamDispatchIDs(sessionID)
}

func (fs *SessionRuntime) inferenceProgressPublisher(
	sessionID string,
	logger *zap.Logger,
) observations.ProgressPublisher {
	if fs == nil {
		return nil
	}
	factory := fs.requireSessionGateway().InferenceProgressPublisherFactory(logger)
	if factory == nil {
		return nil
	}
	return factory(sessionID)
}

func (fs *SessionRuntime) durableExecutionService() factorysessions.ExecutionService {
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
	return NewWithReconnectValidation(
		SessionServiceHost(fs),
		state,
		sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle),
		streams,
		fs.ReconnectCursorValidator(),
		fs.sessionResultProjection,
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
			return factorysessions.NewValidationError(
				factorysessions.ValidationReasonUnreadable,
				"folderPath",
				fmt.Errorf("initialize factory scaffold: initializer is required"),
			)
		}
		initialize := runtime.factoryScaffoldInitializer
		if initialize == nil {
			return factorysessions.NewValidationError(
				factorysessions.ValidationReasonUnreadable,
				"folderPath",
				fmt.Errorf("initialize factory scaffold: initializer is required"),
			)
		}
		if err := initialize(factoryDir); err != nil {
			return factorysessions.NewValidationError(
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
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
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
	logicalSessionKeyID := func(session *factorysessions.LiveSession) string {
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
	streamGenerationID := func(session *factorysessions.LiveSession) string {
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
		runtime.stopFactorySession,
		runtime.observeLiveLifecycleControl,
		runtime.durableExecutionService,
		runtime.newJavaScriptCheckpointStore,
		runtime.directoryInspection,
		resolveSessionFolder,
		runtime.identity.Select,
	)
}

func (fs *SessionRuntime) requireSessionGateway() factorysessions.Service {
	if fs == nil {
		return newSessionGatewayService(nil)
	}
	if fs.sessionGateway == nil {
		fs.sessionGateway = newSessionGatewayService(fs)
	}
	return fs.sessionGateway
}

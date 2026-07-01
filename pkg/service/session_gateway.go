package service

import (
	"context"
	"fmt"
	"io"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

// sessionGateway is the injectable session gateway collaborator seam.
type sessionGateway interface {
	OpenFactorySession(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	OpenFactorySessionFromFolder(context.Context, string, *FactorySessionTargetRef, bool, bool) (*FactorySessionOpenResult, error)
	ListFactorySessions(context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(context.Context, string) (factoryapi.FactorySession, error)
	GetFactorySessionSyncPreflight(context.Context, string, *interfaces.FactoryEventReconnectCursor) (factoryapi.FactorySessionSyncPreflightResponse, error)
	GetFactorySessionResult(context.Context, string) (factoryapi.FactorySessionLiveResult, error)
	GetFactorySessionPartialResult(context.Context, string) (factoryapi.FactorySessionPartialResult, error)
	PauseLiveFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeLiveFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CloseFactorySession(context.Context, string) error
	PauseDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CancelDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	TerminateDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ApproveDurableFactorySession(context.Context, string, factoryapi.FactorySessionApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	RetryDurableFactorySessionDispatch(context.Context, string, factoryapi.FactorySessionRetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	InterruptDurableFactorySessionDispatch(context.Context, string, factoryapi.FactorySessionInterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	SubscribeSessionResponseStream(sessionID string, dispatchID string, afterSequence int64) (*factorysessions.SessionResponseStreamSubscription, error)
	SessionResponseStreamDispatchIDs(sessionID string) ([]string, error)
	CloseSessionResponseStreams(session *factorysessions.LiveSession)
	JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore
	InferenceProgressPublisherFactory(logger *zap.Logger) func(sessionID string) workerprovider.InferenceProgressPublisher
	DispatchCompletionObserverFactory() func(sessionID string) func(string)
}

var _ sessionGateway = (*factorysessionservice.Service)(nil)

type sessionGatewayHost struct {
	*FactoryService
}

var _ factorysessionservice.Host = sessionGatewayHost{}

func (h sessionGatewayHost) DiscoverTargets(folderPath string) ([]factorysessions.Target, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.discoverFactorySessionTargets(folderPath)
}

func (h sessionGatewayHost) InitializeFactoryScaffold(factoryDir string) error {
	if err := initcmd.Init(initcmd.InitConfig{
		Dir:         factoryDir,
		Diagnostics: io.Discard,
	}); err != nil {
		return factorysessions.NewValidationError(
			factorysessions.ValidationReasonUnreadable,
			"folderPath",
			fmt.Errorf("initialize factory scaffold: %w", err),
		)
	}
	return nil
}

func (h sessionGatewayHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if h.FactoryService == nil {
		return "", fmt.Errorf("factory service is required")
	}
	return h.openFactorySessionForTarget(ctx, target)
}

func (h sessionGatewayHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.requireSession(sessionID)
}

func (h sessionGatewayHost) ListLiveSessionIDs() []string {
	if h.FactoryService == nil || h.FactoryService.sessions == nil {
		return nil
	}
	return h.FactoryService.sessions.IDs()
}

func (h sessionGatewayHost) GetLiveSession(sessionID string) *factorysessions.LiveSession {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.sessionByID(sessionID)
}

func (h sessionGatewayHost) BuildSessionProjectionContext(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.FactoryService == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.buildSessionProjectionContext(ctx, session)
}

func (h sessionGatewayHost) ResolveSyncPreflightTarget(sessionID string) (controlplane.SyncPreflightTarget, error) {
	if h.FactoryService == nil {
		return controlplane.SyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	target, err := h.FactoryService.resolveSessionSyncPreflightTarget(sessionID)
	return controlplane.SyncPreflightTarget{Session: target.session, Remapped: target.remapped}, err
}

func (h sessionGatewayHost) BackendScopeID() string {
	if h.FactoryService == nil {
		return ""
	}
	return factorySessionBackendScopeID(h.FactoryService, nil)
}

func (h sessionGatewayHost) StreamGenerationID(session *factorysessions.LiveSession) string {
	if h.FactoryService == nil {
		return ""
	}
	return factorySessionStreamGenerationID(h.FactoryService, session)
}

func (h sessionGatewayHost) LiveSessionEvents(session *factorysessions.LiveSession) []factoryapi.FactoryEvent {
	handle := liveSessionHandle(session)
	if handle == nil || handle.runtime == nil || handle.runtime.eventHistory == nil {
		return nil
	}
	return handle.runtime.eventHistory.Events()
}

func (h sessionGatewayHost) SessionFactory(sessionID string) (factory.Factory, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.sessionFactory(sessionID)
}

func (h sessionGatewayHost) StopLiveSession(sessionID string) error {
	if h.FactoryService == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.FactoryService.stopFactorySession(sessionID)
}

func (h sessionGatewayHost) ObserveLiveLifecycleControl(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
	err error,
) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeLiveLifecycleControl(sessionID, operation, control, outcome, status, err)
}

func (h sessionGatewayHost) DurableExecution() factorysessionexecution.Service {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.durableExecutionService()
}

func (h sessionGatewayHost) ResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.sessionResponseStreams(session)
}

func (h sessionGatewayHost) NewResponseStream() *factorysessions.SessionResponseStream {
	if h.FactoryService == nil {
		return factorysessions.NewSessionResponseStream()
	}
	return h.FactoryService.newSessionResponseStreamInstance()
}

func (h sessionGatewayHost) CloseResponseStreams(session *factorysessions.LiveSession) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.closeSessionResponseStreamsDirect(session)
}

func (h sessionGatewayHost) CloseResponseStreamDispatch(session *factorysessions.LiveSession, dispatchID string) bool {
	if h.FactoryService == nil {
		return false
	}
	return h.FactoryService.closeSessionResponseStreamDispatchDirect(session, dispatchID)
}

func (h sessionGatewayHost) JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.javascriptCheckpointStoreDirect(session)
}

func (h sessionGatewayHost) ObserveResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeResponseStreamPublished(session, sessionID, event)
}

func (h sessionGatewayHost) ObserveResponseStreamCompaction(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	summary responsestream.CompactionSummary,
) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeResponseStreamCompaction(session, sessionID, dispatchID, summary)
}

func (h sessionGatewayHost) ObserveResponseStreamDegraded(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	reason string,
	fallbackLogger *zap.Logger,
	err error,
) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeResponseStreamDegraded(session, sessionID, dispatchID, reason, fallbackLogger, err)
}

func newSessionGatewayService(fs *FactoryService) *factorysessionservice.Service {
	return factorysessionservice.New(sessionGatewayHost{fs})
}

func wireSessionGatewayCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) sessionGateway {
	if cfg != nil && cfg.SessionGateway != nil {
		return cfg.SessionGateway
	}
	return newSessionGatewayService(fs)
}

func (fs *FactoryService) requireSessionGateway() sessionGateway {
	if fs == nil {
		return newSessionGatewayService(nil)
	}
	if fs.sessionGateway == nil {
		fs.sessionGateway = newSessionGatewayService(fs)
	}
	return fs.sessionGateway
}

// ProvideSessionGatewayCollaborator constructs the session gateway for a built service shell.
func ProvideSessionGatewayCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) sessionGateway {
	return wireSessionGatewayCollaborator(shell.Service, cfg)
}

// AttachSessionGatewayCollaborator assigns the session gateway on the service shell.
func AttachSessionGatewayCollaborator(shell FactoryServiceShell, gateway sessionGateway) *FactoryService {
	if shell.Service != nil {
		shell.Service.sessionGateway = gateway
	}
	return shell.Service
}

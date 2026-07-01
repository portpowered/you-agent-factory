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
)

// sessionGateway is the injectable session gateway collaborator seam.
type sessionGateway interface {
	OpenFactorySession(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	OpenFactorySessionFromFolder(context.Context, string, *FactorySessionTargetRef, bool, bool) (*FactorySessionOpenResult, error)
	ListFactorySessions(context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(context.Context, string) (factoryapi.FactorySession, error)
	PauseLiveFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeLiveFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CloseFactorySession(context.Context, string) error
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

package service

import (
	"context"
	"fmt"
	"io"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
)

// sessionGatewayOpener is the injectable session-open collaborator seam.
type sessionGatewayOpener interface {
	OpenFactorySession(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	OpenFactorySessionFromFolder(context.Context, string, *FactorySessionTargetRef, bool, bool) (*FactorySessionOpenResult, error)
}

var _ sessionGatewayOpener = (*factorysessionservice.Service)(nil)

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

func newSessionGatewayService(fs *FactoryService) *factorysessionservice.Service {
	return factorysessionservice.New(sessionGatewayHost{fs})
}

func wireSessionGatewayCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) sessionGatewayOpener {
	if cfg != nil && cfg.SessionGateway != nil {
		return cfg.SessionGateway
	}
	return newSessionGatewayService(fs)
}

func (fs *FactoryService) requireSessionGateway() sessionGatewayOpener {
	if fs == nil {
		return newSessionGatewayService(nil)
	}
	if fs.sessionGateway == nil {
		fs.sessionGateway = newSessionGatewayService(fs)
	}
	return fs.sessionGateway
}

// ProvideSessionGatewayCollaborator constructs the session gateway for a built service shell.
func ProvideSessionGatewayCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) sessionGatewayOpener {
	return wireSessionGatewayCollaborator(shell.Service, cfg)
}

// AttachSessionGatewayCollaborator assigns the session gateway on the service shell.
func AttachSessionGatewayCollaborator(shell FactoryServiceShell, gateway sessionGatewayOpener) *FactoryService {
	if shell.Service != nil {
		shell.Service.sessionGateway = gateway
	}
	return shell.Service
}

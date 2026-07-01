// Package service provides compatibility aliases and composition helpers for the
// extracted runtime host. Authoritative runtime/session ownership lives in
// pkg/runtimehost.
package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"go.uber.org/zap"
)

const defaultFactorySessionID = runtimehost.DefaultFactorySessionID

func ProvideModelServiceCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) ModelService {
	return runtimehost.ProvideModelServiceCollaborator(shell, cfg)
}

func AttachModelServiceCollaborator(shell FactoryServiceShell, modelAPI ModelService) *FactoryService {
	return runtimehost.AttachModelServiceCollaborator(shell, modelAPI)
}

func ProvideFactorySaveCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) runtimehost.FactorySaveSaver {
	return runtimehost.ProvideFactorySaveCollaborator(shell, cfg)
}

func AttachFactorySaveCollaborator(shell FactoryServiceShell, factorySave runtimehost.FactorySaveSaver) *FactoryService {
	return runtimehost.AttachFactorySaveCollaborator(shell, factorySave)
}

func ProvideSessionGatewayCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) runtimehost.SessionGateway {
	return runtimehost.ProvideSessionGatewayCollaborator(shell, cfg)
}

func AttachSessionGatewayCollaborator(shell FactoryServiceShell, gateway runtimehost.SessionGateway) *FactoryService {
	return runtimehost.AttachSessionGatewayCollaborator(shell, gateway)
}

// ValidateReplayModeConfig rejects incompatible replay and record mode combinations.
func ValidateReplayModeConfig(cfg *FactoryServiceConfig) error {
	return runtimehost.ValidateReplayModeConfig(cfg)
}

func newInferenceProgressPublisherFactory(sessions *factorysessions.Registry, logger *zap.Logger) runtimehost.InferenceProgressPublisherFactory {
	return runtimehost.NewInferenceProgressPublisherFactory(sessions, logger)
}

type (
	FactoryService              = runtimehost.Host
	FactoryCore                 = runtimehost.Core
	FactoryServiceConfig        = runtimehost.Config
	FactoryServiceShell         = runtimehost.HostShell
	ComposeCollaboratorSnapshot = runtimehost.ComposeCollaboratorSnapshot
	LocalModelDomain            = runtimehost.LocalModelDomain
	FactoryDefinitionService    = runtimehost.FactoryDefinitionService
	SimpleDashboardRenderInput  = runtimehost.SimpleDashboardRenderInput
	SimpleDashboardRenderer     = runtimehost.SimpleDashboardRenderer
	APIServerStarter            = runtimehost.APIServerStarter
	RuntimeFileLoggingPolicy    = runtimehost.RuntimeFileLoggingPolicy
	RuntimeLogDiagnostics       = runtimehost.RuntimeLogDiagnostics
	InvocationMetricsRecorder   = runtimehost.InvocationMetricsRecorder
	InvocationMetric            = runtimehost.InvocationMetric
	ModelPullMetricsRecorder    = runtimehost.ModelPullMetricsRecorder
)

var (
	ErrFactoryActivationRequiresIdle = runtimehost.ErrFactoryActivationRequiresIdle
	ErrInvalidNamedFactoryName       = runtimehost.ErrInvalidNamedFactoryName
	ErrInvalidNamedFactory           = runtimehost.ErrInvalidNamedFactory
	ErrCurrentFactoryNotFound        = runtimehost.ErrCurrentFactoryNotFound
)

const (
	RuntimeFileLoggingPolicyEnabled  = runtimehost.RuntimeFileLoggingPolicyEnabled
	RuntimeFileLoggingPolicyDisabled = runtimehost.RuntimeFileLoggingPolicyDisabled
)

// NewFactoryServiceFromCore wraps a composed core in the compatibility alias.
func NewFactoryServiceFromCore(core *FactoryCore) *FactoryService {
	return runtimehost.NewHostFromCore(core)
}

// BuildFactoryService loads factory configuration and constructs the compatibility host.
func BuildFactoryService(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryService, error) {
	core, err := BuildFactoryCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	host := NewFactoryServiceFromCore(core)
	shell := FactoryServiceShell{Host: host}
	host = AttachModelServiceCollaborator(shell, ProvideModelServiceCollaborator(shell, cfg))
	host = AttachFactorySaveCollaborator(
		FactoryServiceShell{Host: host},
		ProvideFactorySaveCollaborator(FactoryServiceShell{Host: host}, cfg),
	)
	return AttachSessionGatewayCollaborator(
		FactoryServiceShell{Host: host},
		ProvideSessionGatewayCollaborator(FactoryServiceShell{Host: host}, cfg),
	), nil
}

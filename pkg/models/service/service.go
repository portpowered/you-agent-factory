package service

import (
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"go.uber.org/zap"
)

// Dependencies carries runtime inputs for model-domain catalog operations.
type Dependencies struct {
	RuntimeConfig           func() *factoryconfig.LoadedFactoryConfig
	ModelHost               func() modelhost.Host
	ModelAssetPuller        func() localmodels.AssetPuller
	Logger                  func() *zap.Logger
	ModelPullMetrics        func() PullMetricsRecorder
	ModelInvocationExecutor ModelInvocationExecutor
	FactoryRunnerID         func() string
}

// Service owns direct model catalog, pull, and invocation behavior.
type Service struct {
	deps Dependencies
}

// New constructs a model-domain service with explicit dependencies.
func New(deps Dependencies) *Service {
	return &Service{deps: deps}
}

func (s *Service) runtimeConfig() *factoryconfig.LoadedFactoryConfig {
	if s == nil || s.deps.RuntimeConfig == nil {
		return nil
	}
	return s.deps.RuntimeConfig()
}

func (s *Service) modelHost() modelhost.Host {
	if s == nil || s.deps.ModelHost == nil {
		return nil
	}
	return s.deps.ModelHost()
}

package service

import (
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"go.uber.org/zap"
)

// Dependencies carries runtime inputs for model-domain catalog operations.
type Dependencies struct {
	RuntimeConfig           func() *factoryconfig.LoadedFactoryConfig
	ModelHost               modelhost.Host
	ModelAssetPuller        localmodels.AssetPuller
	Logger                  *zap.Logger
	Clock                   func() time.Time
	ModelPullMetrics        PullMetricsRecorder
	ModelInvocationExecutor ModelInvocationExecutor
	FactoryRunnerID         string
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
	if s == nil {
		return nil
	}
	return s.deps.ModelHost
}

func (s *Service) now() time.Time {
	if s == nil || s.deps.Clock == nil {
		return time.Now()
	}
	return s.deps.Clock()
}

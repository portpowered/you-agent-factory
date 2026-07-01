package service

import (
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"go.uber.org/zap"
)

// Host exposes runtime seams owned by pkg/service for model-domain wiring.
type Host interface {
	RuntimeConfig() func() *factoryconfig.LoadedFactoryConfig
	ModelHost() func() modelhost.Host
	ModelAssetPuller() func() localmodels.AssetPuller
	Logger() func() *zap.Logger
	ModelPullMetrics() func() PullMetricsRecorder
	ModelInvocationExecutor() ModelInvocationExecutor
	FactoryRunnerID() func() string
}

// NewFromHost constructs the canonical model-domain service from explicit host seams.
func NewFromHost(host Host) *Service {
	if host == nil {
		return New(Dependencies{})
	}
	return New(Dependencies{
		RuntimeConfig:           host.RuntimeConfig(),
		ModelHost:               host.ModelHost(),
		ModelAssetPuller:        host.ModelAssetPuller(),
		Logger:                  host.Logger(),
		ModelPullMetrics:        host.ModelPullMetrics(),
		ModelInvocationExecutor: host.ModelInvocationExecutor(),
		FactoryRunnerID:         host.FactoryRunnerID(),
	})
}

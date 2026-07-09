package service

import (
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"go.uber.org/zap"
)

// Host is a temporary migration adapter, not the final dependency-injection
// pattern for model-domain wiring. New model catalog, pull, or invocation
// wiring should pass explicit collaborators to models/service.New(Dependencies)
// instead of adding seams to a broad host object.
type Host interface {
	RuntimeConfig() func() *factoryconfig.LoadedFactoryConfig
	ModelHost() func() modelhost.Host
	ModelAssetPuller() func() localmodels.AssetPuller
	Logger() func() *zap.Logger
	ModelPullMetrics() func() PullMetricsRecorder
	ModelInvocationExecutor() ModelInvocationExecutor
	FactoryRunnerID() func() string
}

// NewFromHost is a temporary migration adapter that converts a broad host
// object into the canonical Dependencies shape. Prefer direct construction
// through models/service.New(Dependencies) for new model-service wiring.
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

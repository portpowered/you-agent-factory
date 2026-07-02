// Package service exposes compose-bridge helpers for pkg/initializer startup
// composition. Runtime bundle construction internals remain here until a
// dedicated runtime-bundle package split; transports must not call these.
package service

import (
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"go.uber.org/zap"
)

// EnsureBackendScopeForCompose resolves backend scope before core composition.
func EnsureBackendScopeForCompose(cfg *runtimehost.Config, logger *zap.Logger) error {
	return ensureServiceBackendScope(cfg, logger)
}

// NewRuntimeBuildServiceForCompose constructs the runtimebuild collaborator for
// initializer-owned core composition.
func NewRuntimeBuildServiceForCompose(
	cfg *runtimehost.Config,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels *LocalModelDomain,
	sessions *factorysessions.Registry,
) *runtimebuild.Service {
	return newRuntimeBuildService(
		cfg,
		clock,
		baseLogger,
		localModels,
		runtimehost.NewInferenceProgressPublisherFactory(sessions, baseLogger),
		runtimehost.NewSessionDispatchCompletionObserverFactory(sessions),
	)
}

// LoadFactoryConfigForStartup loads factory.json and replay metadata for compose.
func LoadFactoryConfigForStartup(
	cfg *runtimehost.Config,
	root FactoryServiceRoot,
) (FactoryConfigLoadResult, error) {
	return LoadFactoryConfigForCompose(cfg, root)
}

// ClockForCompose selects the factory clock for the loaded replay artifact.
func ClockForCompose(cfg *runtimehost.Config, load FactoryConfigLoadResult) factory.Clock {
	return ServiceClockForCompose(cfg, load)
}

// HostedWorkersForCompose builds the hosted-workers collaborator from config.
func HostedWorkersForCompose(
	cfg *runtimehost.Config,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	return NewHostedWorkersConfig(cfg, logger, clock)
}

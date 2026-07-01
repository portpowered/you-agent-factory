package service

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/modelhost"
)

// Dependencies carries runtime inputs for model-domain catalog operations.
type Dependencies struct {
	RuntimeConfig func() *factoryconfig.LoadedFactoryConfig
	ModelHost     func() modelhost.Host
}

// Service owns direct model catalog, pull, and invocation behavior.
type Service struct {
	deps Dependencies
}

// New constructs a model-domain service with explicit dependencies.
func New(deps Dependencies) *Service {
	return &Service{deps: deps}
}

// ListModels returns configured model summaries with managed-runtime readiness projection.
func (s *Service) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	return modelhost.ListModelsWithHost(ctx, s.modelHost(), s.runtimeConfig())
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

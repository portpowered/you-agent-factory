package service

import (
	"context"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
)

// InspectRuntime returns invocation readiness for one model through the Models
// service boundary.
func (s *Service) InspectRuntime(ctx context.Context, modelName string) (models.Runtime, error) {
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: modelName}); err != nil {
		return models.Runtime{}, err
	}
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return models.Runtime{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		return localmodels.EnsureManagedRuntimeReadyForInvocation(
			runtimeCfg,
			modelName,
			nil,
			localmodels.DefaultManagedRuntimeSourceResolver(),
		)
	}
	snapshot, err := host.InspectReadiness(ctx, runtimeCfg, modelName)
	if err != nil {
		return models.Runtime{}, err
	}
	runtime := modelhost.ManagedRuntimeFromSnapshot(snapshot)
	if err := runtime.InvocationError(); err != nil {
		return runtime, err
	}
	return runtime, nil
}

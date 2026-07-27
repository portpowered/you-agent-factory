package runtimeopening

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// modelsRuntimeBind carries the process-scoped Models root and the opaque
// runtime scope opened for one Factory Session selection.
type modelsRuntimeBind struct {
	Root  models.Service
	Scope models.RuntimeScopeRef
}

func bindModelsRuntimeScope(
	ctx context.Context,
	modelService models.Service,
	cacheDirectory string,
	runtimeConfigLoader models.RuntimeConfigLoader,
) (modelsRuntimeBind, error) {
	if modelService == nil {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models service is required")
	}
	if runtimeConfigLoader == nil {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models runtime configuration lookup is required")
	}
	scopeConfig := models.RuntimeScopeConfig{
		CacheDirectory: cacheDirectory,
	}
	if runtimeConfig := runtimeConfigLoader(); runtimeConfig != nil {
		scopeConfig.Runtime = *runtimeConfig
	}
	opened, err := modelService.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: scopeConfig,
	})
	if err != nil {
		return modelsRuntimeBind{}, err
	}
	if opened.Scope.IsZero() {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models service returned zero runtime scope")
	}
	return modelsRuntimeBind{
		Root:  modelService,
		Scope: opened.Scope,
	}, nil
}

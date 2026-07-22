package service

import (
	"context"
	"fmt"
	"sort"

	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/catalog"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

// ListModels returns configured model summaries with managed-runtime readiness projection.
func (s *Service) ListModels(ctx context.Context) (modelcatalog.List, error) {
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return modelcatalog.List{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		return localmodels.ListModelsWithRuntime(
			runtimeCfg, nil, localmodels.DefaultManagedRuntimeSourceResolver(),
		)
	}

	catalog := localmodels.BuildCatalogWithRuntime(
		runtimeCfg, nil, localmodels.DefaultManagedRuntimeSourceResolver(),
	)
	results := make([]modelcatalog.Summary, 0, len(catalog))
	for _, entry := range catalog {
		summary := entry.Summary
		snapshot, err := host.InspectReadiness(ctx, runtimeCfg, entry.Summary.Name)
		if err != nil {
			return modelcatalog.List{}, err
		}
		summary.ManagedRuntime = overlayCatalogManagedRuntime(summary.ManagedRuntime, snapshot)
		results = append(results, summary)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return modelcatalog.List{Results: results}, nil
}

// GetModel returns inspect detail for one configured model with managed-runtime readiness projection.
func (s *Service) GetModel(ctx context.Context, modelName string) (modelcatalog.Detail, error) {
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return modelcatalog.Detail{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		return localmodels.GetModelWithRuntime(
			runtimeCfg, modelName, nil, localmodels.DefaultManagedRuntimeSourceResolver(),
		)
	}

	catalog := localmodels.BuildCatalogWithRuntime(
		runtimeCfg, nil, localmodels.DefaultManagedRuntimeSourceResolver(),
	)
	key := localmodels.CanonicalModelName(modelName)
	if key == "" {
		return modelcatalog.Detail{}, fmt.Errorf("%w: empty model name", managedruntime.ErrNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return modelcatalog.Detail{}, fmt.Errorf("%w: %s", managedruntime.ErrNotFound, modelName)
	}
	snapshot, err := host.InspectReadiness(ctx, runtimeCfg, entry.Summary.Name)
	if err != nil {
		return modelcatalog.Detail{}, err
	}
	detail := entry.Detail
	detail.ManagedRuntime = overlayCatalogManagedRuntime(detail.ManagedRuntime, snapshot)
	detail.Diagnostics = mergeCatalogDiagnostics(detail.Diagnostics, detail.ManagedRuntime.Diagnostics)
	return detail, nil
}

func overlayCatalogManagedRuntime(
	base managedruntime.Runtime,
	snapshot modelhost.ReadinessSnapshot,
) managedruntime.Runtime {
	projected := modelhost.ManagedRuntimeFromSnapshot(snapshot)
	if len(base.SupportedOperations) > 0 {
		projected.SupportedOperations = append([]managedruntime.Operation(nil), base.SupportedOperations...)
	}
	if projected.Locality == "" {
		projected.Locality = base.Locality
	}
	return projected
}

func mergeCatalogDiagnostics(base, managed map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range managed {
		merged[key] = value
	}
	return merged
}

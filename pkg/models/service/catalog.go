package service

import (
	"context"
	"fmt"
	"sort"

	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	modelcatalogmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/modelcatalog"
)

// ListModels returns configured model summaries with managed-runtime readiness projection.
func (s *Service) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return factoryapi.ListModelsResponse{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		models, err := localmodels.ListModelsWithOptions(runtimeCfg, catalogDiscoveryOptions())
		return modelcatalogmapping.ListToGenerated(models), err
	}

	catalog := localmodels.BuildCatalogWithOptions(runtimeCfg, catalogDiscoveryOptions())
	results := make([]factoryapi.ModelSummary, 0, len(catalog))
	for _, entry := range catalog {
		summary := modelcatalogmapping.SummaryToGenerated(entry.Summary)
		snapshot, err := host.InspectReadiness(ctx, runtimeCfg, summary.Name)
		if err != nil {
			return factoryapi.ListModelsResponse{}, err
		}
		summary.ManagedRuntime = overlayCatalogManagedRuntime(summary.ManagedRuntime, snapshot)
		results = append(results, summary)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return factoryapi.ListModelsResponse{Results: results}, nil
}

// GetModel returns inspect detail for one configured model with managed-runtime readiness projection.
func (s *Service) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return factoryapi.ModelDetail{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		detail, err := localmodels.GetModelWithOptions(runtimeCfg, modelName, catalogDiscoveryOptions())
		return modelcatalogmapping.DetailToGenerated(detail), err
	}

	catalog := localmodels.BuildCatalogWithOptions(runtimeCfg, catalogDiscoveryOptions())
	key := localmodels.CanonicalModelName(modelName)
	if key == "" {
		return factoryapi.ModelDetail{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return factoryapi.ModelDetail{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
	}
	snapshot, err := host.InspectReadiness(ctx, runtimeCfg, entry.Summary.Name)
	if err != nil {
		return factoryapi.ModelDetail{}, err
	}
	detail := modelcatalogmapping.DetailToGenerated(entry.Detail)
	detail.ManagedRuntime = overlayCatalogManagedRuntime(detail.ManagedRuntime, snapshot)
	detail.Diagnostics = mergeCatalogDiagnostics(detail.Diagnostics, detail.ManagedRuntime)
	return detail, nil
}

func catalogDiscoveryOptions() localmodels.CatalogOptions {
	return localmodels.CatalogOptions{
		SourceResolver: localmodels.DefaultManagedRuntimeSourceResolver(),
	}
}

func overlayCatalogManagedRuntime(
	base factoryapi.ManagedRuntime,
	snapshot modelhost.ReadinessSnapshot,
) factoryapi.ManagedRuntime {
	projected := apisurface.ManagedRuntimeToAPI(modelhost.ManagedRuntimeFromSnapshot(snapshot))
	if len(base.SupportedOperations) > 0 {
		projected.SupportedOperations = append([]factoryapi.ModelOperation(nil), base.SupportedOperations...)
	}
	if projected.Locality == "" {
		projected.Locality = base.Locality
	}
	return projected
}

func mergeCatalogDiagnostics(base factoryapi.StringMap, managed factoryapi.ManagedRuntime) factoryapi.StringMap {
	merged := factoryapi.StringMap{}
	for key, value := range base {
		merged[key] = value
	}
	if managed.Diagnostics == nil {
		return merged
	}
	for key, value := range *managed.Diagnostics {
		merged[key] = value
	}
	return merged
}

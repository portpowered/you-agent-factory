package modelhost

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

// CatalogDiscoveryOptions configures catalog structure projection without cache-backed readiness.
func CatalogDiscoveryOptions() localmodels.CatalogOptions {
	return localmodels.CatalogOptions{
		SourceResolver: localmodels.DefaultManagedRuntimeSourceResolver(),
	}
}

// ListModelsWithHost projects managed-runtime list responses through the process-wide model host.
func ListModelsWithHost(
	ctx context.Context,
	host Host,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
) (factoryapi.ListModelsResponse, error) {
	if runtimeCfg == nil {
		return factoryapi.ListModelsResponse{}, fmt.Errorf("factory service runtime is not available")
	}
	if host == nil {
		return localmodels.ListModelsWithOptions(runtimeCfg, localmodels.CatalogOptions{
			RuntimeCacheInspector: nil,
			SourceResolver:        localmodels.DefaultManagedRuntimeSourceResolver(),
		})
	}
	catalog := localmodels.BuildCatalogWithOptions(runtimeCfg, CatalogDiscoveryOptions())
	results := make([]factoryapi.ModelSummary, 0, len(catalog))
	for _, entry := range catalog {
		summary := entry.Summary
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

// GetModelWithHost projects managed-runtime inspect responses through the process-wide model host.
func GetModelWithHost(
	ctx context.Context,
	host Host,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (factoryapi.ModelDetail, error) {
	if runtimeCfg == nil {
		return factoryapi.ModelDetail{}, fmt.Errorf("factory service runtime is not available")
	}
	if host == nil {
		return localmodels.GetModelWithOptions(runtimeCfg, modelName, localmodels.CatalogOptions{
			SourceResolver: localmodels.DefaultManagedRuntimeSourceResolver(),
		})
	}
	catalog := localmodels.BuildCatalogWithOptions(runtimeCfg, CatalogDiscoveryOptions())
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
	detail := entry.Detail
	detail.ManagedRuntime = overlayCatalogManagedRuntime(detail.ManagedRuntime, snapshot)
	detail.Diagnostics = mergeCatalogDiagnostics(detail.Diagnostics, detail.ManagedRuntime)
	return detail, nil
}

// PullWithHost delegates managed-runtime pull materialization to the process-wide model host.
func PullWithHost(
	ctx context.Context,
	host Host,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (apisurface.ModelPullResult, error) {
	if runtimeCfg == nil {
		return apisurface.ModelPullResult{}, fmt.Errorf("factory service runtime is not available")
	}
	if host == nil {
		return apisurface.ModelPullResult{}, fmt.Errorf("model host is not available")
	}
	snapshot, err := host.Pull(ctx, runtimeCfg, modelName)
	result := ModelPullResultFromSnapshot(snapshot)
	if err != nil {
		var pullErr *apisurface.ManagedRuntimePullError
		if errors.As(err, &pullErr) {
			return pullErr.Result, err
		}
		if mapped := mapPullHostError(result, err); mapped != nil {
			return result, mapped
		}
		return result, err
	}
	return result, nil
}

// EnsureInvocationReady classifies invocation readiness through the model host boundary.
func EnsureInvocationReady(
	ctx context.Context,
	host Host,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (factoryapi.ManagedRuntime, error) {
	if runtimeCfg == nil {
		return factoryapi.ManagedRuntime{}, fmt.Errorf("runtime config is not available")
	}
	if host == nil {
		return localmodels.EnsureManagedRuntimeReadyForInvocation(runtimeCfg, modelName, localmodels.CatalogOptions{
			SourceResolver: localmodels.DefaultManagedRuntimeSourceResolver(),
		})
	}
	snapshot, err := invocationReadinessSnapshot(ctx, host, runtimeCfg, modelName)
	if err != nil {
		return factoryapi.ManagedRuntime{}, err
	}
	managed := ManagedRuntimeFromSnapshot(snapshot)
	if invocationErr := apisurface.InvocationErrorFromManagedRuntime(managed); invocationErr != nil {
		return managed, invocationErr
	}
	return managed, nil
}

func invocationReadinessSnapshot(
	ctx context.Context,
	host Host,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (ReadinessSnapshot, error) {
	return host.InspectReadiness(ctx, runtimeCfg, modelName)
}

// ModelPullResultFromSnapshot maps one host pull snapshot into the service-owned pull result.
func ModelPullResultFromSnapshot(snapshot PullSnapshot) apisurface.ModelPullResult {
	files := make([]apisurface.ModelPullDownloadedFile, 0, len(snapshot.DownloadedFiles))
	for _, file := range snapshot.DownloadedFiles {
		files = append(files, apisurface.ModelPullDownloadedFile{
			Path:   file.Path,
			Bytes:  file.Bytes,
			SHA256: file.SHA256,
		})
	}
	locality := snapshot.Identity.Locality
	if locality == "" {
		locality = factoryapi.WorkerModelLocalityLocal
	}
	return apisurface.ModelPullResult{
		ModelName:          strings.TrimSpace(snapshot.Identity.Name),
		ProviderLocality:   string(locality),
		Outcome:            strings.TrimSpace(snapshot.LegacyOutcome),
		CachePath:          strings.TrimSpace(snapshot.CachePath),
		Revision:           strings.TrimSpace(snapshot.Revision),
		DownloadedFiles:    files,
		ManagedPullOutcome: string(snapshot.PullOutcome),
		ReadinessState:     string(snapshot.ReadinessState),
		LifecycleState:     string(snapshot.LifecycleState),
		SourceKind:         strings.TrimSpace(snapshot.Identity.SourceKind),
		SourceID:           strings.TrimSpace(snapshot.Identity.SourceID),
		ResolverNotes:      strings.TrimSpace(snapshot.Identity.ResolverNotes),
	}
}

func overlayCatalogManagedRuntime(
	base factoryapi.ManagedRuntime,
	snapshot ReadinessSnapshot,
) factoryapi.ManagedRuntime {
	projected := ManagedRuntimeFromSnapshot(snapshot)
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

func mapPullHostError(result apisurface.ModelPullResult, err error) error {
	if errors.Is(err, ErrUnsupportedRuntime) {
		modelName := strings.TrimSpace(result.ModelName)
		if modelName == "" {
			modelName = "model"
		}
		return fmt.Errorf("%w: model %q is not a local model", apisurface.ErrModelPullUnsupported, modelName)
	}
	var readinessErr *ReadinessError
	if errors.As(err, &readinessErr) {
		if errors.Is(readinessErr.Cause, ErrUnsupportedRuntime) {
			modelName := strings.TrimSpace(readinessErr.Snapshot.Identity.Name)
			if modelName == "" {
				modelName = strings.TrimSpace(result.ModelName)
			}
			if modelName == "" {
				modelName = "model"
			}
			return fmt.Errorf("%w: model %q is not a local model", apisurface.ErrModelPullUnsupported, modelName)
		}
	}
	return nil
}

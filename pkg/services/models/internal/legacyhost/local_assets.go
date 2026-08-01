package modelhost

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

type localAssetGateway struct {
	puller localmodels.AssetPuller
}

// NewLocalAssetGateway adapts the Models local-runtime integration for the model host boundary.
func NewLocalAssetGateway(puller localmodels.AssetPuller) AssetGateway {
	if puller == nil {
		return nil
	}
	return localAssetGateway{puller: puller}
}

func (g localAssetGateway) PullModel(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (AssetPullResult, error) {
	result, err := localmodels.PullModelWithOptions(g.puller, ctx, runtimeCfg, modelName, localmodels.PullOptions{
		RuntimeCacheInspector: g.puller,
		SourceResolver:        localmodels.DefaultManagedRuntimeSourceResolver(),
	})
	outcome := managedruntime.PullOutcome(strings.TrimSpace(result.ManagedPullOutcome))
	if outcome == "" && err == nil {
		outcome = managedruntime.PullOutcomeInstalledSuccessfully
	}
	if err != nil {
		if outcome == "" {
			outcome = managedruntime.PullOutcomeUnsupportedRuntime
		}
		pullResult := AssetPullResult{
			PullOutcome: outcome,
			Snapshot:    snapshotFromPullResult(result),
		}
		return pullResult, err
	}
	return assetPullResultFromService(result, outcome), nil
}

func (g localAssetGateway) InspectRuntimeCache(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (CacheInspection, error) {
	inspection, err := g.puller.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return CacheInspection{}, err
	}
	return CacheInspection{
		Supported:          inspection.Supported,
		Installed:          inspection.Installed,
		Revision:           inspection.Revision,
		CachePath:          inspection.CachePath,
		InstalledFileCount: inspection.InstalledFileCount,
		MissingAssets:      append([]string(nil), inspection.MissingAssets...),
		PartialArtifacts:   inspection.PartialArtifacts,
	}, nil
}

func snapshotFromPullResult(result models.PullResult) ReadinessSnapshot {
	readiness := managedruntime.ReadinessState(strings.TrimSpace(result.ReadinessState))
	if readiness == "" {
		readiness = managedruntime.ReadinessStateReady
	}
	lifecycle := managedruntime.LifecycleState(strings.TrimSpace(result.LifecycleState))
	if lifecycle == "" {
		lifecycle = managedruntime.LifecycleStateInstalled
	}
	diagnostics := map[string]string{
		"readinessState": string(readiness),
		"lifecycleState": string(lifecycle),
		"locality":       result.ProviderLocality,
	}
	if result.SourceKind != "" {
		diagnostics["sourceKind"] = result.SourceKind
		diagnostics["sourceId"] = result.SourceID
		diagnostics["resolverNotes"] = result.ResolverNotes
	}
	return ReadinessSnapshot{
		Identity: Identity{
			Name:          strings.TrimSpace(result.ModelName),
			Locality:      managedruntime.Locality(result.ProviderLocality),
			SourceKind:    result.SourceKind,
			SourceID:      result.SourceID,
			ResolverNotes: result.ResolverNotes,
		},
		ReadinessState: readiness,
		LifecycleState: lifecycle,
		FailureClass:   FailureClassForReadinessState(readiness),
		Diagnostics:    diagnostics,
	}
}

func assetPullResultFromService(result models.PullResult, outcome managedruntime.PullOutcome) AssetPullResult {
	files := make([]PullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		files = append(files, PullDownloadedFile{
			Path:   file.Path,
			Bytes:  file.Bytes,
			SHA256: file.SHA256,
		})
	}
	return AssetPullResult{
		PullOutcome:     outcome,
		Snapshot:        snapshotFromPullResult(result),
		LegacyOutcome:   strings.TrimSpace(result.Outcome),
		CachePath:       strings.TrimSpace(result.CachePath),
		Revision:        strings.TrimSpace(result.Revision),
		DownloadedFiles: files,
	}
}

type localSourceResolverAdapter struct {
	inner localmodels.ManagedRuntimeSourceResolver
}

// DefaultManagedRuntimeSourceResolverAdapter exposes the production source resolver through modelhost.
func DefaultManagedRuntimeSourceResolverAdapter() SourceResolver {
	return localSourceResolverAdapter{inner: localmodels.DefaultManagedRuntimeSourceResolver()}
}

func (a localSourceResolverAdapter) Resolve(modelName, backend, loadPolicy, provider string) SourceResolution {
	resource := models.RuntimeResource{
		Backend:    backend,
		LoadPolicy: loadPolicy,
		Provider:   provider,
	}
	resolution := a.inner.Resolve(modelName, &resource)
	return SourceResolution{
		SourceKind:    resolution.SourceKind,
		SourceID:      resolution.SourceID,
		ResolverNotes: resolution.ResolverNotes,
	}
}

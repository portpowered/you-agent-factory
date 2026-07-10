package modelhost

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

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

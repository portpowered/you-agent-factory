package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	"go.uber.org/zap"
)

const (
	modelPullMetricAttempts      = "managed_runtime.pull.attempts"
	modelPullMetricSuccess       = "managed_runtime.pull.success"
	modelPullMetricFailure       = "managed_runtime.pull.failure"
	modelPullMetricSourceFailure = "managed_runtime.pull.source_failure"
)

// Scoped asset lifecycle is contract-only until the Models implementation
// packet owns runtime-scope registration, asset inspection, and removal.
func (s *Service) PreflightModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *Service) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *Service) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *Service) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

// PullModel starts or reports managed-runtime pull materialization for one model.
func (s *Service) PullModel(ctx context.Context, modelName string) (models.PullResult, error) {
	if s == nil {
		return models.PullResult{}, fmt.Errorf("factory service runtime is not available")
	}
	if err := models.ValidatePullModelRequest(models.PullModelRequest{Name: modelName}); err != nil {
		return models.PullResult{}, err
	}
	started := s.now()
	if logger := s.logger(); logger != nil {
		safeModelName := (models.PullDiagnostics{ModelName: modelName}).Normalize().ModelName
		logger.Info(
			"managed runtime pull started",
			zap.String("model_name", safeModelName),
		)
	}
	host := s.modelHost()
	if host == nil {
		puller := s.modelAssetPuller()
		opts := localmodels.PullOptions{
			RuntimeCacheInspector: puller,
			SourceResolver:        localmodels.DefaultManagedRuntimeSourceResolver(),
		}
		result, err := localmodels.PullModelWithOptions(puller, ctx, s.runtimeConfig(), modelName, opts)
		s.recordManagedRuntimePull(modelName, result, err, s.now().Sub(started))
		return result, err
	}
	result, err := s.pullWithModelHost(ctx, host, modelName)
	s.recordManagedRuntimePull(modelName, result, err, s.now().Sub(started))
	return result, err
}

func (o *Root) pullResolvedModelAfterCatalogMiss(
	ctx context.Context,
	request models.PullModelRequest,
	catalogErr error,
) (models.PullResult, error) {
	resolution, err := o.ResolveModelReference(ctx, models.ResolveModelReferenceRequest{
		Scope: request.Scope,
		Reference: models.ModelReference{
			NameOrURI: request.Name,
		},
	})
	if err != nil {
		// The scoped catalog miss remains the established pull error when the
		// canonical resolver also reports a genuine unknown name. Other
		// resolver failures are meaningful configuration or reference errors
		// and must not be hidden behind the earlier catalog miss.
		if errors.Is(err, models.ErrModelReferenceUnknown) {
			return models.PullResult{}, catalogErr
		}
		return models.PullResult{}, err
	}
	if o == nil || o.assets == nil || o.runtimeScopes == nil {
		return models.PullResult{}, catalogErr
	}
	binding, err := o.runtimeScopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.PullResult{}, runtimeScopeError(err)
	}
	if binding.RuntimeConfig == nil {
		return models.PullResult{}, models.ErrUnavailable
	}
	runtimeConfig := binding.RuntimeConfig()
	if runtimeConfig == nil {
		return models.PullResult{}, models.ErrUnavailable
	}
	resolved := resolution.Resolved.Clone()
	puller, err := localmodels.NewScopedAssetPullerWithPreparation(
		o.assets,
		request.Scope,
		func(ctx context.Context, modelName string, scope models.RuntimeScopeRef) (models.PrepareModelAssetsRequest, error) {
			var backendArtifact modelseffects.BackendArtifactSelection
			if o.resolveBackendArtifact != nil && isJoinedPinnedBackend(resolved.Definition.Backend) {
				var resolveErr error
				backendArtifact, resolveErr = o.resolveJoinedBackendArtifact(ctx, resolved.Definition)
				if resolveErr != nil {
					return models.PrepareModelAssetsRequest{}, resolveErr
				}
			}
			return joinedAssetPreparationRequestWithBackend(
				models.InvokeModelRequest{
					Scope: scope,
					Model: models.ModelReference{NameOrURI: modelName},
				},
				modelName,
				resolved,
				backendArtifact,
			), nil
		},
	)
	if err != nil {
		return models.PullResult{}, err
	}
	return localmodels.PullModelWithOptions(
		puller,
		ctx,
		runtimeConfig,
		request.Name,
		localmodels.PullOptions{
			RuntimeCacheInspector: puller,
			SourceResolver:        localmodels.DefaultManagedRuntimeSourceResolver(),
			ResolvedReference:     &resolved,
		},
	)
}

func (s *Service) pullWithModelHost(
	ctx context.Context,
	host modelhost.Host,
	modelName string,
) (models.PullResult, error) {
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return models.PullResult{}, fmt.Errorf("factory service runtime is not available")
	}
	snapshot, err := host.Pull(ctx, runtimeCfg, modelName)
	result := modelPullResultFromSnapshot(snapshot)
	if err == nil {
		return result, nil
	}
	var pullErr *models.PullError
	if errors.As(err, &pullErr) && pullErr != nil {
		pullErr.Result.PullDiagnostics = pullsupport.MergePullDiagnostics(
			pullErr.Result.PullDiagnostics,
			pullsupport.PullDiagnosticsFromError(pullErr.Cause),
		).WithDefaults(
			modelName, pullErr.Result.SourceID, pullErr.Result.Revision, "", "pull model",
		)
		return pullErr.Result, err
	}
	if errors.Is(err, managedruntime.ErrNotFound) {
		return result, err
	}
	if isUnsupportedModelHostPull(err) {
		name := strings.TrimSpace(result.ModelName)
		if name == "" {
			name = strings.TrimSpace(modelName)
		}
		return result, fmt.Errorf("%w: model %q is not a local model", models.ErrPullUnsupported, name)
	}

	pullOutcome, readiness := localmodels.ClassifyPullFailure(err)
	if strings.TrimSpace(result.ModelName) == "" {
		result.ModelName = strings.TrimSpace(modelName)
	}
	result.Outcome = "FAILED"
	if strings.TrimSpace(result.ManagedPullOutcome) == "" {
		result.ManagedPullOutcome = pullOutcome
	}
	if strings.TrimSpace(result.ReadinessState) == "" {
		result.ReadinessState = readiness
	}
	if strings.TrimSpace(result.LifecycleState) == "" {
		result.LifecycleState = string(managedruntime.LifecycleStateNotInstalled)
	}
	result.PullDiagnostics = pullsupport.MergePullDiagnostics(
		result.PullDiagnostics,
		pullsupport.PullDiagnosticsFromError(err),
	).WithDefaults(modelName, result.SourceID, result.Revision, "", "pull model")
	return result, &models.PullError{Result: result, Cause: err}
}

func isUnsupportedModelHostPull(err error) bool {
	if errors.Is(err, modelhost.ErrUnsupportedRuntime) || errors.Is(err, models.ErrPullUnsupported) {
		return true
	}
	var readinessErr *modelhost.ReadinessError
	return errors.As(err, &readinessErr) && errors.Is(readinessErr.Cause, modelhost.ErrUnsupportedRuntime)
}

func modelPullResultFromSnapshot(snapshot modelhost.PullSnapshot) models.PullResult {
	files := make([]models.DownloadedFile, 0, len(snapshot.DownloadedFiles))
	for _, file := range snapshot.DownloadedFiles {
		files = append(files, models.DownloadedFile{
			Path:   file.Path,
			Bytes:  file.Bytes,
			SHA256: file.SHA256,
		})
	}
	locality := snapshot.Identity.Locality
	if locality == "" {
		locality = managedruntime.LocalityLocal
	}
	return models.PullResult{
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

func (s *Service) modelAssetPuller() localmodels.AssetPuller {
	if s == nil {
		return nil
	}
	return s.assetPuller
}

func (s *Service) recordManagedRuntimePull(modelName string, result models.PullResult, err error, elapsed time.Duration) {
	labels := map[string]string{"model_name": strings.TrimSpace(modelName)}
	s.recordModelPullMetric(modelPullMetricAttempts, labels)
	if err != nil {
		pullOutcome, readiness := localmodels.ClassifyPullFailure(err)
		if strings.TrimSpace(result.ManagedPullOutcome) != "" {
			pullOutcome = strings.TrimSpace(result.ManagedPullOutcome)
		}
		if strings.TrimSpace(result.ReadinessState) != "" {
			readiness = strings.TrimSpace(result.ReadinessState)
		}
		lifecycle := strings.TrimSpace(result.LifecycleState)
		if lifecycle == "" {
			lifecycle = string(managedruntime.LifecycleStateNotInstalled)
		}
		failureLabels := mergeMetricLabels(labels, map[string]string{
			"pull_outcome":    pullOutcome,
			"readiness_state": readiness,
			"lifecycle_state": lifecycle,
		})
		s.recordModelPullMetric(modelPullMetricFailure, failureLabels)
		if !errors.Is(err, context.Canceled) &&
			(errors.Is(err, models.ErrSourceFetchFailed) || pullOutcome == "SOURCE_FETCH_FAILED") {
			s.recordModelPullMetric(modelPullMetricSourceFailure, failureLabels)
		}
		if logger := s.logger(); logger != nil {
			diagnostics := pullsupport.MergePullDiagnostics(
				result.PullDiagnostics,
				pullsupport.PullDiagnosticsFromError(err),
			).WithDefaults(
				modelName, result.SourceID, result.Revision, "", pullDiagnosticOperation(result, err),
			)
			safeModelName := diagnostics.ModelName
			resolvedSource := diagnostics.ResolvedRepository
			if resolvedSource == "" {
				resolvedSource = models.PullDiagnostics{ResolvedRepository: result.SourceKind}.Normalize().ResolvedRepository
			}
			safeSourceID := models.PullDiagnostics{ResolvedRepository: result.SourceID}.Normalize().ResolvedRepository
			fields := []zap.Field{
				zap.String("model_name", safeModelName),
				zap.String("pull_outcome", pullOutcome),
				zap.String("terminal_classification", pullOutcome),
				zap.String("readiness_state", readiness),
				zap.String("lifecycle_state", lifecycle),
				zap.String("failure_reason", managedRuntimePullFailureReason(err)),
				zap.String("operation", diagnostics.Operation),
				zap.String("resolved_source", resolvedSource),
				zap.String("resolved_repository", diagnostics.ResolvedRepository),
				zap.String("revision", diagnostics.Revision),
				zap.String("file", diagnostics.File),
				zap.String("request_url", diagnostics.RequestURL),
				zap.String("source_kind", strings.TrimSpace(result.SourceKind)),
				zap.String("source_id", safeSourceID),
				zap.Duration("duration", elapsed),
			}
			if diagnostics.UpstreamStatusCode != 0 {
				fields = append(fields, zap.Int("upstream_status_code", diagnostics.UpstreamStatusCode))
			}
			logger.Warn(
				"managed runtime pull failed",
				fields...,
			)
		}
		return
	}
	successLabels := mergeMetricLabels(labels, map[string]string{
		"pull_outcome":    strings.TrimSpace(result.ManagedPullOutcome),
		"readiness_state": strings.TrimSpace(result.ReadinessState),
		"lifecycle_state": strings.TrimSpace(result.LifecycleState),
		"source_kind":     strings.TrimSpace(result.SourceKind),
	})
	s.recordModelPullMetric(modelPullMetricSuccess, successLabels)
	if logger := s.logger(); logger != nil {
		logger.Info(
			"managed runtime pull completed",
			zap.String("model_name", modelName),
			zap.String("pull_outcome", result.ManagedPullOutcome),
			zap.String("readiness_state", result.ReadinessState),
			zap.String("lifecycle_state", result.LifecycleState),
			zap.String("source_kind", result.SourceKind),
			zap.String("source_id", result.SourceID),
			zap.Duration("duration", elapsed),
		)
	}
}

func pullDiagnosticOperation(result models.PullResult, err error) string {
	if diagnostics := pullsupport.PullDiagnosticsFromError(err); diagnostics.Operation != "" {
		return diagnostics.Operation
	}
	switch result.FailureStage {
	case models.PullStageSourceResolution:
		return "resolve model source"
	case models.PullStageSourceFetch:
		return "fetch model assets"
	case models.PullStageIntegrityVerification:
		return "verify model assets"
	case models.PullStageAssembly:
		return "assemble model assets"
	case models.PullStageCacheInstallation:
		return "install model cache"
	case models.PullStageReadinessEvaluation:
		return "evaluate model readiness"
	default:
		return "pull model"
	}
}

func managedRuntimePullFailureReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "caller_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed_out"
	case errors.Is(err, models.ErrAssetIntegrityFailed):
		return "integrity_failed"
	case errors.Is(err, models.ErrAssetPreparationInterrupted):
		return "asset_preparation_interrupted"
	case errors.Is(err, models.ErrAssetSourceMissing):
		return "source_missing"
	case errors.Is(err, models.ErrAssetSourceUnsupported):
		return "source_unsupported"
	case errors.Is(err, models.ErrSourceFetchFailed):
		return "source_fetch_failed"
	default:
		return "pull_failed"
	}
}

func (s *Service) recordModelPullMetric(name string, labels map[string]string) {
	if s == nil || s.pullMetrics == nil {
		return
	}
	s.pullMetrics.RecordModelPullMetric(modelseffects.PullMetric{
		Name:   name,
		Labels: cloneMetricLabels(labels),
	})
}

func (s *Service) logger() *zap.Logger {
	if s == nil {
		return nil
	}
	return s.loggerValue
}

func mergeMetricLabels(parts ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, part := range parts {
		for key, value := range part {
			merged[key] = value
		}
	}
	return merged
}

func cloneMetricLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func removeModelCacheStartLog(o *Root, request models.RemoveModelAssetsRequest) {
	if o == nil || o.process.Logger == nil {
		return
	}
	o.process.Logger.Info(
		"models cache removal started",
		zap.String("model_name", strings.TrimSpace(request.Name)),
		zap.String("scope", request.Scope.String()),
	)
}

func removeModelCacheTerminalLog(
	o *Root,
	request models.RemoveModelAssetsRequest,
	result models.RemoveModelAssetsResult,
	err error,
	elapsed time.Duration,
) {
	if o == nil || o.process.Logger == nil {
		return
	}
	outcome := string(result.Outcome)
	if err != nil {
		outcome = "FAILED"
	}
	fields := []zap.Field{
		zap.String("model_name", strings.TrimSpace(request.Name)),
		zap.String("scope", request.Scope.String()),
		zap.String("revision", result.Revision),
		zap.String("cache_path", result.CachePath),
		zap.Int64("bytes_removed", result.BytesRemoved),
		zap.String("outcome", outcome),
		zap.Duration("duration", elapsed),
	}
	if err != nil {
		fields = append(fields,
			zap.String("failure_class", removeModelCacheFailureClass(err)),
			zap.Error(err),
		)
		o.process.Logger.Warn("models cache removal completed", fields...)
		return
	}
	o.process.Logger.Info("models cache removal completed", fields...)
}

func removeModelCacheFailureClass(err error) string {
	switch {
	case errors.Is(err, models.ErrModelCacheInUse):
		return "CACHE_IN_USE"
	case errors.Is(err, models.ErrModelCacheNotFound):
		return "CACHE_NOT_FOUND"
	case errors.Is(err, models.ErrModelCacheUnsafe):
		return "CACHE_UNSAFE"
	case errors.Is(err, models.ErrModelCacheRemovalFailed):
		return "REMOVAL_FAILED"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "CANCELLED"
	default:
		return "INTERNAL"
	}
}

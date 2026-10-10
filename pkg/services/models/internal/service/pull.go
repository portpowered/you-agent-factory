package service

import (
	"context"
	"errors"
	"strings"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
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

// PullModelForScope selects detached configuration and a scoped asset adapter.
func (s *scopedLocalExecution) PullModelForScope(ctx context.Context, request models.PullModelRequest) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(request); err != nil {
		return models.PullResult{}, err
	}
	if request.Scope.IsZero() {
		return models.PullResult{}, models.ErrRuntimeScopeInvalid
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.PullResult{}, runtimeScopeError(err)
	}
	var config *models.RuntimeConfig
	if binding.RuntimeConfig != nil {
		config = binding.RuntimeConfig()
	}
	// Even a successful configuration lookup may finish after close.
	if _, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String())); err != nil {
		return models.PullResult{}, runtimeScopeError(err)
	}
	if err := s.admitInvocation(ctx, request.Scope); err != nil {
		return models.PullResult{}, err
	}
	if config == nil {
		return models.PullResult{}, models.ErrUnavailable
	}
	assets, err := localmodels.NewScopedAssetPuller(s.assets, request.Scope)
	if err != nil {
		return models.PullResult{}, err
	}
	result, err := localmodels.PullModelWithOptions(assets, ctx, config, request.Name, localmodels.PullOptions{
		RuntimeCacheInspector: assets, SourceResolver: localmodels.DefaultManagedRuntimeSourceResolver(),
	})
	if closedErr := s.admitInvocation(ctx, request.Scope); closedErr != nil {
		return models.PullResult{}, closedErr
	}
	return result, err
}

func (o *Root) checkPullScope(ctx context.Context, scope models.RuntimeScopeRef) error {
	if _, err := o.runtimeScopes.Resolve(runtimescopes.Reference(scope.String())); err != nil {
		return runtimeScopeError(err)
	}
	return ctx.Err()
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
	binding, err := o.runtimeScopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.PullResult{}, runtimeScopeError(err)
	}
	if binding.RuntimeConfig == nil {
		return models.PullResult{}, models.ErrUnavailable
	}
	runtimeConfig := binding.RuntimeConfig()
	if err := o.checkPullScope(ctx, request.Scope); err != nil {
		return models.PullResult{}, err
	}
	if runtimeConfig == nil {
		return models.PullResult{}, models.ErrUnavailable
	}
	resolved := resolution.Resolved.Clone()
	puller, err := localmodels.NewScopedAssetPullerWithPreparation(
		o.assets,
		request.Scope,
		func(ctx context.Context, modelName string, scope models.RuntimeScopeRef) (models.PrepareModelAssetsRequest, error) {
			configuration := joinedHostConfiguration(
				models.InvokeModelRequest{
					Scope: scope,
					Model: models.ModelReference{NameOrURI: modelName},
				},
				resolved,
				o.backendArtifactPlatform,
			)
			if isJoinedManagedBackend(resolved.Definition.Backend) {
				var resolveErr error
				configuration.BackendArtifact, resolveErr = o.resolveJoinedBackendArtifact(ctx, configuration, false)
				if resolveErr != nil {
					return models.PrepareModelAssetsRequest{}, resolveErr
				}
			}
			if _, err := o.runtimeScopes.Resolve(runtimescopes.Reference(scope.String())); err != nil {
				return models.PrepareModelAssetsRequest{}, runtimeScopeError(err)
			}
			return joinedAssetPreparationRequestWithConfiguration(
				models.InvokeModelRequest{
					Scope: scope,
					Model: models.ModelReference{NameOrURI: modelName},
				}, configuration, resolved,
			)
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

func recordManagedRuntimePull(logger *zap.Logger, metrics modelseffects.PullMetricsRecorder, modelName string, result models.PullResult, err error, elapsed time.Duration) {
	labels := map[string]string{"model_name": strings.TrimSpace(modelName)}
	recordModelPullMetric(metrics, modelPullMetricAttempts, labels)
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
		recordModelPullMetric(metrics, modelPullMetricFailure, failureLabels)
		if !errors.Is(err, context.Canceled) &&
			(errors.Is(err, models.ErrSourceFetchFailed) || pullOutcome == "SOURCE_FETCH_FAILED") {
			recordModelPullMetric(metrics, modelPullMetricSourceFailure, failureLabels)
		}
		if logger != nil {
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
	recordModelPullMetric(metrics, modelPullMetricSuccess, successLabels)
	if logger != nil {
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

func recordModelPullMetric(metrics modelseffects.PullMetricsRecorder, name string, labels map[string]string) {
	if metrics == nil {
		return
	}
	metrics.RecordModelPullMetric(modelseffects.PullMetric{
		Name:   name,
		Labels: cloneMetricLabels(labels),
	})
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
	o.logger.Info(
		"models cache removal started",
		zap.String("model_name", strings.TrimSpace(request.Name)),
		zap.String("scope", request.Scope.String()),
		zap.Bool("reclaim_unused_cache", request.ReclaimUnusedCache),
	)
}

func removeModelCacheTerminalLog(
	o *Root,
	request models.RemoveModelAssetsRequest,
	result models.RemoveModelAssetsResult,
	err error,
	elapsed time.Duration,
) {
	outcome := string(result.Outcome)
	if err != nil {
		outcome = "FAILED"
	}
	fields := []zap.Field{
		zap.String("model_name", strings.TrimSpace(request.Name)),
		zap.String("scope", request.Scope.String()),
		zap.String("outcome", outcome),
		zap.Duration("duration", elapsed),
	}
	if err != nil {
		fields = append(fields,
			zap.String("failure_class", removeModelCacheFailureClass(err)),
			zap.Error(err),
		)
		o.logger.Warn("models cache removal completed", fields...)
		return
	}
	fields = append(fields,
		zap.String("revision", result.Revision),
		zap.String("cache_path", result.CachePath),
		zap.Int64("bytes_removed", result.BytesRemoved),
		zap.Int64("reclaimed_cache_bytes", result.ReclaimedCacheBytes),
		zap.Int64("retained_shared_cache_bytes", result.RetainedSharedCacheBytes),
	)
	o.logger.Info("models cache removal completed", fields...)
}

func removeModelCacheFailureClass(err error) string {
	switch {
	case errors.Is(err, models.ErrModelCacheInUse):
		return "CACHE_IN_USE"
	case errors.Is(err, models.ErrModelCacheNotFound):
		return "CACHE_NOT_FOUND"
	case errors.Is(err, models.ErrModelCacheUnsafe):
		return "CACHE_UNSAFE"
	case errors.Is(err, models.ErrModelCacheReferenceUncertain):
		return "CACHE_REFERENCES_UNCERTAIN"
	case errors.Is(err, models.ErrModelCacheRemovalFailed):
		return "REMOVAL_FAILED"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "CANCELLED"
	default:
		return "INTERNAL"
	}
}

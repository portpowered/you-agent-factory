package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	modelassets "github.com/portpowered/infinite-you/pkg/models/assets"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	"go.uber.org/zap"
)

// PullMetric records one managed-runtime pull counter with low-cardinality labels.
type PullMetric struct {
	Name   string
	Labels map[string]string
}

// PullMetricsRecorder receives managed-runtime pull counter emissions.
type PullMetricsRecorder interface {
	RecordModelPullMetric(PullMetric)
}

const (
	modelPullMetricAttempts      = "managed_runtime.pull.attempts"
	modelPullMetricSuccess       = "managed_runtime.pull.success"
	modelPullMetricFailure       = "managed_runtime.pull.failure"
	modelPullMetricSourceFailure = "managed_runtime.pull.source_failure"
)

// PullModel starts or reports managed-runtime pull materialization for one model.
func (s *Service) PullModel(ctx context.Context, modelName string) (modelassets.PullResult, error) {
	started := s.now()
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

func (s *Service) pullWithModelHost(
	ctx context.Context,
	host modelhost.Host,
	modelName string,
) (modelassets.PullResult, error) {
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return modelassets.PullResult{}, fmt.Errorf("factory service runtime is not available")
	}
	snapshot, err := host.Pull(ctx, runtimeCfg, modelName)
	result := modelPullResultFromSnapshot(snapshot)
	if err == nil {
		return result, nil
	}
	if pullErr, ok := modelassets.AsPullError(err); ok {
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
		return result, fmt.Errorf("%w: model %q is not a local model", modelassets.ErrPullUnsupported, name)
	}

	pullOutcome, readiness := localmodels.ClassifyPullFailure(err)
	if strings.TrimSpace(result.ModelName) == "" {
		result.ModelName = strings.TrimSpace(modelName)
	}
	if strings.TrimSpace(result.ManagedPullOutcome) == "" {
		result.ManagedPullOutcome = pullOutcome
	}
	if strings.TrimSpace(result.ReadinessState) == "" {
		result.ReadinessState = readiness
	}
	if strings.TrimSpace(result.LifecycleState) == "" {
		result.LifecycleState = string(managedruntime.LifecycleStateNotInstalled)
	}
	return result, &modelassets.PullError{Result: result, Cause: err}
}

func isUnsupportedModelHostPull(err error) bool {
	if errors.Is(err, modelhost.ErrUnsupportedRuntime) || errors.Is(err, modelassets.ErrPullUnsupported) {
		return true
	}
	var readinessErr *modelhost.ReadinessError
	return errors.As(err, &readinessErr) && errors.Is(readinessErr.Cause, modelhost.ErrUnsupportedRuntime)
}

func modelPullResultFromSnapshot(snapshot modelhost.PullSnapshot) modelassets.PullResult {
	files := make([]modelassets.DownloadedFile, 0, len(snapshot.DownloadedFiles))
	for _, file := range snapshot.DownloadedFiles {
		files = append(files, modelassets.DownloadedFile{
			Path:   file.Path,
			Bytes:  file.Bytes,
			SHA256: file.SHA256,
		})
	}
	locality := snapshot.Identity.Locality
	if locality == "" {
		locality = managedruntime.LocalityLocal
	}
	return modelassets.PullResult{
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
	if s == nil || s.deps.ModelAssetPuller == nil {
		return localmodels.NewAssetPuller("")
	}
	return s.deps.ModelAssetPuller
}

func (s *Service) recordManagedRuntimePull(modelName string, result modelassets.PullResult, err error, elapsed time.Duration) {
	labels := map[string]string{"model_name": strings.TrimSpace(modelName)}
	s.recordModelPullMetric(modelPullMetricAttempts, labels)
	if err != nil {
		pullOutcome, readiness := localmodels.ClassifyPullFailure(err)
		failureLabels := mergeMetricLabels(labels, map[string]string{
			"pull_outcome":    pullOutcome,
			"readiness_state": readiness,
		})
		s.recordModelPullMetric(modelPullMetricFailure, failureLabels)
		if errors.Is(err, modelassets.ErrSourceFetchFailed) || pullOutcome == "SOURCE_FETCH_FAILED" {
			s.recordModelPullMetric(modelPullMetricSourceFailure, failureLabels)
		}
		if logger := s.logger(); logger != nil {
			logger.Warn(
				"managed runtime pull failed",
				zap.String("model_name", modelName),
				zap.String("pull_outcome", pullOutcome),
				zap.String("readiness_state", readiness),
				zap.Duration("duration", elapsed),
				zap.Error(err),
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

func (s *Service) recordModelPullMetric(name string, labels map[string]string) {
	if s == nil || s.deps.ModelPullMetrics == nil {
		return
	}
	s.deps.ModelPullMetrics.RecordModelPullMetric(PullMetric{
		Name:   name,
		Labels: cloneMetricLabels(labels),
	})
}

func (s *Service) logger() *zap.Logger {
	if s == nil {
		return nil
	}
	return s.deps.Logger
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

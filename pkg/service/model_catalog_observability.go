package service

import (
	"errors"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"go.uber.org/zap"
)

const (
	modelPullMetricAttempts      = "managed_runtime.pull.attempts"
	modelPullMetricSuccess       = "managed_runtime.pull.success"
	modelPullMetricFailure       = "managed_runtime.pull.failure"
	modelPullMetricSourceFailure = "managed_runtime.pull.source_failure"
)

func (s *runtimeModelService) recordManagedRuntimePull(modelName string, result apisurface.ModelPullResult, err error, elapsed time.Duration) {
	labels := map[string]string{"model_name": strings.TrimSpace(modelName)}
	s.recordModelPullMetric(modelPullMetricAttempts, labels)
	if err != nil {
		pullOutcome, readiness := localmodels.ClassifyPullFailure(err)
		failureLabels := mergeMetricLabels(labels, map[string]string{
			"pull_outcome":    pullOutcome,
			"readiness_state": readiness,
		})
		s.recordModelPullMetric(modelPullMetricFailure, failureLabels)
		if errors.Is(err, apisurface.ErrManagedRuntimeSourceFetchFailed) || pullOutcome == "SOURCE_FETCH_FAILED" {
			s.recordModelPullMetric(modelPullMetricSourceFailure, failureLabels)
		}
		if s.deps.logger != nil {
			s.deps.logger.Warn(
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
	if s.deps.logger != nil {
		s.deps.logger.Info(
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

func (s *runtimeModelService) recordManagedRuntimeInvocationReadiness(
	modelName string,
	managed factoryapi.ManagedRuntime,
	err error,
) {
	if s == nil || s.deps.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("model_name", modelName),
		zap.String("managed_runtime_identity", managed.Identity),
		zap.String("readiness_state", string(managed.ReadinessState)),
		zap.String("lifecycle_state", string(managed.LifecycleState)),
	}
	if err != nil {
		s.deps.logger.Warn("managed runtime invocation blocked", append(fields, zap.Error(err))...)
		return
	}
	s.deps.logger.Info("managed runtime invocation readiness satisfied", fields...)
}

func (s *runtimeModelService) recordManagedRuntimeInvocationFailure(
	modelName string,
	managed factoryapi.ManagedRuntime,
	err error,
) {
	if s == nil || s.deps.logger == nil || err == nil {
		return
	}
	s.deps.logger.Warn(
		"managed runtime invocation asset check failed",
		zap.String("model_name", modelName),
		zap.String("managed_runtime_identity", managed.Identity),
		zap.String("readiness_state", string(managed.ReadinessState)),
		zap.String("lifecycle_state", string(managed.LifecycleState)),
		zap.Error(err),
	)
}

func (s *runtimeModelService) recordModelPullMetric(name string, labels map[string]string) {
	if s == nil || s.deps.modelPullMetrics == nil {
		return
	}
	recorder := s.deps.modelPullMetrics()
	if recorder == nil {
		return
	}
	recorder.RecordModelPullMetric(InvocationMetric{
		Name:   name,
		Labels: cloneMetricLabels(labels),
	})
}

func (fs *FactoryService) modelPullMetricsRecorder() ModelPullMetricsRecorder {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.ModelPullMetricsRecorder
}

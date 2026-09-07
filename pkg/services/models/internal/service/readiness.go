package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"go.uber.org/zap"
)

const (
	joinedLifecycleStageArtifactProvision = "ARTIFACT_PROVISION"
	joinedLifecycleStageBackendStart      = "BACKEND_START"
	joinedLifecycleStageHealth            = "HEALTH"
	joinedLifecycleStageInvoke            = "INVOKE"
	joinedLifecycleStageOutput            = "OUTPUT"
	joinedLifecycleStageRelease           = "RELEASE"
	joinedLifecycleStageTerminal          = "TERMINAL"
	joinedLifecycleOutcomeCompleted       = "COMPLETED"
	joinedLifecycleOutcomeFailed          = "FAILED"
)

// InspectRuntime returns invocation readiness for one model through the Models
// service boundary.
func (s *Service) InspectRuntime(ctx context.Context, modelName string) (models.Runtime, error) {
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: modelName}); err != nil {
		return models.Runtime{}, err
	}
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return models.Runtime{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		return localmodels.EnsureManagedRuntimeReadyForInvocation(
			runtimeCfg,
			modelName,
			nil,
			localmodels.DefaultManagedRuntimeSourceResolver(),
		)
	}
	snapshot, err := host.InspectReadiness(ctx, runtimeCfg, modelName)
	if err != nil {
		return models.Runtime{}, err
	}
	runtime := modelhost.ManagedRuntimeFromSnapshot(snapshot)
	if err := runtime.InvocationError(); err != nil {
		return runtime, err
	}
	return runtime, nil
}

func joinedInvocationContextError(ctx context.Context, err error) error {
	if err == nil || ctx == nil || ctx.Err() == nil || errors.Is(err, models.ErrInferenceCancelled) {
		return err
	}
	return errors.Join(models.ErrInferenceCancelled, err)
}

func joinedInvocationStart(o *Root) time.Time {
	if o != nil && o.process.Clock != nil {
		return o.process.Clock()
	}
	return time.Time{}
}

func joinedInvocationElapsed(o *Root, started time.Time) time.Duration {
	if o == nil || o.process.Clock == nil || started.IsZero() {
		return 0
	}
	ended := o.process.Clock()
	if ended.Before(started) {
		return 0
	}
	return ended.Sub(started)
}

func nextJoinedInvocationCorrelation(o *Root) string {
	if o == nil {
		return "models-invocation-0"
	}
	sequence := atomic.AddUint64(&o.correlationSequence, 1)
	return fmt.Sprintf("models-invocation-%d", sequence)
}

func joinedInvocationRevision(source string) string {
	_, revision, found := strings.Cut(strings.TrimSpace(source), "@")
	if !found {
		return ""
	}
	return strings.TrimSpace(revision)
}

func joinedInvocationLifecycleRecord(
	o *Root,
	modelName string,
	backend string,
	revision string,
	correlation string,
	operation string,
	invocation models.ModelInvocationRef,
	stage string,
	outcome string,
	elapsed time.Duration,
	err error,
) {
	if o == nil || o.process.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("stage", boundedLifecycleIdentity(stage)),
		zap.String("outcome", boundedLifecycleIdentity(outcome)),
		zap.Duration("duration", elapsed),
		zap.Int64("duration_millis", elapsed.Milliseconds()),
	}
	appendLifecycleIdentityField(&fields, "model_name", modelName)
	appendLifecycleIdentityField(&fields, "backend", backend)
	appendLifecycleIdentityField(&fields, "revision", revision)
	appendLifecycleIdentityField(&fields, "correlation_id", correlation)
	appendLifecycleIdentityField(&fields, "operation", operation)
	if invocationValue := boundedLifecycleIdentity(invocation.String()); invocationValue != "" {
		fields = append(fields, zap.String("invocation", invocationValue))
	}
	if err != nil {
		diagnostic := modelseffects.ProjectRuntimeFailure(
			modelseffects.WrapRuntimeFailure(modelseffects.RuntimeStageInvoke, err), elapsed,
		)
		fields = append(fields,
			zap.String("failure_class", string(diagnostic.Class)),
			zap.String("cause_sha256", diagnostic.CauseSHA256),
		)
		if diagnostic.Subcause != "" {
			fields = append(fields, zap.String("failure_subcause", string(diagnostic.Subcause)))
		}
		o.process.Logger.Warn("models invocation stage", fields...)
		return
	}
	o.process.Logger.Info("models invocation stage", fields...)
}

func appendLifecycleIdentityField(fields *[]zap.Field, key string, value string) {
	if fields == nil {
		return
	}
	if value = boundedLifecycleIdentity(value); value != "" {
		*fields = append(*fields, zap.String(key, value))
	}
}

func boundedLifecycleIdentity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' || char == '.' {
			continue
		}
		return ""
	}
	return value
}

func joinedInvocationRecord(
	o *Root,
	modelName string,
	operation string,
	invocation models.ModelInvocationRef,
	backend string,
	revision string,
	correlation string,
	stage modelseffects.RuntimeStage,
	err error,
	elapsed time.Duration,
) {
	if o == nil || o.process.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("stage", joinedLifecycleStageTerminal),
		zap.String("evidence_kind", joinedLifecycleStageTerminal),
		zap.String("runtime_stage", string(stage)),
		zap.Duration("duration", elapsed),
		zap.Int64("duration_millis", elapsed.Milliseconds()),
	}
	appendLifecycleIdentityField(&fields, "model_name", modelName)
	appendLifecycleIdentityField(&fields, "operation", operation)
	appendLifecycleIdentityField(&fields, "invocation", invocation.String())
	appendLifecycleIdentityField(&fields, "backend", backend)
	appendLifecycleIdentityField(&fields, "revision", revision)
	appendLifecycleIdentityField(&fields, "correlation_id", correlation)
	if err != nil {
		diagnostic := modelseffects.ProjectRuntimeFailure(
			modelseffects.WrapRuntimeFailure(stage, err), elapsed,
		)
		fields = append(fields,
			zap.String("outcome", "FAILED"),
			zap.String("runtime_stage", string(diagnostic.Stage)),
			zap.String("failure_class", string(diagnostic.Class)),
			zap.String("cause_sha256", diagnostic.CauseSHA256),
		)
		if diagnostic.Subcause != "" {
			fields = append(fields, zap.String("failure_subcause", string(diagnostic.Subcause)))
		}
		o.process.Logger.Warn("models invocation completed", fields...)
		return
	}
	fields = append(fields, zap.String("outcome", "COMPLETED"))
	o.process.Logger.Info("models invocation completed", fields...)
}

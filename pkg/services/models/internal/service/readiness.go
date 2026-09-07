package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"go.uber.org/zap"
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

func joinedInvocationRecord(
	o *Root,
	modelName string,
	operation string,
	invocation models.ModelInvocationRef,
	stage modelseffects.RuntimeStage,
	err error,
	elapsed time.Duration,
) {
	if o == nil || o.process.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("model_name", modelName),
		zap.String("operation", operation),
		zap.String("invocation", invocation.String()),
		zap.String("runtime_stage", string(stage)),
		zap.Duration("duration", elapsed),
		zap.Int64("duration_millis", elapsed.Milliseconds()),
	}
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
		o.process.Logger.Warn("models invocation completed", fields...)
		return
	}
	fields = append(fields, zap.String("outcome", "COMPLETED"))
	o.process.Logger.Info("models invocation completed", fields...)
}

package internal

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// InvokeModel remains only as the compatibility implementation of the
// RuntimeService interface. Direct model operations are admitted by the
// Factory Session runtime and enter workers.Service.Execute with a complete,
// request-scoped target; this legacy runtime role has no implicit session
// configuration from which to construct a Workstation executor.
func (s *Service) InvokeModel(
	ctx context.Context,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	_ = s
	_ = ctx
	failureContext := workers.InferenceFailureContext{
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(request.Operation),
	}
	return modelinference.Result{}, classifyModelInvocationError(
		fmt.Errorf("factory service runtime is not available"), failureContext,
	)
}

func classifyModelInvocationError(err error, context workers.InferenceFailureContext) error {
	failure, ok := workers.ClassifyInferenceFailure(err, context)
	if !ok {
		return err
	}
	return failure
}

func (s *Service) ensureAndRecordInvocationReady(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	modelName string,
	operation string,
) error {
	managed, err := s.ensureInvocationReady(ctx, runtimeCfg, modelName, operation)
	s.recordManagedRuntimeInvocationReadiness(modelName, managed, err)
	return err
}

func (s *Service) ensureInvocationReady(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	modelName string,
	operation string,
) (modelinference.Runtime, error) {
	if runtimeCfg == nil {
		return modelinference.Runtime{}, fmt.Errorf("runtime config is not available")
	}
	if s == nil || s.models == nil {
		return modelinference.Runtime{}, fmt.Errorf("Models service is not available")
	}
	if s.modelsScope.IsZero() {
		return modelinference.Runtime{}, modelinference.ErrRuntimeScopeInvalid
	}
	readiness, err := s.models.GetModelReadiness(ctx, modelinference.GetModelReadinessRequest{
		Scope:     s.modelsScope,
		Name:      modelName,
		Operation: operation,
	})
	if err != nil {
		return readiness.Readiness, err
	}
	return readiness.Readiness, readiness.Readiness.InvocationError()
}

func (s *Service) recordManagedRuntimeInvocationReadiness(
	modelName string,
	managed modelinference.Runtime,
	err error,
) {
	var logger *zap.Logger
	if s != nil {
		logger = s.logger
	}
	if logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("model_name", modelName),
		zap.String("managed_runtime_identity", managed.Identity),
		zap.String("readiness_state", string(managed.ReadinessState)),
		zap.String("lifecycle_state", string(managed.LifecycleState)),
	}
	if err != nil {
		logger.Warn("managed runtime invocation blocked", append(fields, zap.Error(err))...)
		return
	}
	logger.Info("managed runtime invocation readiness satisfied", fields...)
}

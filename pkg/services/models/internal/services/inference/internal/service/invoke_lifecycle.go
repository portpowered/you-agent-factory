package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

const defaultInferenceExecutionDeadline = 30 * time.Minute

func defaultExecutionDeadline() time.Duration {
	return defaultInferenceExecutionDeadline
}

func (s *service) putInvocation(invocation models.ModelInvocationRef, result models.InvokeModelResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invocations[invocation] = result.Clone()
}

func (s *service) invokeWithDeadline(parent context.Context) (context.Context, context.CancelFunc) {
	if s == nil || s.executionDeadline == nil {
		return parent, func() {}
	}
	duration := s.executionDeadline()
	if duration <= 0 {
		return parent, func() {}
	}
	if parentDeadline, ok := parent.Deadline(); ok && s.clock != nil {
		remaining := parentDeadline.Sub(s.clock())
		if remaining > 0 && remaining < duration {
			return parent, func() {}
		}
	}
	return context.WithTimeout(parent, duration)
}

func acceptedInvocationResult(
	request models.InvokeModelRequest,
	invocation models.ModelInvocationRef,
) models.InvokeModelResult {
	return models.InvokeModelResult{
		Invocation:       invocation,
		Scope:            request.Scope,
		Lease:            request.Lease,
		ModelName:        request.ModelName,
		Operation:        request.Operation,
		Status:           models.ModelInvocationStatusAccepted,
		LeaseDisposition: models.InvocationLeaseRetained,
	}.Clone()
}

func cancelledInvocationResult(
	request models.InvokeModelRequest,
	invocation models.ModelInvocationRef,
) models.InvokeModelResult {
	return models.InvokeModelResult{
		Invocation:          invocation,
		Scope:               request.Scope,
		Lease:               request.Lease,
		ModelName:           request.ModelName,
		Operation:           request.Operation,
		Status:              models.ModelInvocationStatusCancelled,
		LeaseDisposition:    models.InvocationLeaseReleased,
		CancellationOutcome: models.InvocationCancellationRequested,
	}.Clone()
}

func (s *service) finishCancelledInvocation(
	ctx context.Context,
	request models.InvokeModelRequest,
	invocation models.ModelInvocationRef,
	cause error,
) (models.InvokeModelResult, error) {
	result := cancelledInvocationResult(request, invocation)
	s.putInvocation(invocation, result)
	s.releaseInvocationLease(ctx, request)
	return result.Clone(), classifyInvokeCancellationError(cause)
}

func (s *service) finishFailedInvocation(
	invokeCtx context.Context,
	request models.InvokeModelRequest,
	invocation models.ModelInvocationRef,
	runtimeErr error,
) (models.InvokeModelResult, error) {
	classified := classifyInvokeRuntimeError(invokeCtx, runtimeErr)
	if isPrivateOMNIOperation(request.Operation) {
		classified = classifyPrivateOMNIError(classified)
	}
	result := failedInvocationResult(request, invocation)
	if errors.Is(classified, models.ErrInferenceCancelled) {
		result = cancelledInvocationResult(request, invocation)
	}
	s.putInvocation(invocation, result)
	s.releaseInvocationLease(invokeCtx, request)
	return result.Clone(), classified
}

func (s *service) finishCompletedInvocation(
	ctx context.Context,
	request models.InvokeModelRequest,
	invocation models.ModelInvocationRef,
	runtimeResult inference.InvocationRuntimeResult,
	operation models.Operation,
) (models.InvokeModelResult, error) {
	if isPrivateOMNIOperation(request.Operation) {
		if err := validatePrivateOMNIResult(runtimeResult); err != nil {
			return s.finishFailedInvocation(ctx, request, invocation, err)
		}
	}
	artifacts, err := s.registerInvocationArtifacts(runtimeResult.Artifacts)
	if err != nil {
		return s.finishFailedInvocation(ctx, request, invocation, err)
	}
	var outputs []models.InferenceOutput
	if request.UsesGenericInvocationShape() {
		outputs, err = models.NormalizeGenericInvocationOutputs(
			operation,
			runtimeResult.Content,
			artifacts,
		)
		if err != nil {
			return s.finishFailedInvocation(ctx, request, invocation, err)
		}
	}
	result := models.InvokeModelResult{
		Invocation:       invocation,
		Scope:            request.Scope,
		Lease:            request.Lease,
		ModelName:        request.ModelName,
		Operation:        request.Operation,
		Status:           models.ModelInvocationStatusCompleted,
		Content:          append([]models.InferenceContent(nil), runtimeResult.Content...),
		Artifacts:        artifacts,
		Outputs:          outputs,
		LeaseDisposition: models.InvocationLeaseReleased,
	}.Clone()
	s.putInvocation(invocation, result)
	s.releaseInvocationLease(ctx, request)
	return result, nil
}

func (s *service) registerInvocationArtifacts(
	sources []inference.InvocationArtifactSource,
) ([]models.InferenceArtifact, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if s == nil || s.artifacts == nil {
		return nil, models.ErrUnavailable
	}
	artifacts := make([]models.InferenceArtifact, 0, len(sources))
	for _, source := range sources {
		artifact, err := s.artifacts.Register(source)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func classifyInvokeCancellationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, models.ErrInferenceCancelled) {
		return err
	}
	return fmt.Errorf("%w: %v", models.ErrInferenceCancelled, err)
}

func classifyInvokeRuntimeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var invocationFailure *models.InvocationFailure
	if errors.As(err, &invocationFailure) {
		return err
	}
	if errors.Is(err, inference.ErrInvocationInFlight) {
		return err
	}
	if errors.Is(err, models.ErrInferenceTimeout) {
		return err
	}
	if errors.Is(err, models.ErrInferenceCancelled) {
		return err
	}
	if errors.Is(err, models.ErrInferenceArtifactInvalid) {
		return err
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return models.ErrInferenceTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("%w: %v", models.ErrInferenceCancelled, ctx.Err())
	}
	return normalizeInvokeError(err)
}

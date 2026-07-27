package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func invokeContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", models.ErrInferenceCancelled, err)
	}
	return nil
}

func validateInvocationResponseMode(request models.InvokeModelRequest) error {
	if request.ResponseMode != "" &&
		request.ResponseMode != models.ResponseModeAudioStream {
		return models.ErrUnsupportedResponseMode
	}
	return nil
}

func releasesInferenceCapacity(err error) bool {
	return errors.Is(err, models.ErrInferenceTimeout) ||
		errors.Is(err, models.ErrInferenceFailed)
}

func normalizeInvokeError(err error) error {
	if err == nil {
		return nil
	}
	if releasesInferenceCapacity(err) {
		return err
	}
	return fmt.Errorf("%w: %v", models.ErrInferenceFailed, err)
}

func failedInvocationResult(
	request models.InvokeModelRequest,
	invocation models.ModelInvocationRef,
) models.InvokeModelResult {
	return models.InvokeModelResult{
		Invocation:       invocation,
		Scope:            request.Scope,
		Lease:            request.Lease,
		ModelName:        request.ModelName,
		Operation:        request.Operation,
		Status:           models.ModelInvocationStatusFailed,
		LeaseDisposition: models.InvocationLeaseReleased,
	}.Clone()
}

func validateInvocationLease(
	request models.InvokeModelRequest,
	lease models.ModelLease,
) error {
	if lease.Scope != request.Scope {
		return models.ErrHostLeaseNotFound
	}
	if lease.ModelName != request.ModelName {
		return models.ErrHostLeaseNotFound
	}
	if strings.TrimSpace(lease.Holder) != strings.TrimSpace(request.Holder) {
		return models.ErrHostInvalidHolder
	}
	if lease.Status != models.ModelLeaseStatusActive {
		return models.ErrHostLeaseNotFound
	}
	return nil
}

func catalogInvokeError(err error) error {
	if errors.Is(err, models.ErrUnsupportedOperation) {
		return models.ErrUnsupportedModelOperation
	}
	return err
}

func (s *service) nextInvocationRef() (models.ModelInvocationRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextInvocation++
	return (models.ModelInvocationRef{}).Parse(
		fmt.Sprintf("models-inference:%d", s.nextInvocation),
	)
}

func isInvocationInFlight(err error) bool {
	return errors.Is(err, inference.ErrInvocationInFlight)
}

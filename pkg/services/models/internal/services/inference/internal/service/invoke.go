package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

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

func (s *service) nextInvocationRef() (models.ModelInvocationRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextInvocation++
	return (models.ModelInvocationRef{}).Parse(
		fmt.Sprintf("models-inference:%d", s.nextInvocation),
	)
}

func catalogInvokeError(err error) error {
	if errors.Is(err, models.ErrUnsupportedOperation) {
		return models.ErrUnsupportedModelOperation
	}
	return err
}

func invokeContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

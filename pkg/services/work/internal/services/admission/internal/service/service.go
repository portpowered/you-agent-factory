package service

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
)

// Service is the private admission implementation. Story-001 lands the
// FND-02 shell; normalize/validate and idempotent-accept behavior arrive in
// later IMP-WORK-01 stories.
type Service struct{}

var _ admission.Service = (*Service)(nil)

var errAdmissionShell = errors.New("Work admission subservice shell is not yet implemented")

// New constructs the private admission subservice implementation shell.
func New() *Service {
	return &Service{}
}

func (s *Service) Normalize(
	ctx context.Context,
	_ admission.NormalizeRequest,
) (admission.NormalizeResult, error) {
	if err := requireContext(ctx); err != nil {
		return admission.NormalizeResult{}, err
	}
	return admission.NormalizeResult{}, errAdmissionShell
}

func (s *Service) Validate(ctx context.Context, _ admission.ValidateRequest) error {
	if err := requireContext(ctx); err != nil {
		return err
	}
	return errAdmissionShell
}

func (s *Service) Accept(
	ctx context.Context,
	_ admission.AcceptRequest,
) (work.WorkRequestSubmitResult, error) {
	if err := requireContext(ctx); err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return work.WorkRequestSubmitResult{}, errAdmissionShell
}

func requireContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Work admission context is required")
	}
	return ctx.Err()
}

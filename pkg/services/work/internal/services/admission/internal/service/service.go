package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
)

// Service is the private admission implementation. Normalize/validate live
// here for IMP-WORK-01; idempotent accept arrives in a later story.
type Service struct{}

var _ admission.Service = (*Service)(nil)

var errAdmissionAcceptShell = errors.New("Work admission accept is not yet implemented")

// New constructs the private admission subservice implementation.
func New() *Service {
	return &Service{}
}

func (s *Service) Normalize(
	ctx context.Context,
	request admission.NormalizeRequest,
) (admission.NormalizeResult, error) {
	if err := requireContext(ctx); err != nil {
		return admission.NormalizeResult{}, err
	}
	return normalizeAdmission(request.Request, request.Options)
}

func (s *Service) Validate(ctx context.Context, request admission.ValidateRequest) error {
	if err := requireContext(ctx); err != nil {
		return err
	}
	_, err := normalizeAdmission(request.Request, request.Options)
	return err
}

func (s *Service) Accept(
	ctx context.Context,
	_ admission.AcceptRequest,
) (work.WorkRequestSubmitResult, error) {
	if err := requireContext(ctx); err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return work.WorkRequestSubmitResult{}, errAdmissionAcceptShell
}

func normalizeAdmission(
	request work.WorkRequest,
	opts work.WorkRequestNormalizeOptions,
) (admission.NormalizeResult, error) {
	normalized, err := work.NormalizeWorkRequest(request, opts)
	if err != nil {
		return admission.NormalizeResult{}, mapNormalizeFailure(err)
	}
	requestID := request.RequestID
	if requestID == "" && len(normalized) > 0 {
		requestID = normalized[0].RequestID
	}
	return admission.NormalizeResult{
		RequestID:  requestID,
		Normalized: normalized,
	}, nil
}

func mapNormalizeFailure(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, work.ErrInvalidWorkRequest) ||
		errors.Is(err, work.ErrWorkRequestRejected) ||
		errors.Is(err, work.ErrWorkRequestConflict) {
		return err
	}
	return fmt.Errorf("%w: %w", work.ErrInvalidWorkRequest, err)
}

func requireContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Work admission context is required")
	}
	return ctx.Err()
}

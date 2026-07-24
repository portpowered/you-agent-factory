package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
)

// Service is the private admission implementation for normalize, validate, and
// idempotent accept behind the CTR-WORK admission slice.
type Service struct {
	mu       sync.Mutex
	accepted map[string]acceptedAdmission
}

type acceptedAdmission struct {
	normalized []work.SubmitRequest
}

var _ admission.Service = (*Service)(nil)

// New constructs the private admission subservice implementation.
func New() *Service {
	return &Service{
		accepted: make(map[string]acceptedAdmission),
	}
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
	request admission.AcceptRequest,
) (work.WorkRequestSubmitResult, error) {
	if err := requireContext(ctx); err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	if request.RequestID == "" || len(request.Normalized) == 0 {
		return work.WorkRequestSubmitResult{}, fmt.Errorf(
			"%w: admission accept requires request id and normalized works",
			work.ErrWorkRequestRejected,
		)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.accepted[request.RequestID]; ok {
		if admissionBatchesEquivalent(existing.normalized, request.Normalized) {
			return work.WorkRequestSubmitResult{}, fmt.Errorf(
				"%w: request %q was already applied",
				work.ErrWorkRequestConflict,
				request.RequestID,
			)
		}
		return work.WorkRequestSubmitResult{}, fmt.Errorf(
			"%w: request %q conflicts with previously accepted admission state",
			work.ErrWorkRequestConflict,
			request.RequestID,
		)
	}

	result := work.SubmitResultFromNormalized(request.RequestID, request.Normalized)
	s.accepted[request.RequestID] = acceptedAdmission{
		normalized: cloneSubmitRequests(request.Normalized),
	}
	return result, nil
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

func admissionBatchesEquivalent(left, right []work.SubmitRequest) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].RequestID != right[i].RequestID ||
			left[i].WorkID != right[i].WorkID ||
			left[i].Name != right[i].Name ||
			left[i].WorkTypeID != right[i].WorkTypeID ||
			left[i].CurrentChainingTraceID != right[i].CurrentChainingTraceID ||
			left[i].TraceID != right[i].TraceID {
			return false
		}
	}
	return true
}

func cloneSubmitRequests(in []work.SubmitRequest) []work.SubmitRequest {
	if len(in) == 0 {
		return nil
	}
	out := make([]work.SubmitRequest, len(in))
	copy(out, in)
	return out
}

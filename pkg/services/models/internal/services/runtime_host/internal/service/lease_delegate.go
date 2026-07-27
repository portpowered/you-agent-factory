package service

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (s *service) AcquireModelLease(
	ctx context.Context,
	request models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	if s == nil || s.leases == nil {
		return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
	}
	return s.leases.AcquireModelLease(ctx, request)
}

func (s *service) GetModelLease(
	ctx context.Context,
	request models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	if s == nil || s.leases == nil {
		return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
	}
	return s.leases.GetModelLease(ctx, request)
}

func (s *service) ReleaseModelLease(
	ctx context.Context,
	request models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	if s == nil || s.leases == nil {
		return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
	}
	return s.leases.ReleaseModelLease(ctx, request)
}

package service

import (
	"context"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
)

type service struct {
	hostClock       models.HostClock
	mu              sync.Mutex
	leases          map[string]leaseRecord
	capacityHolders map[string]int
}

type leaseRecord struct {
	lease models.ModelLease
}

var _ hostleases.Service = (*service)(nil)

// New constructs an inert leases owner that retains injected effects and
// allocates lease/capacity state without launching subprocesses or timers.
func New(hostClock models.HostClock) hostleases.Service {
	return &service{
		hostClock:       hostClock,
		leases:          make(map[string]leaseRecord),
		capacityHolders: make(map[string]int),
	}
}

func (s *service) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (s *service) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (s *service) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

package service

import (
	"context"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
)

// Scoped host and lease lifecycle is contract-only until the Models
// implementation packet owns runtime-scope registration and lease expiry.
func (s *Service) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (s *Service) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (s *Service) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (s *Service) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (s *Service) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (s *Service) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

// Scoped inference is contract-only until the Models implementation packet
// owns runtime-scope registration and invocation cancellation state.
func (s *Service) InvokeModelWithLease(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (s *Service) CancelInvocation(
	context.Context,
	models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}

// AcquireLease acquires Models-owned local capacity through the singular root
// Service host/lease surface.
func (s *Service) AcquireLease(ctx context.Context, request models.AcquireLeaseRequest) (models.HostLease, error) {
	if err := models.ValidateAcquireLeaseRequest(request); err != nil {
		return models.HostLease{}, err
	}
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return models.HostLease{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		return models.HostLease{}, models.ErrHostRuntimeNotReady
	}
	return host.AcquireLease(ctx, runtimeCfg, request.ModelName, modelhost.LeaseOptions{
		Holder: request.Holder,
	})
}

// ReleaseLease releases one Models-owned HostLease through the singular root
// Service host/lease surface.
func (s *Service) ReleaseLease(ctx context.Context, request models.ReleaseLeaseRequest) error {
	if err := models.ValidateReleaseLeaseRequest(request); err != nil {
		return err
	}
	host := s.modelHost()
	if host == nil {
		return models.ErrHostLeaseNotFound
	}
	return host.ReleaseLease(ctx, request.LeaseID)
}

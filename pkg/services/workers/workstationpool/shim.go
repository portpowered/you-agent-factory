// Package workstationpool is a transitional compile shim that re-exports
// workstation pool boundary construction from the private workstations
// destination. Canonical pool-boundary implementation lives under
// pkg/services/workers/internal/services/workstations/poolboundary; baseline
// deletion of this path is owned by DEL-WRK.
package workstationpool

import (
	"context"

	workstationpoolboundary "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/poolboundary"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const DefaultRuntimePoolBindingCapacity = workstationpoolboundary.DefaultRuntimePoolBindingCapacity

type (
	WorkstationExecutionService   = workstationpoolboundary.WorkstationExecutionService
	WorkstationDispatchAcceptFunc = workstationpoolboundary.WorkstationDispatchAcceptFunc
	WorkstationPoolBoundary       = workstationpoolboundary.WorkstationPoolBoundary
	WorkstationPoolBoundaryConfig = workstationpoolboundary.WorkstationPoolBoundaryConfig
)

var NewWorkstationPoolBoundary = workstationpoolboundary.NewWorkstationPoolBoundary

// WorkstationExecutionServiceFromRoot adapts the published Workers root service
// to the pool-boundary execution port.
func WorkstationExecutionServiceFromRoot(service workers.Service) WorkstationExecutionService {
	return rootWorkstationExecutionService{service: service}
}

type rootWorkstationExecutionService struct {
	service workers.Service
}

func (a rootWorkstationExecutionService) StartWorkstationPool(
	ctx context.Context,
	request workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return a.service.StartWorkstationPool(ctx, request)
}

func (a rootWorkstationExecutionService) StopWorkstationPool(
	ctx context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return a.service.StopWorkstationPool(ctx)
}

func (a rootWorkstationExecutionService) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return a.service.DispatchWorkstation(ctx, request)
}

func (a rootWorkstationExecutionService) CancelWorkstationDispatch(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return a.service.CancelWorkstationDispatch(ctx, request)
}

package internal

import (
	"context"
	"fmt"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Root is the inert Workers root composed from parent-private runtime assembly
// and workstation owners. It starts no lifecycle, runner execution, or
// workstation pool admission.
type Root struct {
	runtimeAssembly runtimeassembly.Service
	workstations    workstations.Service
}

var _ workers.Service = (*Root)(nil)

// NewRoot constructs the inert Workers root from parent-private runtime
// assembly and workstation owners.
func NewRoot(
	runtimeAssembly runtimeassembly.Service,
	workstationsOwner workstations.Service,
) (workers.Service, error) {
	if runtimeAssembly == nil {
		return nil, fmt.Errorf("construct Workers: runtime assembly owner is required")
	}
	if workstationsOwner == nil {
		return nil, fmt.Errorf("construct Workers: workstations owner is required")
	}
	return &Root{
		runtimeAssembly: runtimeAssembly,
		workstations:    workstationsOwner,
	}, nil
}

// RootFrom composes a Workers root value from parent-private owners for
// owner-local runtime construction and testing.
func RootFrom(
	runtimeAssembly runtimeassembly.Service,
	workstationsOwner workstations.Service,
) Root {
	return Root{
		runtimeAssembly: runtimeAssembly,
		workstations:    workstationsOwner,
	}
}

// ReplaceRuntimeAssembly returns a copy with an updated runtime assembly owner.
func (r Root) ReplaceRuntimeAssembly(runtimeAssembly runtimeassembly.Service) Root {
	r.runtimeAssembly = runtimeAssembly
	return r
}

// ReplaceWorkstations returns a copy with an updated workstation owner.
func (r Root) ReplaceWorkstations(workstationsOwner workstations.Service) Root {
	r.workstations = workstationsOwner
	return r
}

// BuildRuntime delegates the singular Workers root operation to its
// parent-private Runtime Assembly capability.
func (r *Root) BuildRuntime(
	ctx context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	if r == nil || r.runtimeAssembly == nil {
		return workers.RuntimeBuildResult{}, fmt.Errorf(
			"%w: Workers Runtime Assembly is required",
			workers.ErrIncompleteRuntimeAssembly,
		)
	}
	return r.runtimeAssembly.Build(ctx, request)
}

// StartWorkstationPool delegates lifecycle activation to the parent-private
// workstation capability.
func (r *Root) StartWorkstationPool(
	ctx context.Context,
	request workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationPoolStartResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Start(ctx, request)
}

// StopWorkstationPool delegates terminal shutdown to the parent-private
// workstation capability.
func (r *Root) StopWorkstationPool(
	ctx context.Context,
) (workers.WorkstationPoolStopResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationPoolStopResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Stop(ctx)
}

// WorkstationRoute reports availability through the private lifecycle owner.
func (r *Root) WorkstationRoute(
	ctx context.Context,
	request workers.WorkstationRouteRequest,
) (workers.WorkstationRouteResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationRouteResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Route(ctx, request)
}

// DispatchWorkstation delegates execution to the private workstation owner.
func (r *Root) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Dispatch(ctx, request)
}

// CancelWorkstationDispatch delegates explicit cancellation to the private
// workstation owner.
func (r *Root) CancelWorkstationDispatch(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Cancel(ctx, request)
}

// InvokeModel is unavailable on the inert Workers root; direct model invocation
// requires a constructed runtime service.
func (r *Root) InvokeModel(
	context.Context,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, fmt.Errorf("factory service runtime is not available")
}

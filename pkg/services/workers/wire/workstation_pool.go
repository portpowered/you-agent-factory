package wire

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
)

// NewWorkstationPool constructs a standalone Workers workstation pool: the
// admission, capacity, and cancellation capability, with no runner selection,
// prompt rendering, or worktree machinery attached.
//
// It exists so a session whose Factory has no authored workstations can still
// run its Workers through the one real execution route. A JavaScript Factory is
// exactly that case: its children are Workers, but there is no Petri runtime
// composed for it and therefore no pool. Without this, such a session has no
// way to reach Worker Sessions at all, and children end up executing through a
// private path that the Workers contract already declares must not exist.
//
// The pool is inert until Start commits its binding snapshot, and the bindings
// carry their own executors, so this constructor needs nothing but a logger.
func NewWorkstationPool(logger logging.Logger) workers.WorkstationExecutionService {
	return workstationPool{service: workstationswire.NewService(logging.EnsureLogger(logger))}
}

// workstationPool adapts the owner-private workstation capability onto the
// published execution port. The two differ only in method naming: the private
// owner is already scoped to workstations, while the published port names the
// subject because peers see it beside unrelated Workers operations.
type workstationPool struct {
	service workstations.Service
}

var _ workers.WorkstationExecutionService = workstationPool{}

func (p workstationPool) StartWorkstationPool(
	ctx context.Context,
	request workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return p.service.Start(ctx, request)
}

func (p workstationPool) StopWorkstationPool(
	ctx context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return p.service.Stop(ctx)
}

func (p workstationPool) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return p.service.Dispatch(ctx, request)
}

func (p workstationPool) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	return p.service.DispatchWithAdmission(ctx, request, admitted)
}

func (p workstationPool) CancelWorkstationDispatch(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return p.service.Cancel(ctx, request)
}

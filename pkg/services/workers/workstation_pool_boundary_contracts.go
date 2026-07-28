package workers

import (
	"context"
)

// WorkstationExecutionService is the narrow Workers pool API Factory Runtime
// dispatch planning consumes without importing Workers implementation packages.
type WorkstationExecutionService interface {
	StartWorkstationPool(context.Context, WorkstationPoolStartRequest) (WorkstationPoolStartResult, error)
	StopWorkstationPool(context.Context) (WorkstationPoolStopResult, error)
	DispatchWorkstation(context.Context, WorkstationDispatchRequest) (WorkstationDispatchResult, error)
	CancelWorkstationDispatch(context.Context, WorkstationDispatchCancelRequest) (WorkstationDispatchCancelResult, error)
}

// WorkstationDispatchAcceptFunc receives one detached dispatch result from an
// asynchronous or synchronous workstation publish.
type WorkstationDispatchAcceptFunc func(
	context.Context,
	WorkstationDispatchRequest,
	WorkstationDispatchResult,
	error,
)

// WorkstationPoolBoundary owns workstation pool lifecycle and dispatch
// execution for one Factory Runtime session. Runtime plans identities and
// observes results; Workers owns route admission, capacity, executor
// invocation, and cancellation.
type WorkstationPoolBoundary interface {
	Start(context.Context) error
	Publish(
		context.Context,
		WorkstationDispatchRequest,
		WorkstationDispatchAcceptFunc,
	) error
	Cancel(context.Context, WorkstationDispatchCancelRequest) (WorkstationDispatchCancelResult, error)
	Stop(context.Context) error
}

// WorkstationPoolBoundaryConfig assembles one immutable workstation-route
// snapshot for a runtime session.
type WorkstationPoolBoundaryConfig struct {
	Service       WorkstationExecutionService
	Executors     map[string]WorkerExecutor
	RouteNames    []string
	Async         bool
	Capacity      int
	QueueCapacity int
}

// DefaultRuntimePoolBindingCapacity preserves legacy Factory Runtime pool
// concurrency when assembling workstation bindings for a runtime session.
const DefaultRuntimePoolBindingCapacity = 64

// WorkstationExecutionServiceFromRoot adapts the published Workers root service
// to the pool-boundary execution port.
func WorkstationExecutionServiceFromRoot(service Service) WorkstationExecutionService {
	return rootWorkstationExecutionService{service: service}
}

type rootWorkstationExecutionService struct {
	service Service
}

func (a rootWorkstationExecutionService) StartWorkstationPool(
	ctx context.Context,
	request WorkstationPoolStartRequest,
) (WorkstationPoolStartResult, error) {
	return a.service.StartWorkstationPool(ctx, request)
}

func (a rootWorkstationExecutionService) StopWorkstationPool(
	ctx context.Context,
) (WorkstationPoolStopResult, error) {
	return a.service.StopWorkstationPool(ctx)
}

func (a rootWorkstationExecutionService) DispatchWorkstation(
	ctx context.Context,
	request WorkstationDispatchRequest,
) (WorkstationDispatchResult, error) {
	return a.service.DispatchWorkstation(ctx, request)
}

func (a rootWorkstationExecutionService) CancelWorkstationDispatch(
	ctx context.Context,
	request WorkstationDispatchCancelRequest,
) (WorkstationDispatchCancelResult, error) {
	return a.service.CancelWorkstationDispatch(ctx, request)
}

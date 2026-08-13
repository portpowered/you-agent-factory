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
	DispatchWorkstationWithAdmission(context.Context, WorkstationDispatchRequest, WorkstationDispatchAdmissionFunc) (WorkstationDispatchResult, error)
	CancelWorkstationDispatch(context.Context, WorkstationDispatchCancelRequest) (WorkstationDispatchCancelResult, error)
}

// WorkstationDispatchAdmissionFunc observes the exact instant Workers accepts
// a dispatch into its cancellable queue or running set. It is invoked at most
// once, only after Cancel can address the dispatch ID, and must return
// promptly. The callback is a Workers boundary synchronization signal; it
// does not own dispatch execution or terminal-result authority.
type WorkstationDispatchAdmissionFunc func()

// WorkstationDispatchAcceptFunc receives one detached dispatch result from an
// asynchronous or synchronous workstation publish. Publish returns only after
// Workers has called its admission callback or has delivered its terminal
// callback without admitting the dispatch, so callers can safely issue exact
// cancellation immediately after a successful publish.
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
	// PublishWithAdmission preserves Publish's terminal callback and return
	// semantics while exposing the Workers-owned point at which the exact
	// dispatch becomes cancellable. The callback is independent of Publish's
	// return so synchronous publishers can be controlled while waiting for
	// their terminal result.
	PublishWithAdmission(
		context.Context,
		WorkstationDispatchRequest,
		WorkstationDispatchAdmissionFunc,
		WorkstationDispatchAcceptFunc,
	) error
	Cancel(context.Context, WorkstationDispatchCancelRequest) (WorkstationDispatchCancelResult, error)
	Stop(context.Context) error
}

// WorkstationPoolBoundaryConfig assembles one immutable workstation-route
// snapshot for a runtime session.
type WorkstationPoolBoundaryConfig struct {
	// Service is the compatibility pool service used when no factory is
	// supplied. Production composition should provide ServiceFactory so every
	// Factory Runtime receives a private pool owner rather than mutating the
	// process-scoped Workers root.
	Service WorkstationExecutionService
	// ServiceFactory constructs the pool owner for this runtime. The returned
	// service must be inert until StartWorkstationPool receives the immutable
	// route bindings for this runtime.
	ServiceFactory func() WorkstationExecutionService
	Executors      map[string]WorkerExecutor
	RouteNames     []string
	// ProviderInvocation is the executor for Workers with no authored
	// workstation behind them, published under ProviderInvocationRoute. It is
	// assembled as a worker-role binding rather than a workstation-role one
	// because that is exactly what it is: a Worker whose selections were
	// already resolved by its caller, so there is no workstation to render.
	//
	// A nil value omits the route entirely, which is the correct shape for a
	// session that has no orchestrator able to produce such a Worker.
	ProviderInvocation WorkstationRequestExecutor
	Async              bool
	Capacity           int
	QueueCapacity      int
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

func (a rootWorkstationExecutionService) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request WorkstationDispatchRequest,
	admitted WorkstationDispatchAdmissionFunc,
) (WorkstationDispatchResult, error) {
	return a.service.DispatchWorkstationWithAdmission(ctx, request, admitted)
}

func (a rootWorkstationExecutionService) CancelWorkstationDispatch(
	ctx context.Context,
	request WorkstationDispatchCancelRequest,
) (WorkstationDispatchCancelResult, error) {
	return a.service.CancelWorkstationDispatch(ctx, request)
}

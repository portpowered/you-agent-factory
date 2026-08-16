package workers

import "context"

// WorkstationExecutionService is the narrow Workers execution API consumed by
// Worker Sessions and Factory Runtime. Workers owns route admission, capacity,
// executor invocation, cancellation, and terminal results behind this port.
type WorkstationExecutionService interface {
	StartWorkstationPool(context.Context, WorkstationPoolStartRequest) (WorkstationPoolStartResult, error)
	StopWorkstationPool(context.Context) (WorkstationPoolStopResult, error)
	DispatchWorkstation(context.Context, WorkstationDispatchRequest) (WorkstationDispatchResult, error)
	DispatchWorkstationWithAdmission(context.Context, WorkstationDispatchRequest, WorkstationDispatchAdmissionFunc) (WorkstationDispatchResult, error)
	CancelWorkstationDispatch(context.Context, WorkstationDispatchCancelRequest) (WorkstationDispatchCancelResult, error)
}

// WorkstationDispatchAdmissionFunc observes the exact instant Workers accepts
// a dispatch into its cancellable queue or running set. It is invoked at most
// once, only after cancellation can address the dispatch ID, and must return
// promptly.
type WorkstationDispatchAdmissionFunc func()

// WorkstationDispatchAcceptFunc receives one detached dispatch result from a
// runtime-owned asynchronous dispatch operation.
type WorkstationDispatchAcceptFunc func(
	context.Context,
	WorkstationDispatchRequest,
	WorkstationDispatchResult,
	error,
)

// DefaultRuntimePoolBindingCapacity preserves the established Factory Runtime
// pool concurrency when assembling workstation bindings for one session.
const DefaultRuntimePoolBindingCapacity = 64

// WorkstationExecutionServiceFromRoot exposes the Workers root execution
// capability through the narrow service port without leaking the aggregate
// Workers implementation.
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

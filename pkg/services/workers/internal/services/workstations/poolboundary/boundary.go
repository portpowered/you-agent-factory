package poolboundary

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// DefaultRuntimePoolBindingCapacity preserves legacy Factory Runtime pool
// concurrency when assembling workstation bindings for a runtime session.
const DefaultRuntimePoolBindingCapacity = 64

// WorkstationExecutionService is the narrow Workers pool API Factory Runtime
// dispatch planning consumes without importing Workers implementation packages.
type WorkstationExecutionService interface {
	StartWorkstationPool(context.Context, workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error)
	StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error)
	DispatchWorkstation(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	CancelWorkstationDispatch(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
}

// WorkstationDispatchAcceptFunc receives one detached dispatch result from an
// asynchronous or synchronous workstation publish.
type WorkstationDispatchAcceptFunc func(
	context.Context,
	workers.WorkstationDispatchRequest,
	workers.WorkstationDispatchResult,
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
		workers.WorkstationDispatchRequest,
		WorkstationDispatchAcceptFunc,
	) error
	Cancel(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
	Stop(context.Context) error
}

// WorkstationPoolBoundaryConfig assembles one immutable workstation-route
// snapshot for a runtime session.
type WorkstationPoolBoundaryConfig struct {
	Service       WorkstationExecutionService
	Executors     map[string]workers.WorkerExecutor
	RouteNames    []string
	Async         bool
	Capacity      int
	QueueCapacity int
}

// NewWorkstationPoolBoundary constructs a Workers-owned pool boundary from
// detached executor bindings and route names supplied by the runtime peer.
func NewWorkstationPoolBoundary(cfg WorkstationPoolBoundaryConfig) WorkstationPoolBoundary {
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = DefaultRuntimePoolBindingCapacity
	}
	queueCapacity := cfg.QueueCapacity
	if queueCapacity <= 0 {
		queueCapacity = DefaultRuntimePoolBindingCapacity
	}
	adapter := workerExecutorRequestAdapter{executors: cfg.Executors}
	bindings := assembleWorkstationPoolBindings(cfg.RouteNames, adapter, capacity, queueCapacity)
	return &workstationPoolBoundary{
		service:  cfg.Service,
		bindings: bindings,
		async:    cfg.Async,
	}
}

type workstationPoolBoundary struct {
	service  WorkstationExecutionService
	bindings []workers.AssembledRuntimeBinding
	async    bool
	started  bool
	stopped  bool
	mu       sync.Mutex
}

type workerExecutorRequestAdapter struct {
	executors map[string]workers.WorkerExecutor
}

func (a workerExecutorRequestAdapter) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (result workers.WorkResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = workers.WorkResult{
				DispatchID: request.Dispatch.DispatchID, TransitionID: request.Dispatch.TransitionID,
				Outcome: workers.OutcomeFailed,
				Error:   fmt.Sprintf("executor panic: %v", recovered),
			}
			err = nil
		}
	}()
	workerType := request.WorkerType
	if workerType == "" {
		workerType = request.Dispatch.WorkerType
	}
	executor := a.executors[workerType]
	if executor == nil {
		return workers.WorkResult{}, fmt.Errorf(
			"no executor registered for worker type %q",
			workerType,
		)
	}
	return executor.Execute(ctx, request.Dispatch)
}

func assembleWorkstationPoolBindings(
	routeNames []string,
	executor workers.WorkstationRequestExecutor,
	capacity int,
	queueCapacity int,
) []workers.AssembledRuntimeBinding {
	names := append([]string(nil), routeNames...)
	sort.Strings(names)
	bindings := make([]workers.AssembledRuntimeBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, workers.AssembledRuntimeBinding{
			RoleName:      name,
			RoleKind:      workers.RuntimeBuildRoleKindWorkstation,
			Executor:      executor,
			Capacity:      capacity,
			QueueCapacity: queueCapacity,
		})
	}
	return bindings
}

func (b *workstationPoolBoundary) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		return nil
	}
	if b.stopped {
		return workers.ErrWorkstationPoolStopped
	}
	if b.service == nil {
		return workers.ErrWorkstationPoolUnavailable
	}
	if _, err := b.service.StartWorkstationPool(
		ctx,
		workers.WorkstationPoolStartRequest{
			Bindings: append([]workers.AssembledRuntimeBinding(nil), b.bindings...),
		},
	); err != nil {
		return err
	}
	b.started = true
	return nil
}

func (b *workstationPoolBoundary) Publish(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	accept WorkstationDispatchAcceptFunc,
) error {
	if err := b.Start(ctx); err != nil {
		return err
	}
	execute := func() {
		result, err := b.service.DispatchWorkstation(context.WithoutCancel(ctx), request)
		accept(context.Background(), request, result, err)
	}
	if b.async {
		go execute()
		return nil
	}
	execute()
	return nil
}

func (b *workstationPoolBoundary) Cancel(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if b.service == nil {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return b.service.CancelWorkstationDispatch(ctx, request)
}

func (b *workstationPoolBoundary) Stop(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped || !b.started {
		b.stopped = true
		return nil
	}
	_, err := b.service.StopWorkstationPool(ctx)
	if err == nil {
		b.stopped = true
	}
	return err
}

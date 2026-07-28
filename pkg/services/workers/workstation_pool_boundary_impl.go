package workers

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

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
	bindings []AssembledRuntimeBinding
	async    bool
	started  bool
	stopped  bool
	mu       sync.Mutex
}

type workerExecutorRequestAdapter struct {
	executors map[string]WorkerExecutor
}

func (a workerExecutorRequestAdapter) Execute(
	ctx context.Context,
	request WorkstationExecutionRequest,
) (result WorkResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = WorkResult{
				DispatchID: request.Dispatch.DispatchID, TransitionID: request.Dispatch.TransitionID,
				Outcome: OutcomeFailed,
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
		return WorkResult{}, fmt.Errorf(
			"no executor registered for worker type %q",
			workerType,
		)
	}
	return executor.Execute(ctx, request.Dispatch)
}

func assembleWorkstationPoolBindings(
	routeNames []string,
	executor WorkstationRequestExecutor,
	capacity int,
	queueCapacity int,
) []AssembledRuntimeBinding {
	names := append([]string(nil), routeNames...)
	sort.Strings(names)
	bindings := make([]AssembledRuntimeBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, AssembledRuntimeBinding{
			RoleName:      name,
			RoleKind:      RuntimeBuildRoleKindWorkstation,
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
		return ErrWorkstationPoolStopped
	}
	if b.service == nil {
		return ErrWorkstationPoolUnavailable
	}
	if _, err := b.service.StartWorkstationPool(
		ctx,
		WorkstationPoolStartRequest{
			Bindings: append([]AssembledRuntimeBinding(nil), b.bindings...),
		},
	); err != nil {
		return err
	}
	b.started = true
	return nil
}

func (b *workstationPoolBoundary) Publish(
	ctx context.Context,
	request WorkstationDispatchRequest,
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
	request WorkstationDispatchCancelRequest,
) (WorkstationDispatchCancelResult, error) {
	if b.service == nil {
		return WorkstationDispatchCancelResult{}, ErrWorkstationPoolUnavailable
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

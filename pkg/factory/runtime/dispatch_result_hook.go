package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type workerPoolDispatchResultHook struct {
	net           *state.Net
	pool          *workers.WorkerPool
	executors     map[string]workers.WorkerExecutor
	logger        logging.Logger
	waitCh        chan struct{}
	results       []interfaces.WorkResult
	deliveryTicks map[string]int
	planner       factory.CompletionDeliveryPlanner
	mu            sync.Mutex
}

var _ factory.DispatchResultHook = (*workerPoolDispatchResultHook)(nil)

type replayTickValidator interface {
	ValidateReplayTick(currentTick int) error
}

type plannedCompletionResultProvider interface {
	PlannedResultForDispatch(dispatch interfaces.WorkDispatch) (interfaces.WorkResult, bool, error)
}

func newWorkerPoolDispatchResultHook(
	net *state.Net,
	pool *workers.WorkerPool,
	executors map[string]workers.WorkerExecutor,
	logger logging.Logger,
	buffer int,
	planner factory.CompletionDeliveryPlanner,
) *workerPoolDispatchResultHook {
	if buffer <= 0 {
		buffer = 1
	}
	return &workerPoolDispatchResultHook{
		net:           net,
		pool:          pool,
		executors:     executors,
		logger:        logging.EnsureLogger(logger),
		waitCh:        make(chan struct{}, buffer),
		deliveryTicks: make(map[string]int),
		planner:       planner,
	}
}

func (h *workerPoolDispatchResultHook) SubmitDispatch(_ context.Context, dispatch interfaces.WorkDispatch) error {
	tr, ok := h.net.Transitions[dispatch.TransitionID]
	if !ok {
		return fmt.Errorf("unknown transition %q", dispatch.TransitionID)
	}
	runnerKey := dispatchRunnerKey(tr, dispatch)
	deliveryTick, hasDeliveryTick := 0, false
	if h.planner != nil {
		tick, ok, err := h.planner.DeliveryTickForDispatch(dispatch)
		if err != nil {
			return err
		}
		if ok {
			deliveryTick, hasDeliveryTick = tick, true
		}
	}
	if hasDeliveryTick {
		h.mu.Lock()
		h.deliveryTicks[dispatch.DispatchID] = deliveryTick
		h.mu.Unlock()
	}
	if h.planner != nil {
		result := executeDispatchSynchronously(dispatch, runnerKey, h.executors)
		if provider, ok := h.planner.(plannedCompletionResultProvider); ok {
			planned, hasPlanned, err := provider.PlannedResultForDispatch(dispatch)
			if err != nil {
				return err
			}
			if hasPlanned && result.Outcome != interfaces.OutcomeFailed {
				result = planned
			}
		}
		h.mu.Lock()
		h.results = append(h.results, result)
		h.signalWaitLocked()
		h.mu.Unlock()
		return nil
	}
	if !h.pool.Dispatch(runnerKey, dispatch) {
		if hasDeliveryTick {
			h.mu.Lock()
			delete(h.deliveryTicks, dispatch.DispatchID)
			h.mu.Unlock()
		}
		return fmt.Errorf("no worker pool runner for worker type %q", runnerKey)
	}
	return nil
}

func (h *workerPoolDispatchResultHook) OnTick(_ context.Context, input interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.DispatchResultHookResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if validator, ok := h.planner.(replayTickValidator); ok {
		if err := validator.ValidateReplayTick(input.Snapshot.TickCount); err != nil {
			return interfaces.DispatchResultHookResult{}, err
		}
	}
	if len(h.results) == 0 {
		return interfaces.DispatchResultHookResult{}, nil
	}

	results := h.takeDueResults(input.Snapshot.TickCount)
	if len(h.results) > 0 {
		h.signalWaitLocked()
	}
	if len(results) == 0 {
		return interfaces.DispatchResultHookResult{}, nil
	}

	return interfaces.DispatchResultHookResult{Results: results}, nil
}

func (h *workerPoolDispatchResultHook) takeDueResults(currentTick int) []interfaces.WorkResult {
	results := make([]interfaces.WorkResult, 0, len(h.results))
	pending := h.results[:0]
	for _, result := range h.results {
		deliveryTick, delayed := h.deliveryTicks[result.DispatchID]
		if delayed && deliveryTick > currentTick {
			pending = append(pending, result)
			continue
		}
		if delayed {
			delete(h.deliveryTicks, result.DispatchID)
		}
		results = append(results, result)
	}
	h.results = pending
	return results
}

func (h *workerPoolDispatchResultHook) WaitCh() <-chan struct{} {
	return h.waitCh
}

func (h *workerPoolDispatchResultHook) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case result := <-h.pool.ResultCh():
				h.mu.Lock()
				h.results = append(h.results, result)
				h.signalWaitLocked()
				h.mu.Unlock()
			case <-ctx.Done():
				h.logger.Info("factory worker pool dispatch/result hook completed", "reason", ctx.Err())
				return
			}
		}
	}()
}

func (h *workerPoolDispatchResultHook) signalWaitLocked() {
	select {
	case h.waitCh <- struct{}{}:
	default:
	}
}

func executeDispatchSynchronously(
	dispatch interfaces.WorkDispatch,
	runnerKey string,
	executors map[string]workers.WorkerExecutor,
) interfaces.WorkResult {
	if exec, ok := executors[runnerKey]; ok {
		var (
			result interfaces.WorkResult
			err    error
		)
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					result = workers.PanicAsFailedResult(dispatch, recovered, 0)
					err = nil
				}
			}()
			result, err = exec.Execute(context.Background(), dispatch)
		}()
		if err == nil {
			return result
		}
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        err.Error(),
		}
	}
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeFailed,
		Error:        fmt.Sprintf("no executor registered for worker type %q (transition %s)", runnerKey, dispatch.TransitionID),
	}
}

func dispatchRunnerKey(tr *petri.Transition, dispatch interfaces.WorkDispatch) string {
	if tr != nil && tr.WorkerType != "" {
		return tr.WorkerType
	}
	if dispatch.WorkstationName != "" {
		return dispatch.WorkstationName
	}
	return dispatch.TransitionID
}

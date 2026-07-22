package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type workerPoolDispatchResultHook struct {
	net           *state.Net
	pool          dispatchPool
	executors     map[string]workers.WorkerExecutor
	logger        logging.Logger
	waitCh        chan struct{}
	results       []workerexecution.WorkResult
	deliveryTicks map[string]int
	planner       factory.CompletionDeliveryPlanner
	clock         factory.Clock
	onResult      func()
	mu            sync.Mutex
}

// SetOnBufferedResult installs an owner-local observation hook. It must be
// configured before the dispatch hook starts receiving results.
func (h *workerPoolDispatchResultHook) SetOnBufferedResult(fn func()) {
	h.onResult = fn
}

var _ factory.DispatchResultHook = (*workerPoolDispatchResultHook)(nil)

type replayTickValidator interface {
	ValidateReplayTick(currentTick int) error
}

type plannedCompletionResultProvider interface {
	PlannedResultForDispatch(dispatch work.WorkDispatch) (workerexecution.WorkResult, bool, error)
}

func newWorkerPoolDispatchResultHook(
	net *state.Net,
	pool dispatchPool,
	executors map[string]workers.WorkerExecutor,
	logger logging.Logger,
	buffer int,
	planner factory.CompletionDeliveryPlanner,
	clock factory.Clock,
) *workerPoolDispatchResultHook {
	if clock == nil {
		panic("Factory Runtime dispatch-result clock is required")
	}
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
		clock:         clock,
	}
}

func (h *workerPoolDispatchResultHook) SubmitDispatch(ctx context.Context, dispatch work.WorkDispatch) error {
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
		result := executeDispatchSynchronously(ctx, dispatch, runnerKey, h.executors, h.clock)
		if provider, ok := h.planner.(plannedCompletionResultProvider); ok {
			planned, hasPlanned, err := provider.PlannedResultForDispatch(dispatch)
			if err != nil {
				return err
			}
			if hasPlanned && (result.Outcome != workerexecution.OutcomeFailed ||
				planned.Outcome == workerexecution.OutcomeFailed) {
				result = planned
			}
		}
		h.mu.Lock()
		h.results = append(h.results, result)
		h.signalWaitLocked()
		h.notifyResultLocked()
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

func (h *workerPoolDispatchResultHook) OnTick(_ context.Context, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) ([]workerexecution.WorkResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if validator, ok := h.planner.(replayTickValidator); ok {
		if err := validator.ValidateReplayTick(snapshot.TickCount); err != nil {
			return nil, err
		}
	}
	if len(h.results) == 0 {
		return nil, nil
	}

	results := h.takeDueResults(snapshot.TickCount)
	if len(h.results) > 0 {
		h.signalWaitLocked()
	}
	if len(results) == 0 {
		return nil, nil
	}

	return results, nil
}

func (h *workerPoolDispatchResultHook) takeDueResults(currentTick int) []workerexecution.WorkResult {
	results := make([]workerexecution.WorkResult, 0, len(h.results))
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

func (h *workerPoolDispatchResultHook) HasPendingResults() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.results) > 0
}

func (h *workerPoolDispatchResultHook) HasBufferedResults() bool {
	return h.HasPendingResults()
}

func (h *workerPoolDispatchResultHook) SignalBufferedResults() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.results) > 0 {
		h.signalWaitLocked()
	}
}

func (h *workerPoolDispatchResultHook) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case result := <-h.pool.ResultCh():
				h.mu.Lock()
				h.results = append(h.results, result)
				h.signalWaitLocked()
				h.notifyResultLocked()
				h.mu.Unlock()
			case <-ctx.Done():
				h.logger.Info("factory worker pool dispatch/result hook completed", "reason", ctx.Err())
				return
			}
		}
	}()
}

func (h *workerPoolDispatchResultHook) notifyResultLocked() {
	if h.onResult != nil {
		h.onResult()
	}
}

func (h *workerPoolDispatchResultHook) signalWaitLocked() {
	select {
	case h.waitCh <- struct{}{}:
	default:
	}
}

func executeDispatchSynchronously(
	ctx context.Context,
	dispatch work.WorkDispatch,
	runnerKey string,
	executors map[string]workers.WorkerExecutor,
	clock factory.Clock,
) workerexecution.WorkResult {
	if exec, ok := executors[runnerKey]; ok {
		var (
			result workerexecution.WorkResult
			err    error
		)
		start := clock.Now()
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					result = panicAsFailedResult(dispatch, recovered, clock.Now().Sub(start))
					err = nil
				}
			}()
			result, err = exec.Execute(ctx, dispatch)
		}()
		if err == nil {
			return result
		}
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        err.Error(),
		}
	}
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        fmt.Sprintf("no executor registered for worker type %q (transition %s)", runnerKey, dispatch.TransitionID),
	}
}

func dispatchRunnerKey(tr *petri.Transition, dispatch work.WorkDispatch) string {
	if tr != nil && tr.WorkerType != "" {
		return tr.WorkerType
	}
	if dispatch.WorkstationName != "" {
		return dispatch.WorkstationName
	}
	return dispatch.TransitionID
}

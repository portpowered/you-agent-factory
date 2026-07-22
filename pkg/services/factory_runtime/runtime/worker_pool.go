package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type dispatchPool interface {
	Dispatch(string, work.WorkDispatch) bool
	ResultCh() <-chan workerexecution.WorkResult
	Start()
	Stop()
}

type workerPool struct {
	runners  map[string]*workerRunner
	resultCh chan workerexecution.WorkResult
	logger   logging.Logger
	mu       sync.RWMutex
	clock    factory.Clock
}

func newWorkerPool(logger logging.Logger, clock factory.Clock) *workerPool {
	if clock == nil {
		panic("Factory Runtime worker-pool clock is required")
	}
	return &workerPool{
		runners:  make(map[string]*workerRunner),
		resultCh: make(chan workerexecution.WorkResult, defaultRuntimeBufferSize),
		logger:   logging.EnsureLogger(logger),
		clock:    clock,
	}
}

func (p *workerPool) Register(workerType string, executor workers.WorkerExecutor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.runners[workerType] = newWorkerRunner(workerType, executor, p.resultCh, p.logger, p.clock)
	p.logger.Debug("pool: worker registered", "worker_type", workerType)
}

func (p *workerPool) Dispatch(workerType string, dispatch work.WorkDispatch) bool {
	p.mu.RLock()
	runner, ok := p.runners[workerType]
	p.mu.RUnlock()
	if !ok {
		p.logger.Error("pool: no runner for worker type", "worker_type", workerType)
		return false
	}
	p.logger.Info("pool: dispatch submitted",
		runtimeWorkLogFields(dispatch.Execution,
			"event_name", "worker_pool.submitted",
			"status", "submitted",
			"worker_type", workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID)...)
	runner.dispatchCh <- dispatch
	return true
}

func (p *workerPool) ResultCh() <-chan workerexecution.WorkResult { return p.resultCh }

func (p *workerPool) Start() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, runner := range p.runners {
		runner.Start()
	}
}

func (p *workerPool) Stop() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, runner := range p.runners {
		runner.Stop()
	}
}

type workerRunner struct {
	workerType string
	executor   workers.WorkerExecutor
	logger     logging.Logger
	dispatchCh chan work.WorkDispatch
	resultCh   chan<- workerexecution.WorkResult
	stopOnce   sync.Once
	clock      factory.Clock
}

func newWorkerRunner(
	workerType string,
	executor workers.WorkerExecutor,
	resultCh chan<- workerexecution.WorkResult,
	logger logging.Logger,
	clock factory.Clock,
) *workerRunner {
	return &workerRunner{
		workerType: workerType,
		executor:   executor,
		logger:     logging.EnsureLogger(logger),
		dispatchCh: make(chan work.WorkDispatch, 16),
		resultCh:   resultCh,
		clock:      clock,
	}
}

func (r *workerRunner) Start() { go r.run() }

func (r *workerRunner) Stop() {
	r.stopOnce.Do(func() { close(r.dispatchCh) })
}

func (r *workerRunner) run() {
	var wait sync.WaitGroup
	for dispatch := range r.dispatchCh {
		wait.Add(1)
		go func(d work.WorkDispatch) {
			defer wait.Done()
			result := r.execute(d)
			r.resultCh <- result
			r.logger.Info("runner: response submitted",
				runtimeWorkLogFields(d.Execution,
					"event_name", "worker_pool.response_submitted",
					"status", "response_submitted",
					"worker_type", r.workerType,
					"transition_id", d.TransitionID,
					"dispatch_id", d.DispatchID,
					"outcome", result.Outcome)...)
		}(dispatch)
	}
	wait.Wait()
}

func (r *workerRunner) execute(dispatch work.WorkDispatch) (result workerexecution.WorkResult) {
	start := r.clock.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			result = panicAsFailedResult(dispatch, recovered, r.clock.Now().Sub(start))
			r.logger.Error("runner: execution panic recovered",
				runtimeWorkLogFields(dispatch.Execution,
					"worker_type", r.workerType,
					"transition_id", dispatch.TransitionID,
					"dispatch_id", dispatch.DispatchID,
					"panic", recovered)...)
		}
	}()

	r.logger.Info("runner: execution started",
		runtimeWorkLogFields(dispatch.Execution,
			"event_name", "worker_pool.executor_entered",
			"status", "entered_executor",
			"worker_type", r.workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID)...)
	result, err := r.executor.Execute(context.Background(), dispatch)
	elapsed := r.clock.Now().Sub(start)
	if err != nil {
		r.logger.Error("runner: execution error",
			runtimeWorkLogFields(dispatch.Execution,
				"worker_type", r.workerType,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"error", err)...)
		return workerexecution.WorkResult{
			DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
			Outcome: workerexecution.OutcomeFailed, Error: err.Error(),
			Metrics: workerexecution.WorkMetrics{Duration: elapsed},
		}
	}
	if result.Metrics.Duration == 0 {
		result.Metrics.Duration = elapsed
	}
	r.logger.Info("runner: execution completed",
		runtimeWorkLogFields(dispatch.Execution,
			"worker_type", r.workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"outcome", result.Outcome,
			"elapsed_ms", elapsed.Milliseconds())...)
	return result
}

func panicAsFailedResult(
	dispatch work.WorkDispatch,
	recovered any,
	duration time.Duration,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workerexecution.OutcomeFailed,
		Error:   fmt.Sprintf("executor panic: %v", recovered),
		Metrics: workerexecution.WorkMetrics{Duration: duration},
	}
}

func runtimeWorkLogFields(metadata work.ExecutionMetadata, keysAndValues ...any) []any {
	workIDs := append([]string(nil), metadata.WorkIDs...)
	primaryWorkID := ""
	for _, workID := range workIDs {
		if workID != "" {
			primaryWorkID = workID
			break
		}
	}
	fields := []any{
		"request_id", metadata.RequestID,
		"trace_id", metadata.TraceID,
		"work_id", primaryWorkID,
		"work_ids", workIDs,
	}
	return append(fields, keysAndValues...)
}

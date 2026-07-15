package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
)

// WorkerRunner is an active goroutine that processes work for a specific worker type.
// It reads WorkDispatches from its dispatchCh, calls WorkerExecutor.Execute,
// and sends the WorkResult to the shared resultCh.
type WorkerRunner struct {
	workerType string
	executor   WorkerExecutor
	logger     logging.Logger
	dispatchCh chan work.WorkDispatch
	resultCh   chan<- workerexecution.WorkResult
	stopOnce   sync.Once
}

// NewWorkerRunner creates a runner for the given worker type. The runner reads
// dispatches from its own channel and sends results to the shared resultCh.
func NewWorkerRunner(workerType string, executor WorkerExecutor, resultCh chan<- workerexecution.WorkResult, logger logging.Logger) *WorkerRunner {
	return &WorkerRunner{
		workerType: workerType,
		executor:   executor,
		logger:     logging.EnsureLogger(logger),
		dispatchCh: make(chan work.WorkDispatch, 16),
		resultCh:   resultCh,
	}
}

// Start launches the runner's goroutine. It processes dispatches until the
// dispatch channel is closed.
func (r *WorkerRunner) Start() {
	go r.run()
}

// Stop closes the dispatch channel, signaling the runner goroutine to exit.
func (r *WorkerRunner) Stop() {
	r.stopOnce.Do(func() {
		close(r.dispatchCh)
	})
}

// run is the main goroutine loop. Each dispatch is handled in its own goroutine
// so that multiple work items can execute concurrently — matching the real-world
// pattern of independent parallel agent executions.
func (r *WorkerRunner) run() {
	var wg sync.WaitGroup
	for dispatch := range r.dispatchCh {
		wg.Add(1)
		go func(d work.WorkDispatch) {
			defer wg.Done()
			result := r.executeWithTimeout(d)
			r.resultCh <- result
			r.logger.Info("runner: response submitted",
				WorkLogFields(d.Execution,
					"event_name", WorkLogEventWorkerPoolResponseSubmitted,
					"status", "response_submitted",
					"worker_type", r.workerType,
					"transition_id", d.TransitionID,
					"dispatch_id", d.DispatchID,
					"outcome", result.Outcome)...)
		}(dispatch)
	}
	wg.Wait()
}

// executeWithTimeout executes a single dispatch. Workstation-specific timeout
// handling is resolved inside WorkstationExecutor from runtime config.
func (r *WorkerRunner) executeWithTimeout(dispatch work.WorkDispatch) (result workerexecution.WorkResult) {
	ctx := context.Background()
	start := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			result = PanicAsFailedResult(dispatch, recovered, time.Since(start))
			r.logger.Error("runner: execution panic recovered",
				WorkLogFields(dispatch.Execution,
					"worker_type", r.workerType,
					"transition_id", dispatch.TransitionID,
					"dispatch_id", dispatch.DispatchID,
					"panic", recovered)...)
		}
	}()

	r.logger.Info("runner: execution started",
		WorkLogFields(dispatch.Execution,
			"event_name", WorkLogEventWorkerPoolExecutorEntered,
			"status", "entered_executor",
			"worker_type", r.workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID)...)

	result, err := r.executor.Execute(ctx, dispatch)
	elapsed := time.Since(start)

	if err != nil {
		// Context deadline exceeded means execution timeout.
		if ctx.Err() == context.DeadlineExceeded {
			r.logger.Info("runner: execution timeout",
				WorkLogFields(dispatch.Execution,
					"worker_type", r.workerType,
					"transition_id", dispatch.TransitionID,
					"dispatch_id", dispatch.DispatchID,
					"elapsed_ms", elapsed.Milliseconds())...)
			return workerexecution.WorkResult{
				DispatchID:   dispatch.DispatchID,
				TransitionID: dispatch.TransitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        "execution timeout",
				Metrics:      workerexecution.WorkMetrics{Duration: elapsed},
			}
		}
		// Other executor errors are system failures.
		r.logger.Error("runner: execution error",
			WorkLogFields(dispatch.Execution,
				"worker_type", r.workerType,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"error", err)...)
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        err.Error(),
			Metrics:      workerexecution.WorkMetrics{Duration: elapsed},
		}
	}

	// Ensure metrics duration is set even if the executor didn't set it.
	if result.Metrics.Duration == 0 {
		result.Metrics.Duration = elapsed
	}

	r.logger.Info("runner: execution completed",
		WorkLogFields(dispatch.Execution,
			"worker_type", r.workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"outcome", result.Outcome,
			"elapsed_ms", elapsed.Milliseconds())...)

	return result
}

func PanicAsFailedResult(dispatch work.WorkDispatch, recovered any, duration time.Duration) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        fmt.Sprintf("executor panic: %v", recovered),
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

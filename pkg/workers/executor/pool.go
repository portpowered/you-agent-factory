package executor

import (
	"sync"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
)

// WorkerPool manages WorkerRunners by worker type, providing a shared resultCh
// that the engine selects on to wake when workers complete.
type WorkerPool struct {
	runners  map[string]*WorkerRunner        // worker type ID → runner
	resultCh chan workerexecution.WorkResult // shared result channel — engine selects on this
	logger   logging.Logger
	mu       sync.RWMutex
}

// NewWorkerPool creates a WorkerPool with a shared result channel.
func NewWorkerPool(logger logging.Logger) *WorkerPool {
	return &WorkerPool{
		runners:  make(map[string]*WorkerRunner),
		resultCh: make(chan workerexecution.WorkResult, 64),
		logger:   logging.EnsureLogger(logger),
	}
}

// Register adds a WorkerRunner for the given worker type. If a runner already
// exists for this type, it is replaced (the old runner is not stopped).
func (p *WorkerPool) Register(workerType string, executor WorkerExecutor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.runners[workerType] = NewWorkerRunner(workerType, executor, p.resultCh, p.logger)
	p.logger.Debug("pool: worker registered", "worker_type", workerType)
}

// Dispatch sends a WorkDispatch to the appropriate runner's dispatch channel.
// Returns false if no runner is registered for the dispatch's worker type.
func (p *WorkerPool) Dispatch(workerType string, dispatch work.WorkDispatch) bool {
	p.mu.RLock()
	runner, ok := p.runners[workerType]
	p.mu.RUnlock()
	if !ok {
		p.logger.Error("pool: no runner for worker type", "worker_type", workerType)
		return false
	}
	p.logger.Info("pool: dispatch submitted",
		WorkLogFields(dispatch.Execution,
			"event_name", WorkLogEventWorkerPoolSubmitted,
			"status", "submitted",
			"worker_type", workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID)...)
	runner.dispatchCh <- dispatch
	return true
}

// ResultCh returns the shared result channel that the engine should select on.
func (p *WorkerPool) ResultCh() <-chan workerexecution.WorkResult {
	return p.resultCh
}

// Start launches all registered runners. Each runner starts its goroutine
// to process dispatches.
func (p *WorkerPool) Start() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, runner := range p.runners {
		runner.Start()
	}
}

// Stop signals all runners to shut down by closing their dispatch channels.
func (p *WorkerPool) Stop() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, runner := range p.runners {
		runner.Stop()
	}
}

package service

import (
	"errors"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type cleanupFunc func() error

type cleanupRegistry struct {
	mu    sync.Mutex
	once  sync.Once
	hooks []cleanupFunc
	err   error
}

func newCleanupRegistry() *cleanupRegistry {
	return &cleanupRegistry{}
}

func (registry *cleanupRegistry) add(hook cleanupFunc) {
	if registry == nil || hook == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.hooks = append(registry.hooks, hook)
}

func (registry *cleanupRegistry) run(logger logging.Logger, correlation workers.ExecutionCorrelation) error {
	if registry == nil {
		return nil
	}
	registry.once.Do(func() {
		registry.mu.Lock()
		hooks := append([]cleanupFunc(nil), registry.hooks...)
		registry.hooks = nil
		registry.mu.Unlock()
		for index := len(hooks) - 1; index >= 0; index-- {
			if category, err := runCleanupHook(hooks[index]); err != nil {
				registry.err = errors.Join(registry.err, err)
				logger.Warn("workers execute cleanup failed", append(executionLogFields(correlation), "error", category)...)
			}
		}
	})
	return registry.err
}

func runCleanupHook(hook cleanupFunc) (category string, err error) {
	if hook == nil {
		return "", nil
	}
	category = "cleanup_error"
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("workers execute cleanup panicked")
			category = "cleanup_panic"
		}
	}()
	return category, hook()
}

// executionLogFields contains only the existing attempt attribution.
func executionLogFields(correlation workers.ExecutionCorrelation) []any {
	return []any{
		"factory_session_id", correlation.FactorySessionID,
		"runtime_id", correlation.RuntimeID,
		"generation_id", correlation.GenerationID,
		"dispatch_id", correlation.DispatchID,
		"attempt_id", correlation.AttemptID,
		"request_id", correlation.RequestID,
		"trace_id", correlation.TraceID,
	}
}

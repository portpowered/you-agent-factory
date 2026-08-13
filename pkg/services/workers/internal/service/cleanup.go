package service

import (
	"errors"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
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

func (registry *cleanupRegistry) run(logger logging.Logger) error {
	if registry == nil {
		return nil
	}
	registry.once.Do(func() {
		registry.mu.Lock()
		hooks := append([]cleanupFunc(nil), registry.hooks...)
		registry.hooks = nil
		registry.mu.Unlock()
		for index := len(hooks) - 1; index >= 0; index-- {
			if err := runCleanupHook(hooks[index]); err != nil {
				registry.err = errors.Join(registry.err, err)
				if logger != nil {
					logger.Warn("workers execute cleanup failed", "error", err.Error())
				}
			}
		}
	})
	return registry.err
}

func runCleanupHook(hook cleanupFunc) (err error) {
	if hook == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("workers execute cleanup panicked")
		}
	}()
	return hook()
}

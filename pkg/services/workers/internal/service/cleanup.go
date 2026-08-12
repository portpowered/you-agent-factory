package service

import (
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

type cleanupFunc func() error

type cleanupRegistry struct {
	mu    sync.Mutex
	hooks []cleanupFunc
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

func (registry *cleanupRegistry) run(logger logging.Logger) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	hooks := append([]cleanupFunc(nil), registry.hooks...)
	registry.hooks = nil
	registry.mu.Unlock()
	for index := len(hooks) - 1; index >= 0; index-- {
		if err := hooks[index](); err != nil && logger != nil {
			logger.Warn("workers execute cleanup failed", "error", err.Error())
		}
	}
}

package runtimeopening

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// modelsRuntimeBind carries the process-scoped Models root and the opaque
// runtime scope opened for one Factory Session selection.
type modelsRuntimeBind struct {
	Root  models.Service
	Scope models.RuntimeScopeRef
}

// runtimeOpeningCleanup owns resources in acquisition order and releases them
// exactly once in reverse order on either opening failure or runtime
// shutdown. Add is safe to call concurrently with Close: a later Factory
// Session build (for example a named-factory activation racing session
// shutdown) can register a cleanup action after opening has already
// returned, so both the action slice and the once-guarded release read it
// under the same mutex.
type runtimeOpeningCleanup struct {
	mu      sync.Mutex
	actions []func() error
	once    sync.Once
	err     error
}

func (cleanup *runtimeOpeningCleanup) Add(action func() error) {
	if action == nil {
		return
	}
	cleanup.mu.Lock()
	cleanup.actions = append(cleanup.actions, action)
	cleanup.mu.Unlock()
}

func (cleanup *runtimeOpeningCleanup) OwnModelsScope(
	ctx context.Context,
	bind modelsRuntimeBind,
) {
	cleanup.Add(func() error {
		closed, err := bind.Root.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{
			Scope: bind.Scope,
		})
		if err != nil {
			return fmt.Errorf("close Models runtime scope: %w", err)
		}
		if !closed.Closed || closed.Scope != bind.Scope {
			return fmt.Errorf("close Models runtime scope: Models service did not confirm the issued scope")
		}
		return nil
	})
}

func (cleanup *runtimeOpeningCleanup) Close() error {
	cleanup.once.Do(func() {
		cleanup.mu.Lock()
		actions := append([]func() error(nil), cleanup.actions...)
		cleanup.actions = nil
		cleanup.mu.Unlock()
		for index := len(actions) - 1; index >= 0; index-- {
			cleanup.err = errors.Join(cleanup.err, actions[index]())
		}
	})
	return cleanup.err
}

func (cleanup *runtimeOpeningCleanup) Unwind(cause error) error {
	return errors.Join(cause, cleanup.Close())
}

// sessionBuildRuntimeSink accumulates every independently constructed
// session-build Workers runtime for one Factory Session, so they close
// together at session shutdown even though each is built at its own later
// time (for example a named-factory activation, long after the Factory
// Session's own opening returned). Add and Close are safe to call
// concurrently: a later Factory Session build can race session shutdown.
// Once closed, Add reports false and registers nothing -- the caller then
// owns closing the runtime it just built instead of leaking it.
type sessionBuildRuntimeSink struct {
	mu      sync.Mutex
	closed  bool
	runtime []workers.RuntimeService
}

func (sink *sessionBuildRuntimeSink) Add(runtime workers.RuntimeService) bool {
	if sink == nil || runtime == nil {
		return true
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.closed {
		return false
	}
	sink.runtime = append(sink.runtime, runtime)
	return true
}

func (sink *sessionBuildRuntimeSink) Close(ctx context.Context) error {
	if sink == nil {
		return nil
	}
	sink.mu.Lock()
	if sink.closed {
		sink.mu.Unlock()
		return nil
	}
	sink.closed = true
	runtimes := append([]workers.RuntimeService(nil), sink.runtime...)
	sink.runtime = nil
	sink.mu.Unlock()
	var result error
	for index := len(runtimes) - 1; index >= 0; index-- {
		result = errors.Join(result, runtimes[index].Close(ctx))
	}
	return result
}

func bindModelsRuntimeScope(
	ctx context.Context,
	modelService models.Service,
	cacheDirectory string,
	runtimeConfigLoader models.RuntimeConfigLoader,
) (modelsRuntimeBind, error) {
	if modelService == nil {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models service is required")
	}
	if runtimeConfigLoader == nil {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models runtime configuration lookup is required")
	}
	scopeConfig := models.RuntimeScopeConfig{
		CacheDirectory: cacheDirectory,
	}
	if runtimeConfig := runtimeConfigLoader(); runtimeConfig != nil {
		scopeConfig.Runtime = *runtimeConfig
	}
	opened, err := modelService.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: scopeConfig,
	})
	if err != nil {
		return modelsRuntimeBind{}, err
	}
	if opened.Scope.IsZero() {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models service returned zero runtime scope")
	}
	return modelsRuntimeBind{
		Root:  modelService,
		Scope: opened.Scope,
	}, nil
}

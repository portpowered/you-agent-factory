package runtimeopening

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// modelsRuntimeBind carries the process-scoped Models root and the opaque
// runtime scope opened for one Factory Session selection.
type modelsRuntimeBind struct {
	Root  models.Service
	Scope models.RuntimeScopeRef
}

// runtimeOpeningCleanup owns resources in acquisition order and releases them
// exactly once in reverse order on either opening failure or runtime shutdown.
type runtimeOpeningCleanup struct {
	actions []func() error
	once    sync.Once
	err     error
}

func (cleanup *runtimeOpeningCleanup) Add(action func() error) {
	if action != nil {
		cleanup.actions = append(cleanup.actions, action)
	}
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
		for index := len(cleanup.actions) - 1; index >= 0; index-- {
			cleanup.err = errors.Join(cleanup.err, cleanup.actions[index]())
		}
	})
	return cleanup.err
}

func (cleanup *runtimeOpeningCleanup) Unwind(cause error) error {
	return errors.Join(cause, cleanup.Close())
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

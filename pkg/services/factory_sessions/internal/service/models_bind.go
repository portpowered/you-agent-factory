package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// modelsRuntimeBind carries the process-scoped Models root and the opaque
// runtime scope opened for one Factory Session selection.
type modelsRuntimeBind struct {
	Root  models.Service
	Scope models.RuntimeScopeRef
}

// runtimeOpeningCleanup releases owned resources on opening failure or runtime
// shutdown, with Models retained until its consumers close. Successful releases
// are removed; failed releases remain retryable. Close calls are serialized,
// while Add can register newly acquired ownership during a release.
type runtimeOpeningCleanup struct {
	mu      sync.Mutex
	actions []func() error
	closeMu sync.Mutex
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
	if bind.Root == nil || bind.Scope.IsZero() {
		return
	}
	closeScope := func() error {
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
	}
	// Durable execution may still consume Models while it closes. Retain the
	// existing lifetime order: release this scope after every other owned
	// resource, even when durable execution was registered before Models opened.
	cleanup.mu.Lock()
	cleanup.actions = append([]func() error{closeScope}, cleanup.actions...)
	cleanup.mu.Unlock()
}

// OwnRuntimeRecord registers partial opening ownership before validating the
// session result. Recording finalization precedes artifact release on each
// attempt; failed releases remain owned by this cleanup.
func (cleanup *runtimeOpeningCleanup) OwnRuntimeRecord(record factoryruntime.RuntimeRecord, clock factoryruntime.Clock) {
	if record == nil {
		return
	}
	finalized, artifactsClosed := false, false
	cleanup.Add(func() error {
		var finalizationErr, artifactsErr error
		if !finalized {
			if finalizer, ok := record.(interface{ FinalizeRecording(time.Time) error }); ok {
				finalizationErr = finalizer.FinalizeRecording(clock.Now().UTC())
			}
			finalized = finalizationErr == nil
		}
		if !artifactsClosed {
			artifactsErr = record.CloseArtifacts()
			artifactsClosed = artifactsErr == nil
		}
		return errors.Join(finalizationErr, artifactsErr)
	})
}

func (cleanup *runtimeOpeningCleanup) Close() error {
	cleanup.closeMu.Lock()
	defer cleanup.closeMu.Unlock()
	cleanup.mu.Lock()
	actions := cleanup.actions
	cleanup.actions = nil
	cleanup.mu.Unlock()
	var closeErr error
	for index := len(actions) - 1; index >= 0; index-- {
		if err := actions[index](); err != nil {
			closeErr = errors.Join(closeErr, err)
		} else {
			actions[index] = nil
		}
	}
	var pending []func() error
	for _, action := range actions {
		if action != nil {
			pending = append(pending, action)
		}
	}
	cleanup.mu.Lock()
	cleanup.actions = append(pending, cleanup.actions...)
	cleanup.mu.Unlock()
	return closeErr
}

func bindModelsRuntimeScope(
	ctx context.Context,
	modelService models.Service,
	cacheDirectory string,
	runtimeConfigLoader models.RuntimeConfigLoader,
	operatorModels map[string]models.ModelOverlay,
) (modelsRuntimeBind, error) {
	if modelService == nil {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models service is required")
	}
	if runtimeConfigLoader == nil {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models runtime configuration lookup is required")
	}
	scopeConfig := models.RuntimeScopeConfig{
		CacheDirectory: cacheDirectory,
		OperatorModels: cloneOperatorModelOverlays(operatorModels),
	}
	if runtimeConfig := runtimeConfigLoader(); runtimeConfig != nil {
		scopeConfig.Runtime = *runtimeConfig
	}
	opened, err := modelService.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: scopeConfig,
	})
	bind := modelsRuntimeBind{Root: modelService, Scope: opened.Scope}
	if err != nil {
		// Preserve a partial scope so the opening owner can release it before
		// returning the failure, or retain cleanup when release also fails.
		return bind, err
	}
	if opened.Scope.IsZero() {
		return modelsRuntimeBind{}, fmt.Errorf("construct runtime scope: Models service returned zero runtime scope")
	}
	return bind, nil
}

func cloneOperatorModelOverlays(
	overlays map[string]models.ModelOverlay,
) map[string]models.ModelOverlay {
	if len(overlays) == 0 {
		return nil
	}
	cloned := make(map[string]models.ModelOverlay, len(overlays))
	for name, overlay := range overlays {
		cloned[name] = overlay.Clone()
	}
	return cloned
}

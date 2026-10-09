package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	models  []func() error
	targets []func() error
	closeMu sync.Mutex
}

var errRuntimeOpeningCleanupPending = errors.New("runtime opening cleanup still owns pending resources")

// OwnRecordingTarget retains destination ownership until every runtime and
// writer consumer has stopped, including cleanup retries after a failure.
func (cleanup *runtimeOpeningCleanup) OwnRecordingTarget(lease io.Closer) {
	if lease == nil {
		return
	}
	cleanup.mu.Lock()
	cleanup.targets = append(cleanup.targets, lease.Close)
	cleanup.mu.Unlock()
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
	cleanup.models = append(cleanup.models, closeScope)
	cleanup.mu.Unlock()
}

func (cleanup *runtimeOpeningCleanup) Close() error {
	cleanup.closeMu.Lock()
	defer cleanup.closeMu.Unlock()
	cleanup.mu.Lock()
	actions := cleanup.actions
	models := cleanup.models
	targets := cleanup.targets
	cleanup.actions = nil
	cleanup.models = nil
	cleanup.targets = nil
	cleanup.mu.Unlock()
	pending, closeErr := cleanup.releaseActions(actions)
	cleanup.mu.Lock()
	consumersAdded := len(cleanup.actions) != 0
	cleanup.mu.Unlock()
	// A failed consumer still owns its dependency. Release independent resources
	// now, but keep Models available until every consumer has closed successfully.
	if len(pending) == 0 && !consumersAdded {
		var err error
		models, err = cleanup.releaseModels(models)
		closeErr = errors.Join(closeErr, err)
	}
	if len(pending) == 0 && len(models) == 0 && !consumersAdded {
		var err error
		targets, err = cleanup.releaseModels(targets)
		closeErr = errors.Join(closeErr, err)
	}
	cleanup.mu.Lock()
	cleanup.actions = append(pending, cleanup.actions...)
	cleanup.models = append(models, cleanup.models...)
	cleanup.targets = append(targets, cleanup.targets...)
	// Opening callers retain the retry capability only when Close reports an
	// incomplete release. Ownership registered by a closer must not turn into
	// a successful release merely because the original batch completed.
	if closeErr == nil && (len(cleanup.actions) != 0 || len(cleanup.models) != 0 || len(cleanup.targets) != 0) {
		closeErr = errRuntimeOpeningCleanupPending
	}
	complete := len(cleanup.actions) == 0 && len(cleanup.models) == 0 && len(cleanup.targets) == 0
	cleanup.mu.Unlock()
	if closeErr != nil {
		return &recordings.RecordingCleanupError{Cause: closeErr, Complete: complete}
	}
	return closeErr
}

// A dependency release can itself register a consumer. Recheck between Models
// releases so the remaining dependencies stay available until that consumer
// closes on an explicit retry.
func (cleanup *runtimeOpeningCleanup) releaseModels(actions []func() error) ([]func() error, error) {
	var pending []func() error
	var closeErr error
	for index := len(actions) - 1; index >= 0; index-- {
		cleanup.mu.Lock()
		consumersAdded := len(cleanup.actions) != 0
		cleanup.mu.Unlock()
		if consumersAdded {
			return append(actions[:index+1], pending...), closeErr
		}
		if err := actions[index](); err != nil {
			closeErr = errors.Join(closeErr, err)
			pending = append([]func() error{actions[index]}, pending...)
		}
	}
	return pending, closeErr
}

func (*runtimeOpeningCleanup) releaseActions(actions []func() error) ([]func() error, error) {
	var closeErr error
	for index := len(actions) - 1; index >= 0; index-- {
		if err := actions[index](); err != nil {
			closeErr = errors.Join(closeErr, err)
			var terminal *recordings.RecordingCleanupError
			if errors.As(err, &terminal) && terminal.Complete {
				actions[index] = nil
			}
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
	return pending, closeErr
}

func (acquisition *RuntimeResourceAcquisition) bindModelsRuntimeScope(
	ctx context.Context,
	cacheDirectory string,
	runtimeConfig *models.RuntimeConfig,
	operatorModels map[string]models.ModelOverlay,
) (modelsRuntimeBind, error) {
	scopeConfig := models.RuntimeScopeConfig{
		CacheDirectory: cacheDirectory,
		OperatorModels: cloneOperatorModelOverlays(operatorModels),
	}
	if runtimeConfig != nil {
		scopeConfig.Runtime = *runtimeConfig
	}
	opened, err := acquisition.modelService.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: scopeConfig.Clone(),
	})
	bind := modelsRuntimeBind{Root: acquisition.modelService, Scope: opened.Scope}
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

// Factory Runtime state adapters remain session-owned.
package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"go.uber.org/zap"
)

func (fs *SessionRuntime) syncActiveSessionDir(runtimeBundle factoryRuntimeBundle) {
	if fs == nil {
		return
	}
	runtimebinding.SyncActiveDirectory(&fs.runtimeMu, &fs.dir, fs.factoryRootDir, runtimeBundle)
}

// SubmitWorkRequest submits a canonical work request batch to the factory.
func (fs *SessionRuntime) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if fs == nil || fs.sessionState == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("factory session service is required")
	}
	var result work.WorkRequestSubmitResult
	err := fs.sessionState.WithRuntimeRead(func(runtime *factorysessions.LiveRuntime) error {
		var submitErr error
		result, submitErr = runtime.Factory.SubmitWorkRequest(ctx, request)
		return submitErr
	})
	return result, err
}

// SubscribeFactoryEvents returns canonical factory event history followed by
// live events from the current service-owned runtime.
func (fs *SessionRuntime) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return nil, fmt.Errorf("factory runtime is not available")
	}
	return runtime.SubscribeFactoryEvents(ctx, reconnect, scope)
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight. Delegates to
// the underlying factory's termination signal.
func (fs *SessionRuntime) WaitToComplete() <-chan struct{} {
	if runtime := fs.currentRuntimeService(); runtime != nil {
		return runtime.WaitToComplete()
	}
	done := make(chan struct{})
	close(done)
	return done
}

// GetEngineStateSnapshot returns the factory boundary's aggregate
// observability snapshot.
func (fs *SessionRuntime) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet], error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return nil, fmt.Errorf("factory runtime is not available")
	}
	return runtime.GetEngineStateSnapshot(ctx)
}

// Pause pauses the current runtime instance.
func (fs *SessionRuntime) Pause(ctx context.Context) error {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return fmt.Errorf("factory runtime is not available")
	}
	if err := runtime.Pause(ctx); err != nil {
		return fmt.Errorf("pause factory: %w", err)
	}
	return nil
}

// Resume resumes the current runtime instance and wakes buffered work.
func (fs *SessionRuntime) Resume(ctx context.Context) error {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return fmt.Errorf("factory runtime is not available")
	}
	if err := runtime.Resume(ctx); err != nil {
		return fmt.Errorf("resume factory: %w", err)
	}
	return nil
}

// Terminate requests cooperative stop of the current runtime instance through
// the published Factory Runtime root control contract.
func (fs *SessionRuntime) Terminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.TerminateResult{}, fmt.Errorf("factory runtime is not available")
	}
	result, err := runtime.Terminate(ctx, req)
	if err != nil {
		return factory.TerminateResult{}, fmt.Errorf("terminate factory: %w", err)
	}
	return result, nil
}

// GetFactoryEvents returns the canonical factory event history.
func (fs *SessionRuntime) GetFactoryEvents(ctx context.Context) ([]interfaces.FactoryEvent, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return nil, fmt.Errorf("factory runtime is not available")
	}
	return runtime.GetFactoryEvents(ctx)
}

func (fs *SessionRuntime) submitWorkFile(ctx context.Context) error {
	workFile := fs.workFile
	if fs.initialWorkFiles == nil {
		return fmt.Errorf("Factory Session initial Work file reader is required")
	}
	data, err := fs.initialWorkFiles.ReadFile(workFile)
	if err != nil {
		return fmt.Errorf("read work file %s: %w", workFile, err)
	}
	request, err := work.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return fmt.Errorf("parse work file %s: %w", workFile, err)
	}
	target := fs.currentRuntimeService()
	if target == nil {
		return fmt.Errorf("factory runtime is not available")
	}
	if _, err := target.SubmitWorkRequest(ctx, request); err != nil {
		return fmt.Errorf("submit initial work: %w", err)
	}
	fs.logger.Info("submitted initial work", zap.String("file", workFile))
	return nil
}

func (fs *SessionRuntime) currentRuntimeConfig() interfaces.LoadedFactorySource {
	if bundle := fs.currentRuntimeBundle(); bundle != nil {
		loaded, _ := bundle.LoadedRuntimeConfig().(interfaces.LoadedFactorySource)
		return loaded
	}
	if fs == nil || fs.sessionState == nil {
		return nil
	}
	spec, _ := runtimebinding.PreparedSpecFromSession(fs.sessionState.Default()).(*factory.SessionBuildSpec)
	if spec == nil {
		return nil
	}
	loaded, _ := spec.LoadedFactoryCfg.(interfaces.LoadedFactorySource)
	return loaded
}

// CurrentRuntimeConfig returns the loaded definition/configuration for the
// currently selected runtime without exposing its host bundle.
func (fs *SessionRuntime) CurrentRuntimeConfig() interfaces.LoadedFactorySource {
	return fs.currentRuntimeConfig()
}

func (fs *SessionRuntime) currentRuntimeService() factory.Service {
	if instance := fs.currentRuntimeBundle(); instance != nil {
		return instance.RuntimeService()
	}
	return nil
}

// StartupWorkerConfig returns the named worker from the built startup runtime config.
func (fs *SessionRuntime) StartupWorkerConfig(name string) (*interfaces.FactoryWorkerConfig, bool) {
	runtimeCfg := fs.currentRuntimeConfig()
	if runtimeCfg == nil {
		return nil, false
	}
	return runtimeCfg.Worker(name)
}

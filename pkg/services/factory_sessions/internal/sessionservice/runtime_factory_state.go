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
		legacyRuntime, ok := runtime.Factory.(factory.APIFactory)
		if !ok {
			return fmt.Errorf("legacy Factory Runtime submission is required")
		}
		result, submitErr = legacyRuntime.SubmitWorkRequest(ctx, request)
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
	legacyRuntime, ok := runtime.(factory.APIFactory)
	if !ok {
		return nil, fmt.Errorf("legacy Factory Runtime event subscription is required")
	}
	return legacyRuntime.SubscribeFactoryEvents(ctx, reconnect, scope)
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight. Delegates to
// the underlying factory's termination signal.
func (fs *SessionRuntime) WaitToComplete() <-chan struct{} {
	if runtime := fs.currentRuntimeService(); runtime != nil {
		return runtime.ControlWaitToComplete(factory.WaitToCompleteRequest{}).Done
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
	legacyObservation, err := runtimebinding.LegacyObservationForService(runtime)
	if err != nil {
		return nil, err
	}
	return legacyObservation.GetEngineStateSnapshot(ctx)
}

// Pause pauses the current runtime instance.
func (fs *SessionRuntime) Pause(ctx context.Context) error {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return fmt.Errorf("factory runtime is not available")
	}
	if _, err := runtime.ControlPause(ctx, factory.PauseRequest{}); err != nil {
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
	if _, err := runtime.ControlResume(ctx, factory.ResumeRequest{}); err != nil {
		return fmt.Errorf("resume factory: %w", err)
	}
	return nil
}

// ControlPause routes root control to the current replaceable runtime.
func (fs *SessionRuntime) ControlPause(ctx context.Context, req factory.PauseRequest) (factory.PauseResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.PauseResult{}, factory.ErrNotFound
	}
	return runtime.ControlPause(ctx, req)
}

// ControlResume routes root control to the current replaceable runtime.
func (fs *SessionRuntime) ControlResume(ctx context.Context, req factory.ResumeRequest) (factory.ResumeResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.ResumeResult{}, factory.ErrNotFound
	}
	return runtime.ControlResume(ctx, req)
}

// ControlTerminate routes root control to the current replaceable runtime.
func (fs *SessionRuntime) ControlTerminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.TerminateResult{}, factory.ErrNotFound
	}
	return runtime.ControlTerminate(ctx, req)
}

// ControlWaitToComplete returns the current runtime's completion signal.
func (fs *SessionRuntime) ControlWaitToComplete(req factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	runtime := fs.currentRuntimeService()
	if runtime != nil {
		return runtime.ControlWaitToComplete(req)
	}
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}

// ControlMoveWork routes root work relocation to the current replaceable runtime.
func (fs *SessionRuntime) ControlMoveWork(ctx context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.MoveWorkResult{}, factory.ErrNotFound
	}
	return runtime.ControlMoveWork(ctx, req)
}

// Observe routes root observation to the current replaceable runtime.
func (fs *SessionRuntime) Observe(ctx context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.ObserveResult{}, factory.ErrNotFound
	}
	return runtime.Observe(ctx, req)
}

// PlanDispatch routes root dispatch planning to the current replaceable runtime.
func (fs *SessionRuntime) PlanDispatch(ctx context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.PlanDispatchResult{}, factory.ErrNotFound
	}
	return runtime.PlanDispatch(ctx, req)
}

// AcceptDispatchResult routes correlated worker results to the current runtime.
func (fs *SessionRuntime) AcceptDispatchResult(ctx context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.AcceptDispatchResultResult{}, factory.ErrNotFound
	}
	return runtime.AcceptDispatchResult(ctx, req)
}

// CaptureCheckpoint routes checkpoint capture to the current replaceable runtime.
func (fs *SessionRuntime) CaptureCheckpoint(ctx context.Context, req factory.CaptureCheckpointRequest) (factory.CaptureCheckpointResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.CaptureCheckpointResult{}, factory.ErrNotFound
	}
	return runtime.CaptureCheckpoint(ctx, req)
}

// LoadCheckpoint routes checkpoint loading to the current replaceable runtime.
func (fs *SessionRuntime) LoadCheckpoint(ctx context.Context, req factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.LoadCheckpointResult{}, factory.ErrNotFound
	}
	return runtime.LoadCheckpoint(ctx, req)
}

// RestoreCheckpoint routes checkpoint restoration to the current replaceable runtime.
func (fs *SessionRuntime) RestoreCheckpoint(ctx context.Context, req factory.RestoreCheckpointRequest) (factory.RestoreCheckpointResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.RestoreCheckpointResult{}, factory.ErrNotFound
	}
	return runtime.RestoreCheckpoint(ctx, req)
}

// GetFactoryEvents returns the canonical factory event history.
func (fs *SessionRuntime) GetFactoryEvents(ctx context.Context) ([]interfaces.FactoryEvent, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return nil, fmt.Errorf("factory runtime is not available")
	}
	eventSource, ok := runtime.(interface {
		GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error)
	})
	if !ok {
		return nil, fmt.Errorf("legacy Factory Runtime event history is required")
	}
	return eventSource.GetFactoryEvents(ctx)
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
	legacyRuntime, ok := target.(factory.APIFactory)
	if !ok {
		return fmt.Errorf("legacy Factory Runtime submission is required")
	}
	if _, err := legacyRuntime.SubmitWorkRequest(ctx, request); err != nil {
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

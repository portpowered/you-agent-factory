// Runtime lifecycle operations expose domain actions to initializer.
package service

import (
	"context"
	"errors"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimebinding"
)

type RuntimeStop = factorysessions.RuntimeStop

// StartLifecycle starts the runtime phase selected by the Factory
// Sessions-owned process lifecycle plan. Initializer only executes that
// already-declared neutral plan.
func (runtime *SessionRuntime) StartLifecycle(ctx, runCtx context.Context) error {
	if runtime == nil {
		return errors.New("start runtime: Factory Session runtime is required")
	}
	runtime.startTime = runtime.clock.Now()
	serviceMode := factory.RuntimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService
	if !serviceMode {
		bundle := runtime.currentRuntimeBundle()
		if err := runtime.PreseedRuntimeInputs(ctx, bundle); err != nil {
			return err
		}
		if runtime.workFile != "" {
			if err := runtime.submitWorkFile(ctx); err != nil {
				return err
			}
		}
	}
	// Initializer owns sidecar activation as the next lifecycle phase.
	_, err := runtime.StartDefaultRuntime(ctx, runCtx, false)
	return err
}

// StartWorkerLifecycle activates the runtime's worker-side automation.
func (runtime *SessionRuntime) StartWorkerLifecycle(ctx context.Context) (RuntimeStop, error) {
	if runtime == nil {
		return nil, errors.New("start runtime automation: Factory Session runtime is required")
	}
	current := runtime.runtimeState.ActiveHandle()
	if current == nil || current.RuntimeInstance() == nil {
		return nil, errors.New("start runtime automation: runtime is not started")
	}
	serviceMode := factory.RuntimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService
	if serviceMode {
		if err := runtime.PreseedRuntimeInputs(ctx, current.RuntimeInstance()); err != nil {
			return nil, err
		}
	}
	if err := runtime.StartLiveRuntimeSidecars(ctx, current); err != nil {
		return nil, err
	}
	return func(context.Context) error {
		runtime.StopLiveRuntimeSidecars(current)
		return nil
	}, nil
}

// CompleteStartup submits service-mode startup work after the process
// transport is readable.
func (runtime *SessionRuntime) CompleteStartup(ctx context.Context) error {
	if runtime == nil {
		return errors.New("complete runtime startup: Factory Session runtime is required")
	}
	current := runtime.runtimeState.ActiveHandle()
	serviceMode := factory.RuntimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService
	if serviceMode && runtime.workFile != "" {
		if err := runtime.submitWorkFile(ctx); err != nil {
			return runtimebinding.FailStartup(
				runtime.sessionState, &runtime.runtimeState, DefaultFactorySessionID,
				current, runtime.StopLiveRuntime, err,
			)
		}
	}
	return nil
}

// WaitForRuntime waits for the currently active runtime, following a
// replacement handle when a session swap occurs.
func (runtime *SessionRuntime) WaitForRuntime(ctx context.Context) error {
	if runtime == nil {
		return errors.New("wait for runtime: Factory Session runtime is required")
	}
	for {
		current := runtime.runtimeState.ActiveHandle()
		if current == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-current.RunDoneCh():
		}
		if runtime.runtimeState.ActiveHandle() != current {
			continue
		}
		if factory.RuntimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService && runtime.sessionState.Registry() != nil && runtime.sessionState.Registry().Count() == 0 {
			continue
		}
		return current.Result()
	}
}

// StopLifecycle stops the active runtime and any other live session runtimes.
func (runtime *SessionRuntime) StopLifecycle(_ context.Context) error {
	if runtime == nil {
		return nil
	}
	current := runtime.runtimeState.ActiveHandle()
	var result error
	if err := runtime.StopLiveRuntime(current); err != nil && !errors.Is(err, context.Canceled) {
		result = err
	}
	if err := runtime.ShutdownOtherLiveSessions(current); err != nil {
		result = errors.Join(result, err)
	}
	runtime.runtimeState.ClearActive()
	return result
}

// FailStartup records a process-startup failure on the default Factory Session.
func (runtime *SessionRuntime) FailStartup(err error) error {
	if runtime == nil {
		return err
	}
	current := runtime.runtimeState.ActiveHandle()
	return runtimebinding.FailStartup(
		runtime.sessionState, &runtime.runtimeState, DefaultFactorySessionID,
		current, runtime.StopLiveRuntime, err,
	)
}

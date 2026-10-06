// Runtime lifecycle operations expose domain actions to initializer.
package service

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/initializer"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

type RuntimeStop = factorysessions.RuntimeStop

// Close drains every live Factory Session owned by this process root. A
// command stops only its admitted session; process shutdown owns the rest.
func (a *Assembly) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	var result error
	if a.registry != nil {
		ids := a.registry.IDs()
		for index := len(ids) - 1; index >= 0; index-- {
			if err := a.CloseSession(ctx, ids[index]); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, factorysessions.ErrSessionNotFound) {
				result = errors.Join(result, err)
			}
		}
	}
	if service, ok := a.SessionGateway.(*Service); ok && service.durable != nil {
		if closer, ok := service.durable.(interface{ Close() error }); ok {
			result = errors.Join(result, closer.Close())
		}
	}
	return result
}

// StartLifecycle starts the runtime phase selected by the Factory
// Sessions-owned process lifecycle plan. Initializer only executes that
// already-declared neutral plan.
func (a *Assembly) scopedStartLifecycle(runtime *SessionRuntime, ctx, runCtx context.Context) error {
	if runtime == nil {
		return errors.New("start runtime: Factory Session runtime is required")
	}
	runtime.runtimeMu.Lock()
	runtime.callerContext = initializer.CancellationOrigin(ctx)
	if runtime.callerContext == nil {
		runtime.callerContext = ctx
	}
	runtime.runtimeMu.Unlock()
	runtime.startTime = runtime.clock.Now()
	serviceMode := runtimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService
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
	handle, err := runtime.StartDefaultRuntime(ctx, runCtx)
	if err != nil {
		return err
	}
	if handle == nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.New("start runtime: Factory Session runtime was not activated")
	}
	return nil
}

// StartWorkerLifecycle activates the runtime's worker-side automation.
func (a *Assembly) scopedStartWorkerLifecycle(runtime *SessionRuntime, ctx context.Context) (RuntimeStop, error) {
	if runtime == nil {
		return nil, errors.New("start runtime automation: Factory Session runtime is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current := runtime.runtimeState.ActiveHandle()
	if current == nil || current.RuntimeInstance() == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("start runtime automation: runtime is not started")
	}
	serviceMode := runtimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService
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
func (a *Assembly) scopedCompleteStartup(runtime *SessionRuntime, ctx context.Context) error {
	if runtime == nil {
		return errors.New("complete runtime startup: Factory Session runtime is required")
	}
	serviceMode := runtimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService
	if serviceMode && runtime.workFile != "" {
		// A process-backed run starts through Root.Start and then hosts its
		// transport. Both phases complete startup on this same runtime.
		runtime.startupWorkOnce.Do(func() {
			if err := runtime.submitWorkFile(ctx); err != nil {
				sessionID := runtime.runSessionID()
				runtime.startupWorkErr = runtimebinding.FailStartup(
					a.state, &runtime.runtimeState, sessionID,
					runtime.runtimeState.ActiveHandle(), runtime.StopLiveRuntime, err,
				)
				a.releaseWorkAdmissionProjection(sessionID)
			}
		})
		return runtime.startupWorkErr
	}
	return nil
}

// WaitForRuntime waits for the currently active runtime, following a
// replacement handle when a session swap occurs.
func (a *Assembly) scopedWaitForRuntime(runtime *SessionRuntime, ctx context.Context) error {
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
		active := runtime.runtimeState.Active()
		if runtimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService &&
			active != nil && a.state.Resolve(active.SessionID) != nil {
			continue
		}
		return current.Result()
	}
}

// StopLifecycle stops only the runtime admitted by this lifecycle. Other live
// sessions can belong to concurrent invocations in the same process graph and
// must not be treated as children of this invocation.
func (a *Assembly) scopedStopLifecycle(runtime *SessionRuntime, unused context.Context) error {
	if runtime == nil {
		return nil
	}
	sessionID := runtime.startupSessionID
	if sessionID == "" {
		sessionID = DefaultFactorySessionID
	}
	var result error
	if a.state.Resolve(sessionID) != nil {
		if err := runtime.StopLiveRuntime(runtime.runtimeState.ActiveHandle()); err != nil &&
			!errors.Is(err, context.Canceled) &&
			!errors.Is(err, factorysessions.ErrSessionNotFound) &&
			!errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
			result = err
		}
	}
	runtime.runtimeState.ClearActive()
	return result
}

// FailStartup records a process-startup failure on the default Factory Session.
func (a *Assembly) scopedFailStartup(runtime *SessionRuntime, err error) error {
	if runtime == nil {
		return err
	}
	current := runtime.runtimeState.ActiveHandle()
	sessionID := runtime.runSessionID()
	failure := runtimebinding.FailStartup(
		a.state, &runtime.runtimeState, sessionID,
		current, runtime.StopLiveRuntime, err,
	)
	a.releaseWorkAdmissionProjection(sessionID)
	return failure
}

func (runtime *SessionRuntime) StartLifecycle(ctx, runCtx context.Context) error {
	var a *Assembly
	if runtime != nil {
		a = runtime.owner
	}
	return a.scopedStartLifecycle(runtime, ctx, runCtx)
}

func (runtime *SessionRuntime) StartWorkerLifecycle(ctx context.Context) (RuntimeStop, error) {
	var a *Assembly
	if runtime != nil {
		a = runtime.owner
	}
	return a.scopedStartWorkerLifecycle(runtime, ctx)
}

func (runtime *SessionRuntime) CompleteStartup(ctx context.Context) error {
	var a *Assembly
	if runtime != nil {
		a = runtime.owner
	}
	return a.scopedCompleteStartup(runtime, ctx)
}

func (runtime *SessionRuntime) WaitForRuntime(ctx context.Context) error {
	var a *Assembly
	if runtime != nil {
		a = runtime.owner
	}
	return a.scopedWaitForRuntime(runtime, ctx)
}

func (runtime *SessionRuntime) StopLifecycle(unused context.Context) error {
	var a *Assembly
	if runtime != nil {
		a = runtime.owner
	}
	return a.scopedStopLifecycle(runtime, unused)
}

func (runtime *SessionRuntime) FailStartup(err error) error {
	var a *Assembly
	if runtime != nil {
		a = runtime.owner
	}
	return a.scopedFailStartup(runtime, err)
}

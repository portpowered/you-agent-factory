// Runtime lifecycle operations expose domain actions to initializer.
package service

import (
	"context"
	"errors"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

type RuntimeStop = factorysessions.RuntimeStop

type ownedSession struct {
	id    string
	owner factorysessions.Service
}

// Close drains every live Factory Session owned by this process root. A
// command stops only its admitted session; process shutdown owns the rest.
func (a *Assembly) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	var result error
	for _, session := range a.ownedSessionsForClose() {
		result = errors.Join(result, closeOwnedSession(ctx, session))
	}
	return result
}

func (a *Assembly) ownedSessionsForClose() []ownedSession {
	a.detachedMu.RLock()
	defer a.detachedMu.RUnlock()
	if a.state == nil || a.state.Registry() == nil {
		return a.detachedSessionsInReverseOrder()
	}
	ids := a.state.Registry().IDs()
	owned := make([]ownedSession, 0, len(ids))
	for index := len(ids) - 1; index >= 0; index-- {
		id := ids[index]
		if owner := a.ownerForClose(id); owner != nil {
			owned = append(owned, ownedSession{id: id, owner: owner})
		}
	}
	return owned
}

func (a *Assembly) detachedSessionsInReverseOrder() []ownedSession {
	owned := make([]ownedSession, 0, len(a.detachedGatewayOrder))
	for index := len(a.detachedGatewayOrder) - 1; index >= 0; index-- {
		id := a.detachedGatewayOrder[index]
		if owner := a.detachedGateways[id]; owner != nil {
			owned = append(owned, ownedSession{id: id, owner: owner})
		}
	}
	return owned
}

func (a *Assembly) ownerForClose(sessionID string) factorysessions.Service {
	if owner := a.detachedGateways[sessionID]; owner != nil {
		return owner
	}
	for index := len(a.detachedGatewayOrder) - 1; index >= 0; index-- {
		if owner := a.detachedGateways[a.detachedGatewayOrder[index]]; owner != nil {
			return owner
		}
	}
	return nil
}

func closeOwnedSession(ctx context.Context, session ownedSession) error {
	control, ok := session.owner.(factorysessions.LiveControlService)
	if !ok {
		return fmt.Errorf("close Factory Session %s: %w: live control capability unavailable", session.id, factorysessions.ErrDetachedServiceUnavailable)
	}
	err := control.CloseFactorySession(ctx, session.id)
	if errors.Is(err, context.Canceled) || errors.Is(err, factorysessions.ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("close Factory Session %s: %w", session.id, err)
	}
	return nil
}

// StartLifecycle starts the runtime phase selected by the Factory
// Sessions-owned process lifecycle plan. Initializer only executes that
// already-declared neutral plan.
func (runtime *SessionRuntime) StartLifecycle(ctx, runCtx context.Context) error {
	if runtime == nil {
		return errors.New("start runtime: Factory Session runtime is required")
	}
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
	_, err := runtime.StartDefaultRuntime(ctx, runCtx)
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
func (runtime *SessionRuntime) CompleteStartup(ctx context.Context) error {
	if runtime == nil {
		return errors.New("complete runtime startup: Factory Session runtime is required")
	}
	current := runtime.runtimeState.ActiveHandle()
	serviceMode := runtimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService
	if serviceMode && runtime.workFile != "" {
		if err := runtime.submitWorkFile(ctx); err != nil {
			sessionID := runtime.runSessionID()
			failure := runtimebinding.FailStartup(
				runtime.sessionState, &runtime.runtimeState, sessionID,
				current, runtime.StopLiveRuntime, err,
			)
			if runtime.releaseWorkAdmissionProjection != nil {
				runtime.releaseWorkAdmissionProjection(sessionID)
			}
			return failure
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
		active := runtime.runtimeState.Active()
		if runtimeModeOrDefault(runtime.runtimeMode) == interfaces.RuntimeModeService &&
			active != nil && runtime.sessionState.Resolve(active.SessionID) != nil {
			continue
		}
		return current.Result()
	}
}

// StopLifecycle stops only the runtime admitted by this lifecycle. Other live
// sessions can belong to concurrent invocations in the same process graph and
// must not be treated as children of this invocation.
func (runtime *SessionRuntime) StopLifecycle(_ context.Context) error {
	if runtime == nil {
		return nil
	}
	sessionID := runtime.startupSessionID
	if sessionID == "" {
		sessionID = DefaultFactorySessionID
	}
	var result error
	if runtime.sessionState.Resolve(sessionID) != nil {
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
func (runtime *SessionRuntime) FailStartup(err error) error {
	if runtime == nil {
		return err
	}
	current := runtime.runtimeState.ActiveHandle()
	sessionID := runtime.runSessionID()
	failure := runtimebinding.FailStartup(
		runtime.sessionState, &runtime.runtimeState, sessionID,
		current, runtime.StopLiveRuntime, err,
	)
	if runtime.releaseWorkAdmissionProjection != nil {
		runtime.releaseWorkAdmissionProjection(sessionID)
	}
	return failure
}

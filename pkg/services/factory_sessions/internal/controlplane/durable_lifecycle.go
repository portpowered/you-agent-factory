package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// DurableLifecycleHost exposes durable execution backend access for control-plane routing.
type DurableLifecycleHost interface {
	DurableExecution() factorysessions.ExecutionService
}

// ErrDurableSessionLifecycleRouting reports that session identity does not route to durable execution.
var ErrDurableSessionLifecycleRouting = errors.New("durable factory session lifecycle routing required")

func requireDurableSessionID(host DurableLifecycleHost, sessionID string) error {
	if !IsDurableExecutionSessionID(sessionID) {
		if host != nil {
			if replay, ok := host.DurableExecution().(interface{ IsNonLiveReplay() bool }); ok && replay.IsNonLiveReplay() {
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrDurableSessionLifecycleRouting, strings.TrimSpace(sessionID))
	}
	return nil
}

func durableExecutionService(host DurableLifecycleHost) (factorysessions.ExecutionService, error) {
	if host == nil || host.DurableExecution() == nil {
		return nil, fmt.Errorf("durable session execution is required")
	}
	return host.DurableExecution(), nil
}

// PauseDurableFactorySession routes pause to durable execution when session identity is durable.
func PauseDurableFactorySession(
	ctx context.Context,
	host DurableLifecycleHost,
	sessionID string,
	control factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if err := requireDurableSessionID(host, sessionID); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	execution, err := durableExecutionService(host)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Pause(ctx, sessionID, control)
}

// ResumeDurableFactorySession routes resume to durable execution when session identity is durable.
func ResumeDurableFactorySession(
	ctx context.Context,
	host DurableLifecycleHost,
	sessionID string,
	control factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if err := requireDurableSessionID(host, sessionID); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	execution, err := durableExecutionService(host)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Resume(ctx, sessionID, control)
}

// CancelDurableFactorySession routes cancel to durable execution when session identity is durable.
func CancelDurableFactorySession(
	ctx context.Context,
	host DurableLifecycleHost,
	sessionID string,
	control factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if err := requireDurableSessionID(host, sessionID); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	execution, err := durableExecutionService(host)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Cancel(ctx, sessionID, control)
}

// TerminateDurableFactorySession routes terminate to durable execution when session identity is durable.
func TerminateDurableFactorySession(
	ctx context.Context,
	host DurableLifecycleHost,
	sessionID string,
	control factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if err := requireDurableSessionID(host, sessionID); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	execution, err := durableExecutionService(host)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Terminate(ctx, sessionID, control)
}

// ApproveDurableFactorySession routes approve to durable execution when session identity is durable.
func ApproveDurableFactorySession(
	ctx context.Context,
	host DurableLifecycleHost,
	sessionID string,
	approve factorysessions.ApproveRequest,
) (factorysessions.LifecycleControlResult, error) {
	if err := requireDurableSessionID(host, sessionID); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	execution, err := durableExecutionService(host)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Approve(ctx, sessionID, approve)
}

// RetryDurableFactorySessionDispatch routes retry-dispatch to durable execution when session identity is durable.
func RetryDurableFactorySessionDispatch(
	ctx context.Context,
	host DurableLifecycleHost,
	sessionID string,
	retry factorysessions.RetryDispatchRequest,
) (factorysessions.LifecycleControlResult, error) {
	if err := requireDurableSessionID(host, sessionID); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	execution, err := durableExecutionService(host)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.RetryDispatch(ctx, sessionID, retry)
}

// InterruptDurableFactorySessionDispatch routes interrupt-dispatch to durable execution when session identity is durable.
func InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	host DurableLifecycleHost,
	sessionID string,
	interrupt factorysessions.InterruptDispatchRequest,
) (factorysessions.LifecycleControlResult, error) {
	if err := requireDurableSessionID(host, sessionID); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	execution, err := durableExecutionService(host)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.InterruptDispatch(ctx, sessionID, interrupt)
}

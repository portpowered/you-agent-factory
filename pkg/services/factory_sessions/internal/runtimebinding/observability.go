package runtimebinding

import (
	"context"
	"errors"

	factorymetrics "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"go.uber.org/zap"
)

// ObserveLifecycleControl records the canonical log and runtime metric for one
// live Factory Session lifecycle-control attempt.
func ObserveLifecycleControl(
	logger *zap.Logger,
	resolver LiveSessionResolver,
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	control factorysessions.ControlRequest,
	outcome factorysessions.LifecycleControlOutcome,
	status factorysessions.LifecycleStatus,
	err error,
) {
	if errors.Is(err, factorysessions.ErrSessionNotFound) {
		err = factorysessions.ErrSessionNotFound
	}
	outcomeClass := factorysessions.LifecycleControlOutcomeClass(outcome, err)
	fields := factorysessions.LiveLifecycleControlLogFields(sessionID, operation, outcomeClass, status, control)
	if logger != nil {
		switch outcomeClass {
		case factorysessions.LifecycleControlOutcomeClassNotFound,
			string(factorysessions.LifecycleControlOutcomeInvalidState),
			string(factorysessions.LifecycleControlOutcomeTerminalSession):
			logger.Warn("factory session lifecycle control rejected", fields...)
		default:
			logger.Info("factory session lifecycle control", fields...)
		}
	}

	session, resolveErr := RequireLiveSession(resolver, sessionID)
	if resolveErr != nil {
		return
	}
	instance := HandleFromSession(session).RuntimeInstance()
	if instance == nil {
		return
	}
	_ = instance.RuntimeMetrics().Counter(context.Background(), factorymetrics.RuntimeLifecycleControl, 1, factorymetrics.Fields{
		Outcome: outcomeClass,
		Reason:  string(operation),
	})
}

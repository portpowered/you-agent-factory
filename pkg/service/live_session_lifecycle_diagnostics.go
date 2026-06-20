package service

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	"go.uber.org/zap"
)

const (
	runtimeMetricLifecycleControl = "runtime.lifecycle_control"
)

func (fs *FactoryService) observeLiveLifecycleControl(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
	err error,
) {
	if fs == nil {
		return
	}

	outcomeClass := lifecycleControlOutcomeClass(outcome, err)
	fields := liveLifecycleControlLogFields(sessionID, operation, outcomeClass, status, control)
	switch outcomeClass {
	case lifecycleControlOutcomeClassNotFound,
		string(factorysessionexecution.LifecycleControlOutcomeInvalidState),
		string(factorysessionexecution.LifecycleControlOutcomeTerminalSession):
		fs.logger.Warn("factory session lifecycle control rejected", fields...)
	default:
		fs.logger.Info("factory session lifecycle control", fields...)
	}

	fs.emitLiveLifecycleControlMetric(sessionID, operation, outcomeClass)
}

func lifecycleControlOutcomeClass(
	outcome factorysessionexecution.LifecycleControlOutcome,
	err error,
) string {
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return lifecycleControlOutcomeClassNotFound
		}
		var controlErr *factorysessionexecution.ControlError
		if errors.As(err, &controlErr) {
			return string(controlErr.Outcome)
		}
		return "ERROR"
	}
	if outcome == "" {
		return "ERROR"
	}
	return string(outcome)
}

const (
	lifecycleControlOutcomeClassNotFound = "NOT_FOUND"
)

func liveLifecycleControlLogFields(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcomeClass string,
	status factorysessionexecution.LifecycleStatus,
	control factorysessionexecution.ControlRequest,
) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("operation", string(operation)),
		zap.String("outcome", outcomeClass),
	}
	if status != "" {
		fields = append(fields, zap.String("lifecycle_control_status", string(status)))
	}
	if requestID := control.RequestID; requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}
	return fields
}

func (fs *FactoryService) emitLiveLifecycleControlMetric(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcomeClass string,
) {
	if fs == nil {
		return
	}
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return
	}
	bundle := liveSessionHandle(session).runtime
	if bundle == nil {
		return
	}
	bundle.emitMetricCounter(runtimeMetricLifecycleControl, 1, metrics.Fields{
		Outcome: outcomeClass,
		Reason:  string(operation),
	})
}

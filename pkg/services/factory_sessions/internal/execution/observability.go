package factorysessionexecution

import (
	"errors"

	"go.uber.org/zap"
)

const LifecycleControlOutcomeClassNotFound = "NOT_FOUND"

// LifecycleControlOutcomeClass normalizes lifecycle outcomes and errors into
// the stable low-cardinality class used by logs and metrics.
func LifecycleControlOutcomeClass(outcome LifecycleControlOutcome, err error) string {
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return LifecycleControlOutcomeClassNotFound
		}
		var controlErr *ControlError
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

// LiveLifecycleControlLogFields returns the canonical structured fields for a
// live-session lifecycle control observation.
func LiveLifecycleControlLogFields(sessionID string, operation LifecycleControlKind, outcomeClass string, status LifecycleStatus, control ControlRequest) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("operation", string(operation)),
		zap.String("outcome", outcomeClass),
	}
	if status != "" {
		fields = append(fields, zap.String("lifecycle_control_status", string(status)))
	}
	if control.RequestID != "" {
		fields = append(fields, zap.String("request_id", control.RequestID))
	}
	return fields
}

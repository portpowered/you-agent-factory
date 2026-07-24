package factorysessionexecution

import (
	"go.uber.org/zap"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func LifecycleControlOutcomeClass(outcome LifecycleControlOutcome, err error) string {
	return factorysessions.LifecycleControlOutcomeClass(outcome, err)
}

func LiveLifecycleControlLogFields(sessionID string, operation LifecycleControlKind, outcomeClass string, status LifecycleStatus, control ControlRequest) []zap.Field {
	return factorysessions.LiveLifecycleControlLogFields(sessionID, operation, outcomeClass, status, control)
}

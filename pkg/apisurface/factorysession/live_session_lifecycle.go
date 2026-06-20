package factorysession

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// FactoryStateToLifecycleStatus maps one live factory runtime state to the durable
// lifecycle vocabulary used by lifecycle-control responses.
func FactoryStateToLifecycleStatus(state interfaces.FactoryState) factorysessionexecution.LifecycleStatus {
	switch state {
	case interfaces.FactoryStatePaused:
		return factorysessionexecution.LifecycleStatusPaused
	case interfaces.FactoryStateCompleted:
		return factorysessionexecution.LifecycleStatusSucceeded
	case interfaces.FactoryStateFailed:
		return factorysessionexecution.LifecycleStatusFailed
	default:
		return factorysessionexecution.LifecycleStatusRunning
	}
}

// LiveLifecycleControlLinksForSession builds post-control inspection links for one
// live factory session.
func LiveLifecycleControlLinksForSession(sessionID string) factorysessionexecution.LifecycleControlLinks {
	sessionPath := "/factory-sessions/" + sessionID
	return factorysessionexecution.LifecycleControlLinks{
		Session: sessionPath,
		Status:  sessionPath,
	}
}

// LiveLifecycleControlResponse builds the public lifecycle-control response for one
// live factory session control result.
func LiveLifecycleControlResponse(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
) factoryapi.FactorySessionLifecycleControlResponse {
	return LifecycleControlResponseToAPI(factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    status,
		Links:     LiveLifecycleControlLinksForSession(sessionID),
	})
}

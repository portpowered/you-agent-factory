package apisurface

import (
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactorySessionCursorPreflightResult maps generated HTTP sync-preflight
// fields into the Factory Session owner's recovery-classification input.
func FactorySessionCursorPreflightResult(
	response factoryapi.FactorySessionSyncPreflightResponse,
) factorysessions.CursorPreflightResult {
	return factorysessions.CursorPreflightResult{
		Reason:             factorySessionCursorPreflightReason(response.ReasonCode),
		RequestedSessionID: response.RequestedSessionId,
		Scope: factorysessions.CursorIdentityScope{
			BackendScopeID:      cursorStringValue(response.BackendScopeId),
			LogicalSessionKeyID: cursorStringValue(response.LogicalSessionKeyId),
			FactorySessionID:    cursorStringValue(response.FactorySessionId),
			StreamGenerationID:  cursorStringValue(response.StreamGenerationId),
		},
	}
}

func factorySessionCursorPreflightReason(
	reason factoryapi.FactorySessionSyncPreflightReasonCode,
) factorysessions.CursorPreflightReason {
	switch reason {
	case factoryapi.CursorStale:
		return factorysessions.CursorPreflightStale
	case factoryapi.SessionNotFound:
		return factorysessions.CursorPreflightSessionNotFound
	case factoryapi.LogicalSessionRemap:
		return factorysessions.CursorPreflightSessionRemapped
	default:
		return ""
	}
}

func cursorStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

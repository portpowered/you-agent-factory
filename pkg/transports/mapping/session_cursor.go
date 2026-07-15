package apisurface

import (
	"strings"

	factorysessioncursors "github.com/portpowered/infinite-you/pkg/factory/sessions/cursors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactorySessionCursorPreflightResult maps generated HTTP sync-preflight
// fields into the Factory Session owner's recovery-classification input.
func FactorySessionCursorPreflightResult(
	response factoryapi.FactorySessionSyncPreflightResponse,
) factorysessioncursors.PreflightResult {
	return factorysessioncursors.PreflightResult{
		Reason:             factorySessionCursorPreflightReason(response.ReasonCode),
		RequestedSessionID: response.RequestedSessionId,
		Scope: factorysessioncursors.IdentityScope{
			BackendScopeID:      cursorStringValue(response.BackendScopeId),
			LogicalSessionKeyID: cursorStringValue(response.LogicalSessionKeyId),
			FactorySessionID:    cursorStringValue(response.FactorySessionId),
			StreamGenerationID:  cursorStringValue(response.StreamGenerationId),
		},
	}
}

func factorySessionCursorPreflightReason(
	reason factoryapi.FactorySessionSyncPreflightReasonCode,
) factorysessioncursors.PreflightReason {
	switch reason {
	case factoryapi.CursorStale:
		return factorysessioncursors.PreflightCursorStale
	case factoryapi.SessionNotFound:
		return factorysessioncursors.PreflightSessionNotFound
	case factoryapi.LogicalSessionRemap:
		return factorysessioncursors.PreflightSessionRemapped
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

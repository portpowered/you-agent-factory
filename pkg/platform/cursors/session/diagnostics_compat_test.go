package sessioncursor

import (
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/factory/sessions/cursors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type InvalidationReason = factorysessioncursors.InvalidationReason
type RecoveryAction = factorysessioncursors.RecoveryAction
type IdentityScope = factorysessioncursors.IdentityScope
type InvalidationDiagnostic = factorysessioncursors.InvalidationDiagnostic

const (
	ReasonCursorStale               = factorysessioncursors.ReasonCursorStale
	ReasonSessionNotFound           = factorysessioncursors.ReasonSessionNotFound
	ReasonSessionRemapped           = factorysessioncursors.ReasonSessionRemapped
	ReasonStreamGenerationChanged   = factorysessioncursors.ReasonStreamGenerationChanged
	ReasonBackendScopeChanged       = factorysessioncursors.ReasonBackendScopeChanged
	RecoveryClearCheckpoint         = factorysessioncursors.RecoveryClearCheckpoint
	RecoveryClearStreamDerivedState = factorysessioncursors.RecoveryClearStreamDerivedState
	RecoveryReplayWithoutCursor     = factorysessioncursors.RecoveryReplayWithoutCursor
	RecoveryShowExplicitRecovery    = factorysessioncursors.RecoveryShowExplicitRecovery
)

var (
	ClassifyIdentityMismatch          = factorysessioncursors.ClassifyIdentityMismatch
	SilentReplayRecoveryDiagnostic    = factorysessioncursors.SilentReplayRecoveryDiagnostic
	IdentityMismatchDiagnostic        = factorysessioncursors.IdentityMismatchDiagnostic
	RecoveryActionForIdentityMismatch = factorysessioncursors.RecoveryActionForIdentityMismatch
)

func InvalidationFromSyncPreflight(response factoryapi.FactorySessionSyncPreflightResponse) (InvalidationDiagnostic, bool) {
	return factorysessioncursors.InvalidationFromPreflight(factorysessioncursors.PreflightResult{
		Reason:             preflightReason(response.ReasonCode),
		RequestedSessionID: response.RequestedSessionId,
		Scope: IdentityScope{
			BackendScopeID:      stringValue(response.BackendScopeId),
			LogicalSessionKeyID: stringValue(response.LogicalSessionKeyId),
			FactorySessionID:    stringValue(response.FactorySessionId),
			StreamGenerationID:  stringValue(response.StreamGenerationId),
		},
	})
}

func preflightReason(reason factoryapi.FactorySessionSyncPreflightReasonCode) factorysessioncursors.PreflightReason {
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

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

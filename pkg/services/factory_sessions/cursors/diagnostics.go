package cursors

import "strings"

// InvalidationReason is the sanitized machine-readable reason for checkpoint
// or stream-derived state invalidation.
type InvalidationReason string

const (
	ReasonCursorStale             InvalidationReason = "cursor_stale"
	ReasonSessionNotFound         InvalidationReason = "session_not_found"
	ReasonSessionRemapped         InvalidationReason = "session_remapped"
	ReasonStreamGenerationChanged InvalidationReason = "stream_generation_changed"
	ReasonBackendScopeChanged     InvalidationReason = "backend_scope_changed"
	ReasonUserClearedSessions     InvalidationReason = "user_cleared_sessions"
)

// RecoveryAction describes the Factory Session recovery path selected after
// invalidation.
type RecoveryAction string

const (
	RecoveryClearCheckpoint         RecoveryAction = "clear_checkpoint"
	RecoveryClearStreamDerivedState RecoveryAction = "clear_stream_derived_state"
	RecoveryReplayWithoutCursor     RecoveryAction = "replay_without_cursor"
	RecoveryShowExplicitRecovery    RecoveryAction = "show_explicit_recovery"
	RecoveryReuseCheckpoint         RecoveryAction = "reuse_checkpoint"
)

// IdentityScope carries only safe session persistence identity fields.
type IdentityScope struct {
	BackendScopeID      string
	LogicalSessionKeyID string
	FactorySessionID    string
	StreamGenerationID  string
}

// InvalidationDiagnostic is a structured, sanitized invalidation record.
type InvalidationDiagnostic struct {
	Reason             InvalidationReason
	RecoveryAction     RecoveryAction
	Scope              IdentityScope
	RequestedSessionID string
	PreviousScope      *IdentityScope
}

// PreflightReason is the domain-owned result relevant to cursor recovery.
type PreflightReason string

const (
	PreflightCursorStale     PreflightReason = "cursor_stale"
	PreflightSessionNotFound PreflightReason = "session_not_found"
	PreflightSessionRemapped PreflightReason = "session_remapped"
)

// PreflightResult carries transport-neutral sync-preflight facts used to
// classify reconnect recovery.
type PreflightResult struct {
	Reason             PreflightReason
	Scope              IdentityScope
	RequestedSessionID string
}

// InvalidationFromPreflight classifies a domain-owned preflight result.
func InvalidationFromPreflight(result PreflightResult) (InvalidationDiagnostic, bool) {
	reason, recovery, ok := reasonAndRecoveryFromPreflight(result.Reason)
	if !ok {
		return InvalidationDiagnostic{}, false
	}
	return InvalidationDiagnostic{
		Reason:             reason,
		RecoveryAction:     recovery,
		Scope:              NormalizeScope(result.Scope),
		RequestedSessionID: strings.TrimSpace(result.RequestedSessionID),
	}, true
}

func reasonAndRecoveryFromPreflight(reason PreflightReason) (InvalidationReason, RecoveryAction, bool) {
	switch reason {
	case PreflightCursorStale:
		return ReasonCursorStale, RecoveryReplayWithoutCursor, true
	case PreflightSessionNotFound:
		return ReasonSessionNotFound, RecoveryShowExplicitRecovery, true
	case PreflightSessionRemapped:
		return ReasonSessionRemapped, RecoveryClearStreamDerivedState, true
	default:
		return "", "", false
	}
}

// ClassifyIdentityMismatch returns the most specific invalidation reason when
// cached identity no longer matches the current server-owned identity set.
func ClassifyIdentityMismatch(previous, current IdentityScope) (InvalidationReason, bool) {
	previous = NormalizeScope(previous)
	current = NormalizeScope(current)
	if previous == current {
		return "", false
	}
	if previous.BackendScopeID != "" && current.BackendScopeID != "" && previous.BackendScopeID != current.BackendScopeID {
		return ReasonBackendScopeChanged, true
	}
	if previous.FactorySessionID != "" && current.FactorySessionID != "" && previous.FactorySessionID != current.FactorySessionID {
		return ReasonSessionRemapped, true
	}
	return ReasonStreamGenerationChanged, true
}

// NormalizeScope trims safe identity values at the domain boundary.
func NormalizeScope(scope IdentityScope) IdentityScope {
	return IdentityScope{
		BackendScopeID:      strings.TrimSpace(scope.BackendScopeID),
		LogicalSessionKeyID: strings.TrimSpace(scope.LogicalSessionKeyID),
		FactorySessionID:    strings.TrimSpace(scope.FactorySessionID),
		StreamGenerationID:  strings.TrimSpace(scope.StreamGenerationID),
	}
}

// RecoveryActionForIdentityMismatch returns the defined recovery for an
// identity mismatch classification.
func RecoveryActionForIdentityMismatch(reason InvalidationReason) RecoveryAction {
	switch reason {
	case ReasonBackendScopeChanged, ReasonSessionRemapped, ReasonStreamGenerationChanged:
		return RecoveryClearStreamDerivedState
	default:
		return RecoveryClearCheckpoint
	}
}

// SilentReplayRecoveryDiagnostic reports stale-cursor recovery without
// exposing provider or work data.
func SilentReplayRecoveryDiagnostic(scope IdentityScope, requestedSessionID string) InvalidationDiagnostic {
	return InvalidationDiagnostic{
		Reason:             ReasonCursorStale,
		RecoveryAction:     RecoveryReplayWithoutCursor,
		Scope:              NormalizeScope(scope),
		RequestedSessionID: strings.TrimSpace(requestedSessionID),
	}
}

// IdentityMismatchDiagnostic builds a sanitized mismatch diagnostic.
func IdentityMismatchDiagnostic(previous, current IdentityScope, requestedSessionID string) (InvalidationDiagnostic, bool) {
	reason, ok := ClassifyIdentityMismatch(previous, current)
	if !ok {
		return InvalidationDiagnostic{}, false
	}
	previous = NormalizeScope(previous)
	return InvalidationDiagnostic{
		Reason:             reason,
		RecoveryAction:     RecoveryActionForIdentityMismatch(reason),
		Scope:              NormalizeScope(current),
		PreviousScope:      &previous,
		RequestedSessionID: strings.TrimSpace(requestedSessionID),
	}, true
}

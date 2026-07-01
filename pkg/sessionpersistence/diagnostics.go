package sessionpersistence

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const (
	MetricSessionPersistenceInvalidation = "runtime.session_persistence.invalidation"
)

// InvalidationReason is the sanitized machine-readable reason for checkpoint
// or stream-derived state invalidation.
type InvalidationReason string

const (
	ReasonCursorStale              InvalidationReason = "cursor_stale"
	ReasonSessionNotFound          InvalidationReason = "session_not_found"
	ReasonSessionRemapped          InvalidationReason = "session_remapped"
	ReasonStreamGenerationChanged  InvalidationReason = "stream_generation_changed"
	ReasonBackendScopeChanged      InvalidationReason = "backend_scope_changed"
	ReasonUserClearedSessions      InvalidationReason = "user_cleared_sessions"
)

// RecoveryAction describes the recovery path taken after invalidation.
type RecoveryAction string

const (
	RecoveryClearCheckpoint          RecoveryAction = "clear_checkpoint"
	RecoveryClearStreamDerivedState  RecoveryAction = "clear_stream_derived_state"
	RecoveryReplayWithoutCursor      RecoveryAction = "replay_without_cursor"
	RecoveryShowExplicitRecovery     RecoveryAction = "show_explicit_recovery"
	RecoveryReuseCheckpoint          RecoveryAction = "reuse_checkpoint"
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

// Logger emits structured operator diagnostics.
type Logger interface {
	Info(msg string, fields map[string]string)
}

// MetricsRecorder receives invalidation counter emissions.
type MetricsRecorder interface {
	RecordMetric(name string, labels map[string]string)
}

// Observer records session persistence invalidation diagnostics.
type Observer struct {
	Logger  Logger
	Metrics MetricsRecorder
}

func (o Observer) Record(diagnostic InvalidationDiagnostic) {
	fields := DiagnosticFields(diagnostic)
	if o.Logger != nil {
		o.Logger.Info("session persistence invalidation", fields)
	}
	if o.Metrics != nil {
		o.Metrics.RecordMetric(MetricSessionPersistenceInvalidation, cloneFields(fields))
	}
}

func DiagnosticFields(diagnostic InvalidationDiagnostic) map[string]string {
	fields := map[string]string{
		"reason":          string(diagnostic.Reason),
		"recovery_action": string(diagnostic.RecoveryAction),
	}
	if requested := strings.TrimSpace(diagnostic.RequestedSessionID); requested != "" {
		fields["requested_session_id"] = requested
	}
	appendScopeFields(fields, "scope", diagnostic.Scope)
	if diagnostic.PreviousScope != nil {
		appendScopeFields(fields, "previous_scope", *diagnostic.PreviousScope)
	}
	return fields
}

func appendScopeFields(fields map[string]string, prefix string, scope IdentityScope) {
	if backendScopeID := strings.TrimSpace(scope.BackendScopeID); backendScopeID != "" {
		fields[prefix+"_backend_scope_id"] = backendScopeID
	}
	if logicalSessionKeyID := strings.TrimSpace(scope.LogicalSessionKeyID); logicalSessionKeyID != "" {
		fields[prefix+"_logical_session_key_id"] = logicalSessionKeyID
	}
	if factorySessionID := strings.TrimSpace(scope.FactorySessionID); factorySessionID != "" {
		fields[prefix+"_factory_session_id"] = factorySessionID
	}
	if streamGenerationID := strings.TrimSpace(scope.StreamGenerationID); streamGenerationID != "" {
		fields[prefix+"_stream_generation_id"] = streamGenerationID
	}
}

func cloneFields(fields map[string]string) map[string]string {
	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}

func ScopeFromPreflightResponse(response factoryapi.FactorySessionSyncPreflightResponse) IdentityScope {
	return IdentityScope{
		BackendScopeID:      stringValue(response.BackendScopeId),
		LogicalSessionKeyID: stringValue(response.LogicalSessionKeyId),
		FactorySessionID:    stringValue(response.FactorySessionId),
		StreamGenerationID:  stringValue(response.StreamGenerationId),
	}
}

func InvalidationFromSyncPreflight(
	response factoryapi.FactorySessionSyncPreflightResponse,
) (InvalidationDiagnostic, bool) {
	reason, recovery, ok := reasonAndRecoveryFromSyncPreflight(response.ReasonCode)
	if !ok {
		return InvalidationDiagnostic{}, false
	}
	return InvalidationDiagnostic{
		Reason:             reason,
		RecoveryAction:     recovery,
		Scope:              ScopeFromPreflightResponse(response),
		RequestedSessionID: strings.TrimSpace(response.RequestedSessionId),
	}, true
}

func reasonAndRecoveryFromSyncPreflight(
	reasonCode factoryapi.FactorySessionSyncPreflightReasonCode,
) (InvalidationReason, RecoveryAction, bool) {
	switch reasonCode {
	case factoryapi.CursorStale:
		return ReasonCursorStale, RecoveryReplayWithoutCursor, true
	case factoryapi.SessionNotFound:
		return ReasonSessionNotFound, RecoveryShowExplicitRecovery, true
	case factoryapi.LogicalSessionRemap:
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
	if previous.BackendScopeID != "" &&
		current.BackendScopeID != "" &&
		previous.BackendScopeID != current.BackendScopeID {
		return ReasonBackendScopeChanged, true
	}
	if previous.FactorySessionID != "" &&
		current.FactorySessionID != "" &&
		previous.FactorySessionID != current.FactorySessionID {
		return ReasonSessionRemapped, true
	}
	if previous.StreamGenerationID != "" &&
		current.StreamGenerationID != "" &&
		previous.StreamGenerationID != current.StreamGenerationID {
		return ReasonStreamGenerationChanged, true
	}
	return ReasonStreamGenerationChanged, true
}

func NormalizeScope(scope IdentityScope) IdentityScope {
	return IdentityScope{
		BackendScopeID:      strings.TrimSpace(scope.BackendScopeID),
		LogicalSessionKeyID: strings.TrimSpace(scope.LogicalSessionKeyID),
		FactorySessionID:    strings.TrimSpace(scope.FactorySessionID),
		StreamGenerationID:  strings.TrimSpace(scope.StreamGenerationID),
	}
}

func RecoveryActionForIdentityMismatch(reason InvalidationReason) RecoveryAction {
	switch reason {
	case ReasonBackendScopeChanged, ReasonSessionRemapped, ReasonStreamGenerationChanged:
		return RecoveryClearStreamDerivedState
	default:
		return RecoveryClearCheckpoint
	}
}

func SilentReplayRecoveryDiagnostic(
	scope IdentityScope,
	requestedSessionID string,
) InvalidationDiagnostic {
	return InvalidationDiagnostic{
		Reason:             ReasonCursorStale,
		RecoveryAction:     RecoveryReplayWithoutCursor,
		Scope:              NormalizeScope(scope),
		RequestedSessionID: strings.TrimSpace(requestedSessionID),
	}
}


func IdentityMismatchDiagnostic(
	previous IdentityScope,
	current IdentityScope,
	requestedSessionID string,
) (InvalidationDiagnostic, bool) {
	reason, ok := ClassifyIdentityMismatch(previous, current)
	if !ok {
		return InvalidationDiagnostic{}, false
	}
	return InvalidationDiagnostic{
		Reason:             reason,
		RecoveryAction:     RecoveryActionForIdentityMismatch(reason),
		Scope:              NormalizeScope(current),
		PreviousScope:      scopePointer(NormalizeScope(previous)),
		RequestedSessionID: strings.TrimSpace(requestedSessionID),
	}, true
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func scopePointer(scope IdentityScope) *IdentityScope {
	normalized := NormalizeScope(scope)
	return &normalized
}

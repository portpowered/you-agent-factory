package sessioncursor

import (
	"strings"

	factorysessioncursors "github.com/portpowered/infinite-you/pkg/factory/sessions/cursors"
)

const MetricSessionPersistenceInvalidation = "runtime.session_persistence.invalidation"

// Logger emits structured operator diagnostics.
type Logger interface {
	Info(msg string, fields map[string]string)
}

// MetricsRecorder receives invalidation counter emissions.
type MetricsRecorder interface {
	RecordMetric(name string, labels map[string]string)
}

// Observer records domain-classified session persistence invalidations through
// platform logging and metric sinks.
type Observer struct {
	Logger  Logger
	Metrics MetricsRecorder
}

func (o Observer) Record(diagnostic factorysessioncursors.InvalidationDiagnostic) {
	fields := DiagnosticFields(diagnostic)
	if o.Logger != nil {
		o.Logger.Info("session persistence invalidation", fields)
	}
	if o.Metrics != nil {
		o.Metrics.RecordMetric(MetricSessionPersistenceInvalidation, cloneFields(fields))
	}
}

// DiagnosticFields formats only the domain owner's secret-safe identity and
// recovery fields.
func DiagnosticFields(diagnostic factorysessioncursors.InvalidationDiagnostic) map[string]string {
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

func appendScopeFields(fields map[string]string, prefix string, scope factorysessioncursors.IdentityScope) {
	if value := strings.TrimSpace(scope.BackendScopeID); value != "" {
		fields[prefix+"_backend_scope_id"] = value
	}
	if value := strings.TrimSpace(scope.LogicalSessionKeyID); value != "" {
		fields[prefix+"_logical_session_key_id"] = value
	}
	if value := strings.TrimSpace(scope.FactorySessionID); value != "" {
		fields[prefix+"_factory_session_id"] = value
	}
	if value := strings.TrimSpace(scope.StreamGenerationID); value != "" {
		fields[prefix+"_stream_generation_id"] = value
	}
}

func cloneFields(fields map[string]string) map[string]string {
	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}

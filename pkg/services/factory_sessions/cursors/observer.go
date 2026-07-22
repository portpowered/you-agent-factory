package cursors

import (
	"strings"

	"go.uber.org/zap"
)

const MetricSessionPersistenceInvalidation = "runtime.session_persistence.invalidation"

type Logger interface {
	Info(msg string, fields map[string]string)
}

type MetricsRecorder interface {
	RecordMetric(name string, labels map[string]string)
}

// Observer records classified Factory Session cursor invalidations.
type Observer struct {
	Logger  Logger
	Metrics MetricsRecorder
}

func NewZapObserver(logger *zap.Logger) Observer {
	return Observer{Logger: zapLogger{logger: logger}}
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

func RecordPreflight(observer Observer, result PreflightResult) bool {
	diagnostic, ok := InvalidationFromPreflight(result)
	if !ok {
		return false
	}
	observer.Record(diagnostic)
	return true
}

type zapLogger struct {
	logger *zap.Logger
}

func (l zapLogger) Info(msg string, fields map[string]string) {
	if l.logger == nil {
		return
	}
	zapFields := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		zapFields = append(zapFields, zap.String(key, value))
	}
	l.logger.Info(msg, zapFields...)
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

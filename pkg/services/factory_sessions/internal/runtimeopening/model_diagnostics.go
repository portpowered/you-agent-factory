package runtimeopening

import (
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

func ModelHostDiagnosticLogger(logger *zap.Logger) models.HostDiagnosticLogger {
	if logger == nil {
		return nil
	}
	return zapModelHostLogger{logger: logger.Named("modelhost")}
}

func ModelHostDiagnosticMetrics(recorder roles.InvocationMetricsRecorder) models.HostMetricsRecorder {
	if recorder == nil {
		return nil
	}
	return modelHostMetricsRecorder{recorder: recorder}
}

type zapModelHostLogger struct{ logger *zap.Logger }

func (l zapModelHostLogger) Info(message string, fields map[string]string) {
	l.logger.Info(message, modelHostZapFields(fields)...)
}

func (l zapModelHostLogger) Warn(message string, fields map[string]string) {
	l.logger.Warn(message, modelHostZapFields(fields)...)
}

func modelHostZapFields(fields map[string]string) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		out = append(out, zap.String(key, value))
	}
	return out
}

type modelHostMetricsRecorder struct {
	recorder roles.InvocationMetricsRecorder
}

func (a modelHostMetricsRecorder) RecordMetric(name string, labels map[string]string) {
	if a.recorder == nil || strings.TrimSpace(name) == "" {
		return
	}
	a.recorder.RecordInvocationMetric(factorysessions.InvocationMetric{
		Name: name, Labels: cloneModelMetricLabels(labels),
	})
}

func cloneModelMetricLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

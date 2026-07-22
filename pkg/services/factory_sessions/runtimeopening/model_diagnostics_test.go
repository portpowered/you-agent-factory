package runtimeopening

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"go.uber.org/zap"
)

type modelMetricCapture struct {
	metrics []factorysessions.InvocationMetric
}

func (c *modelMetricCapture) RecordInvocationMetric(metric factorysessions.InvocationMetric) {
	c.metrics = append(c.metrics, metric)
}

func TestModelHostDiagnosticsBridgesInvocationMetrics(t *testing.T) {
	metrics := &modelMetricCapture{}
	logger := ModelHostDiagnosticLogger(zap.NewNop())
	metricRecorder := ModelHostDiagnosticMetrics(metrics)
	if logger == nil || metricRecorder == nil {
		t.Fatalf("model host diagnostics = (%#v, %#v), want logger and metrics", logger, metricRecorder)
	}

	logger.Info("model ready", map[string]string{"model": "fixture"})
	logger.Warn("model reused", nil)
	labels := map[string]string{"model": "fixture"}
	metricRecorder.RecordMetric("model.load", labels)
	labels["model"] = "mutated"
	if len(metrics.metrics) != 1 || metrics.metrics[0].Name != "model.load" {
		t.Fatalf("recorded metrics = %#v, want one model.load metric", metrics.metrics)
	}
	if got := metrics.metrics[0].Labels["model"]; got != "fixture" {
		t.Fatalf("recorded model label = %q, want an isolated copy", got)
	}
}

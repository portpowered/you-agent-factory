package runtime_api

import (
	"testing"
	"time"

	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type modelMetricCapture struct {
	metrics []factoryservice.InvocationMetric
}

func (c *modelMetricCapture) RecordInvocationMetric(metric factoryservice.InvocationMetric) {
	c.metrics = append(c.metrics, metric)
}

func TestModelConstructionDiagnosticsAndRecordedEventsCrossRuntimeBoundaries(t *testing.T) {
	metrics := &modelMetricCapture{}
	domain := factoryservice.LocalModelDomainDependencies(factoryservice.Config{
		ModelCacheDir:             t.TempDir(),
		Logger:                    zap.NewNop(),
		InvocationMetricsRecorder: metrics,
	})
	if domain.CacheDir == "" || domain.Diagnostics.Logger == nil || domain.Diagnostics.Metrics == nil {
		t.Fatalf("model domain dependencies = %#v, want cache and host diagnostics", domain)
	}

	domain.Diagnostics.Logger.Info("model ready", map[string]string{"model": "fixture"})
	domain.Diagnostics.Logger.Warn("model reused", nil)
	labels := map[string]string{"model": "fixture"}
	domain.Diagnostics.Metrics.RecordMetric("model.load", labels)
	labels["model"] = "mutated"
	if len(metrics.metrics) != 1 || metrics.metrics[0].Name != "model.load" {
		t.Fatalf("recorded metrics = %#v, want one model.load metric", metrics.metrics)
	}
	if got := metrics.metrics[0].Labels["model"]; got != "fixture" {
		t.Fatalf("recorded model label = %q, want an isolated copy", got)
	}

	localTime := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.FixedZone("fixture", -7*60*60))
	history := factoryevents.NewFactoryEventHistory(nil, nil)
	history.AppendRecordedEvent(factoryapi.FactoryEvent{
		Id:      "factory-event/model-boundary/1",
		Type:    factoryapi.FactoryEventTypeModelResponse,
		Context: factoryapi.FactoryEventContext{Tick: 1, EventTime: localTime},
	})
	recorded := history.Events()
	if len(recorded) != 1 {
		t.Fatalf("recorded event count = %d, want 1", len(recorded))
	}
	if recorded[0].SchemaVersion != factoryapi.AgentFactoryEventV1 || recorded[0].Context.Sequence != 0 {
		t.Fatalf("recorded event envelope = %#v, want canonical schema and sequence", recorded[0])
	}
	if got := recorded[0].Context.EventTime; got.Location() != time.UTC || !got.Equal(localTime) {
		t.Fatalf("recorded event time = %v, want the same instant normalized to UTC", got)
	}
}

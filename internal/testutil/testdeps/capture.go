package testdeps

import (
	"context"
	"maps"
	"sync"

	"github.com/portpowered/infinite-you/pkg/factory/metrics"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Capture holds handles for asserting on recorded log and metric emissions from
// explicit observability opt-in.
type Capture struct {
	ObservedLogs *observer.ObservedLogs
	Metrics      *RecordingMetricsEmitter
}

// CapturingZapLogger returns a zap logger and observer for tests that assert on
// expected log records.
func CapturingZapLogger(level zapcore.Level) (*zap.Logger, *observer.ObservedLogs) {
	core, observed := observer.New(level)
	return zap.New(core), observed
}

// Capturing returns observability dependencies that record routine log and
// metric emissions for tests where diagnostics are the behavior under test.
func Capturing(level zapcore.Level) (Observability, Capture) {
	zapLogger, observed := CapturingZapLogger(level)
	recorder := NewRecordingMetricsEmitter()
	verbose := level <= zapcore.InfoLevel
	return Observability{
			Logger:         logging.NewZapLogger(zapLogger, verbose),
			MetricsEmitter: recorder,
			ZapLogger:      zapLogger,
		}, Capture{
			ObservedLogs: observed,
			Metrics:      recorder,
		}
}

// RecordingMetricsEmitter records metrics.MetricsEmitter emissions for test
// assertions.
type RecordingMetricsEmitter struct {
	mu       sync.Mutex
	counters []recordedCounter
	gauges   []recordedGauge
	samples  []recordedSample
}

type recordedCounter struct {
	name   string
	delta  float64
	fields metrics.Fields
}

type recordedGauge struct {
	name   string
	value  float64
	fields metrics.Fields
}

type recordedSample struct {
	name   string
	value  float64
	unit   string
	fields metrics.Fields
}

// NewRecordingMetricsEmitter returns an in-memory metrics emitter for tests
// that assert on expected metric samples.
func NewRecordingMetricsEmitter() *RecordingMetricsEmitter {
	return &RecordingMetricsEmitter{}
}

// Counter implements metrics.MetricsEmitter.
func (r *RecordingMetricsEmitter) Counter(_ context.Context, name string, delta float64, fields metrics.Fields) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters = append(r.counters, recordedCounter{name: name, delta: delta, fields: fields})
	return nil
}

// Gauge implements metrics.MetricsEmitter.
func (r *RecordingMetricsEmitter) Gauge(_ context.Context, name string, value float64, fields metrics.Fields) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges = append(r.gauges, recordedGauge{name: name, value: value, fields: fields})
	return nil
}

// Sample implements metrics.MetricsEmitter.
func (r *RecordingMetricsEmitter) Sample(_ context.Context, name string, value float64, unit string, fields metrics.Fields) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, recordedSample{name: name, value: value, unit: unit, fields: fields})
	return nil
}

// ContainsCounter reports whether a counter emission with the given name,
// delta, and fields was recorded.
func (r *RecordingMetricsEmitter) ContainsCounter(name string, delta float64, fields metrics.Fields) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, counter := range r.counters {
		if counter.name == name && counter.delta == delta && counter.fields == fields {
			return true
		}
	}
	return false
}

// ContainsSample reports whether a sample emission with the given name, value,
// unit, and fields was recorded.
func (r *RecordingMetricsEmitter) ContainsSample(name string, value float64, unit string, fields metrics.Fields) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sample := range r.samples {
		if sample.name == name && sample.value == value && sample.unit == unit && sample.fields == fields {
			return true
		}
	}
	return false
}

// RecordingInvocationMetrics records service.InvocationMetricsRecorder emissions
// for FactoryService observability tests.
type RecordingInvocationMetrics struct {
	mu      sync.Mutex
	metrics []service.InvocationMetric
}

// NewRecordingInvocationMetrics returns a recorder for
// FactoryServiceConfig.InvocationMetricsRecorder assertions.
func NewRecordingInvocationMetrics() *RecordingInvocationMetrics {
	return &RecordingInvocationMetrics{}
}

// RecordInvocationMetric implements service.InvocationMetricsRecorder.
func (r *RecordingInvocationMetrics) RecordInvocationMetric(metric service.InvocationMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	labels := make(map[string]string, len(metric.Labels))
	maps.Copy(labels, metric.Labels)
	r.metrics = append(r.metrics, service.InvocationMetric{
		Name:   metric.Name,
		Labels: labels,
	})
}

// Contains reports whether an invocation metric with the given name and labels
// was recorded.
func (r *RecordingInvocationMetrics) Contains(name string, labels map[string]string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, metric := range r.metrics {
		if metric.Name != name {
			continue
		}
		match := true
		for key, value := range labels {
			if metric.Labels[key] != value {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

var (
	_ metrics.MetricsEmitter            = (*RecordingMetricsEmitter)(nil)
	_ service.InvocationMetricsRecorder = (*RecordingInvocationMetrics)(nil)
)

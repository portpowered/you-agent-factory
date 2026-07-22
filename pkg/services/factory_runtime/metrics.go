package factory

import "context"

// Fields carries optional correlation metadata for a runtime metrics record.
// Concrete emitters should treat new fields as additive schema growth.
type Fields struct {
	DispatchID  string
	WorkID      string
	TraceID     string
	Workstation string
	WorkerType  string
	Provider    string
	Outcome     string
	Reason      string
}

// MetricsEmitter records logical factory runtime measurements without exposing
// sink, encoder, or path-layout details to runtime and worker packages.
type MetricsEmitter interface {
	// Counter records a monotonic increment for a named metric.
	Counter(ctx context.Context, name string, delta float64, fields Fields) error
	// Gauge records a point-in-time level for a named metric.
	Gauge(ctx context.Context, name string, value float64, fields Fields) error
	// Sample records a measured value for a named metric. Unit may be empty
	// when a metric-specific unit is not applicable.
	Sample(ctx context.Context, name string, value float64, unit string, fields Fields) error
}

// NoopEmitter safely discards all metric emissions.
type NoopEmitter struct{}

// Counter implements MetricsEmitter.
func (NoopEmitter) Counter(context.Context, string, float64, Fields) error {
	return nil
}

// Gauge implements MetricsEmitter.
func (NoopEmitter) Gauge(context.Context, string, float64, Fields) error {
	return nil
}

// Sample implements MetricsEmitter.
func (NoopEmitter) Sample(context.Context, string, float64, string, Fields) error {
	return nil
}

// EnsureEmitter returns emitter when provided, or a safe no-op emitter when
// metrics are disabled or unavailable.
func EnsureEmitter(emitter MetricsEmitter) MetricsEmitter {
	if emitter == nil {
		return NoopEmitter{}
	}
	return emitter
}

var _ MetricsEmitter = NoopEmitter{}

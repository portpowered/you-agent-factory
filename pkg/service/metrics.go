package service

// InvocationMetric records one emitted runtime counter together with its
// low-cardinality dimensions.
type InvocationMetric struct {
	Name   string
	Labels map[string]string
}

// InvocationMetricsRecorder receives invocation counter emissions from CLI and
// session-runtime boundaries. Implementations should treat each call as a
// single counter increment.
type InvocationMetricsRecorder interface {
	RecordInvocationMetric(InvocationMetric)
}

// ModelPullMetricsRecorder receives managed-runtime pull counter emissions.
// Implementations should treat each call as a single counter increment.
type ModelPullMetricsRecorder interface {
	RecordModelPullMetric(InvocationMetric)
}

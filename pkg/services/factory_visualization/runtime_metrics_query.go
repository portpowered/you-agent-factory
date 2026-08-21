package factory_visualization

import (
	"context"

	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
)

// RuntimeMetricsReader is the narrow Platform Metrics read capability
// consumed by the stateless query. The query owns metric interpretation; the
// reader only returns decoded JSON objects from bounded artifacts.
type RuntimeMetricsReader = platformmetrics.Reader

// RuntimeMetricRecord is the decoded, uninterpreted record supplied by the
// Platform Metrics reader.
type RuntimeMetricRecord = platformmetrics.RuntimeMetricRecord

// RuntimeMetricsQuery is the narrow, read-only Factory Visualization query
// capability over retained runtime metrics artifacts. It is a named operation
// rather than a second service-root interface, so composing this stateless
// read side does not require a runtime event source or presentation sink.
type RuntimeMetricsQuery func(context.Context, RuntimeMetricsQueryRequest) (RuntimeMetricsQueryResult, error)

// QueryRuntimeMetrics invokes the query operation through its descriptive
// method form. The named operation remains callable directly for small
// adapters while the method gives downstream consumers a stable contract.
func (query RuntimeMetricsQuery) QueryRuntimeMetrics(
	ctx context.Context,
	request RuntimeMetricsQueryRequest,
) (RuntimeMetricsQueryResult, error) {
	if query == nil {
		return RuntimeMetricsQueryResult{}, &RuntimeMetricsQueryError{
			Kind:    RuntimeMetricsQueryInvalidInput,
			Message: "query Factory Runtime metrics: operation is required",
		}
	}
	return query(ctx, request)
}

// RuntimeMetricsQueryRequest identifies the artifact root and optional runtime
// scope filters. When both identifiers are supplied they are intersected.
type RuntimeMetricsQueryRequest struct {
	MetricsRoot       string
	SessionID         string
	RuntimeInstanceID string
}

// RuntimeMetricsCostAvailability describes whether a query contains a cost
// calculation. Cost is intentionally unavailable for this read model.
type RuntimeMetricsCostAvailability string

const (
	// RuntimeMetricsCostUnavailable is the only cost state exposed by this
	// query. No price or numeric cost value is represented.
	RuntimeMetricsCostUnavailable RuntimeMetricsCostAvailability = "UNAVAILABLE"
)

// RuntimeMetricsCost is an explicit non-numeric cost representation.
type RuntimeMetricsCost struct {
	Availability RuntimeMetricsCostAvailability `json:"availability"`
}

// RuntimeMetricsDuration contains nearest-rank percentile samples. Nil P50 or
// P95 means that no sample was available; zero is preserved as a real sample.
type RuntimeMetricsDuration struct {
	Unit    string   `json:"unit,omitempty"`
	Samples int      `json:"samples"`
	P50     *float64 `json:"p50,omitempty"`
	P95     *float64 `json:"p95,omitempty"`
}

// RuntimeMetricsAggregate is the metric summary for one scope or group.
type RuntimeMetricsAggregate struct {
	InputTokens         float64                 `json:"input_tokens"`
	OutputTokens        float64                 `json:"output_tokens"`
	CompletedDispatches float64                 `json:"completed_dispatches"`
	FailuresByReason    map[string]float64      `json:"failures_by_reason,omitempty"`
	DispatchDuration    *RuntimeMetricsDuration `json:"dispatch_duration,omitempty"`
	ProviderDuration    *RuntimeMetricsDuration `json:"provider_duration,omitempty"`
}

// RuntimeMetricsBreakdown associates an aggregate with one non-empty
// workstation, worker-type, or provider label.
type RuntimeMetricsBreakdown struct {
	Key       string                  `json:"key"`
	Aggregate RuntimeMetricsAggregate `json:"aggregate"`
}

// RuntimeMetricsQueryResult is deterministic: each breakdown is sorted by
// Key, and each failure map is populated from the same filtered record set.
type RuntimeMetricsQueryResult struct {
	Cost         RuntimeMetricsCost        `json:"cost"`
	Totals       RuntimeMetricsAggregate   `json:"totals"`
	Workstations []RuntimeMetricsBreakdown `json:"workstations,omitempty"`
	WorkerTypes  []RuntimeMetricsBreakdown `json:"worker_types,omitempty"`
	Providers    []RuntimeMetricsBreakdown `json:"providers,omitempty"`
}

// RuntimeMetricsQueryErrorKind classifies query boundary failures.
type RuntimeMetricsQueryErrorKind string

const (
	RuntimeMetricsQueryInvalidInput RuntimeMetricsQueryErrorKind = "INVALID_INPUT"
	RuntimeMetricsQueryReadFailed   RuntimeMetricsQueryErrorKind = "READ_FAILED"
)

// RuntimeMetricsQueryError preserves a safe operation-level message and the
// underlying reader failure without including record payloads.
type RuntimeMetricsQueryError struct {
	Kind    RuntimeMetricsQueryErrorKind
	Message string
	Cause   error
}

func (e *RuntimeMetricsQueryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *RuntimeMetricsQueryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

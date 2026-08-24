package recordings

// RuntimeReadMetric is the small observation vocabulary used by bounded live
// reads. Recordings owns the canonical-history counters; the process-level
// recorder is supplied by the composition root.
type RuntimeReadMetric struct {
	Name   string
	Labels map[string]string
}

// RuntimeReadMetricsRecorder receives bounded runtime-read observations.
// It is optional so existing RuntimeLedger implementations and test doubles
// do not need to participate in process telemetry.
type RuntimeReadMetricsRecorder interface {
	RecordRuntimeReadMetric(RuntimeReadMetric)
}

// RuntimeReadMetricsBinder is implemented by ledgers that can forward their
// read counters to the process-level invocation metrics recorder.
type RuntimeReadMetricsBinder interface {
	SetRuntimeReadMetricsRecorder(RuntimeReadMetricsRecorder)
}

// CanonicalHistoryReadStats is a detached snapshot of canonical-history work
// observed by one runtime ledger.
type CanonicalHistoryReadStats struct {
	CanonicalEventsCalls  uint64
	CanonicalEventsCopied uint64
	FullHistoryReductions uint64
}

// CanonicalHistoryReadStatsReader exposes optional history-read counters to a
// bounded runtime observation without widening the RuntimeLedger contract.
type CanonicalHistoryReadStatsReader interface {
	CanonicalHistoryReadStats() CanonicalHistoryReadStats
}

// CanonicalHistoryReductionRecorder marks a full world-state reduction that
// starts from canonical history.
type CanonicalHistoryReductionRecorder interface {
	RecordCanonicalHistoryReduction()
}

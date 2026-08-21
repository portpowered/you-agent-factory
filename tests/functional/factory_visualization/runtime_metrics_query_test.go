package factory_visualization_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRuntimeMetricsQueryReadsDurableArtifactsThroughCanonicalProcess proves
// the read-only query crosses the public root boundary and reads every
// selected rolling artifact, including a gzip backup, without requiring a
// live runtime or transport endpoint. This query has no CLI/API surface by
// design, so the canonical process capability is the observable boundary.
func TestRuntimeMetricsQueryReadsDurableArtifactsThroughCanonicalProcess(t *testing.T) {
	t.Parallel()

	metricsRoot := t.TempDir()
	matchingSession := "session-metrics-functional"
	matchingRuntime := "runtime-metrics-functional"
	common := func(metricName string, value float64) map[string]any {
		return map[string]any{
			"metric_name":         metricName,
			"value":               value,
			"unit":                "ms",
			"session_id":          matchingSession,
			"runtime_instance_id": matchingRuntime,
			"workstation":         "workstation-a",
			"worker_type":         "worker-type-a",
			"provider":            "provider-a",
		}
	}

	writeRuntimeMetricsArtifact(t, filepath.Join(
		metricsRoot,
		"2026", "08", "20",
		"120000.000000000-runtime-metrics-session-metrics-functional-runtime-metrics-functional-2026-08-20T12-01-00.000.log",
	), false, []map[string]any{
		common(factoryruntime.RuntimeProviderInputTokens, 12),
	})

	writeRuntimeMetricsArtifact(t, filepath.Join(
		metricsRoot,
		"2026", "08", "20",
		"120000.000000000-runtime-metrics-session-metrics-functional-runtime-metrics-functional-2026-08-20T12-02-00.000-1.log.gz",
	), true, []map[string]any{
		common(factoryruntime.RuntimeProviderOutputTokens, 34),
		{
			"metric_name":         factoryruntime.RuntimeProviderInputTokens,
			"value":               900,
			"unit":                "tokens",
			"session_id":          "neighbor-session",
			"runtime_instance_id": matchingRuntime,
			"workstation":         "neighbor-workstation",
			"worker_type":         "neighbor-worker-type",
			"provider":            "neighbor-provider",
		},
	})

	activeRecords := []map[string]any{
		common(factoryruntime.RuntimeDispatchComplete, 1),
		common(factoryruntime.RuntimeDispatchDuration, 40),
		common(factoryruntime.RuntimeDispatchDuration, 60),
		common(factoryruntime.RuntimeProviderDuration, 20),
		common(factoryruntime.RuntimeProviderDuration, 80),
		{
			"metric_name":         factoryruntime.RuntimeProviderFailed,
			"value":               1,
			"unit":                "count",
			"reason":              "timeout",
			"session_id":          matchingSession,
			"runtime_instance_id": matchingRuntime,
			"workstation":         "workstation-a",
			"worker_type":         "worker-type-a",
			"provider":            "provider-a",
		},
	}
	writeRuntimeMetricsArtifact(t, filepath.Join(
		metricsRoot,
		"2026", "08", "20",
		"120000.000000000-runtime-metrics-session-metrics-functional-runtime-metrics-functional.log",
	), false, activeRecords, `{"metric_name":"provider.duration","value":`)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	query := root.RuntimeMetricsQueryFromProcess(process)
	if query == nil {
		t.Fatal("RuntimeMetricsQueryFromProcess() returned nil query")
	}

	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot:       metricsRoot,
		SessionID:         matchingSession,
		RuntimeInstanceID: matchingRuntime,
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}
	if result.Cost.Availability != factoryvisualization.RuntimeMetricsCostUnavailable {
		t.Fatalf("cost availability = %q, want %q", result.Cost.Availability, factoryvisualization.RuntimeMetricsCostUnavailable)
	}
	assertRuntimeMetricsAggregate(t, result.Totals, 12, 34, 1, 1, 40, 60, 20, 80)
	assertRuntimeMetricsBreakdown(t, result.Workstations, "workstation-a")
	assertRuntimeMetricsBreakdown(t, result.WorkerTypes, "worker-type-a")
	assertRuntimeMetricsBreakdown(t, result.Providers, "provider-a")
}

// TestRuntimeMetricsQueryReportsArtifactFailuresThroughCanonicalProcess proves
// the public query reports a durable read failure instead of returning a
// partial aggregate when the selected root or a complete record is invalid.
func TestRuntimeMetricsQueryReportsArtifactFailuresThroughCanonicalProcess(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	query := root.RuntimeMetricsQueryFromProcess(process)
	if query == nil {
		t.Fatal("RuntimeMetricsQueryFromProcess() returned nil query")
	}

	for name, metricsRoot := range map[string]string{
		"missing root":    filepath.Join(t.TempDir(), "missing"),
		"not a directory": writeRuntimeMetricsNonDirectory(t),
	} {
		_, err := query.QueryRuntimeMetrics(t.Context(), factoryvisualization.RuntimeMetricsQueryRequest{
			MetricsRoot: metricsRoot,
		})
		assertRuntimeMetricsReadFailure(t, name, err)
	}

	malformedRoot := t.TempDir()
	writeRuntimeMetricsArtifact(t, filepath.Join(
		malformedRoot,
		"120000.000000000-runtime-metrics-session-runtime-malformed.log",
	), false, nil, `{"metric_name": invalid}`+"\n")
	_, err := query.QueryRuntimeMetrics(t.Context(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: malformedRoot,
	})
	assertRuntimeMetricsReadFailure(t, "malformed complete record", err)

	invalidGzipRoot := t.TempDir()
	invalidGzipPath := filepath.Join(
		invalidGzipRoot,
		"120000.000000000-runtime-metrics-session-runtime-invalid-gzip-2026-08-20T12-03-00.000.log.gz",
	)
	if err := os.WriteFile(invalidGzipPath, []byte("not gzip"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", invalidGzipPath, err)
	}
	_, err = query.QueryRuntimeMetrics(t.Context(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: invalidGzipRoot,
	})
	assertRuntimeMetricsReadFailure(t, "invalid gzip artifact", err)
}

func assertRuntimeMetricsReadFailure(t *testing.T, name string, err error) {
	t.Helper()
	var queryErr *factoryvisualization.RuntimeMetricsQueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != factoryvisualization.RuntimeMetricsQueryReadFailed {
		t.Fatalf("%s error = %v, want READ_FAILED query error", name, err)
	}
}

func writeRuntimeMetricsNonDirectory(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics-root-file")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}

func assertRuntimeMetricsBreakdown(
	t *testing.T,
	breakdowns []factoryvisualization.RuntimeMetricsBreakdown,
	wantKey string,
) {
	t.Helper()
	if len(breakdowns) != 1 || breakdowns[0].Key != wantKey {
		t.Fatalf("breakdowns = %#v, want one %q breakdown", breakdowns, wantKey)
	}
	assertRuntimeMetricsAggregate(t, breakdowns[0].Aggregate, 12, 34, 1, 1, 40, 60, 20, 80)
}

func assertRuntimeMetricsAggregate(
	t *testing.T,
	aggregate factoryvisualization.RuntimeMetricsAggregate,
	wantInputTokens, wantOutputTokens, wantCompletedDispatches, wantFailures float64,
	wantDispatchP50, wantDispatchP95, wantProviderP50, wantProviderP95 float64,
) {
	t.Helper()
	if aggregate.InputTokens != wantInputTokens || aggregate.OutputTokens != wantOutputTokens || aggregate.CompletedDispatches != wantCompletedDispatches {
		t.Fatalf("aggregate totals = %#v, want input/output/completed = %v/%v/%v", aggregate, wantInputTokens, wantOutputTokens, wantCompletedDispatches)
	}
	if aggregate.FailuresByReason["timeout"] != wantFailures {
		t.Fatalf("failure distribution = %#v, want timeout=%v", aggregate.FailuresByReason, wantFailures)
	}
	assertDuration(t, aggregate.DispatchDuration, wantDispatchP50, wantDispatchP95, 2)
	assertDuration(t, aggregate.ProviderDuration, wantProviderP50, wantProviderP95, 2)
}

func assertDuration(t *testing.T, duration *factoryvisualization.RuntimeMetricsDuration, wantP50, wantP95 float64, wantSamples int) {
	t.Helper()
	if duration == nil || duration.Samples != wantSamples || duration.Unit != "ms" || duration.P50 == nil || duration.P95 == nil {
		t.Fatalf("duration = %#v, want %d ms samples with both percentiles", duration, wantSamples)
	}
	if *duration.P50 != wantP50 || *duration.P95 != wantP95 {
		t.Fatalf("duration percentiles = (%v, %v), want (%v, %v)", *duration.P50, *duration.P95, wantP50, wantP95)
	}
}

func writeRuntimeMetricsArtifact(
	t *testing.T,
	path string,
	compressed bool,
	records []map[string]any,
	tail ...string,
) {
	t.Helper()
	var content bytes.Buffer
	encoder := json.NewEncoder(&content)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("encode runtime metric: %v", err)
		}
	}
	if len(tail) > 0 {
		if _, err := io.WriteString(&content, tail[0]); err != nil {
			t.Fatalf("write torn runtime metric: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if !compressed {
		if err := os.WriteFile(path, content.Bytes(), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
		return
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%q): %v", path, err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(content.Bytes()); err != nil {
		_ = writer.Close()
		_ = file.Close()
		t.Fatalf("compress %q: %v", path, err)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("close gzip %q: %v", path, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close %q: %v", path, err)
	}
}

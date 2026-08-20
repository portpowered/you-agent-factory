package factory_visualization_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
)

func TestRuntimeMetricsQueryReadsAcrossActiveRotatedAndCompressedArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeMetricsJSONL(t, filepath.Join(root, "120000.000000000-runtime-metrics-session-a-runtime-a.log"), []factoryvisualization.RuntimeMetricRecord{
		metricRecord("dispatch.completed", 1, "session-a", "runtime-a", "ws", "worker", "provider", "", ""),
		metricRecord("provider.output_tokens", 3, "session-a", "runtime-a", "ws", "worker", "provider", "", "tokens"),
	}, "{\"metric_name\":\"provider.output_tokens\"")
	writeRuntimeMetricsJSONL(t, filepath.Join(root, "120001.000000000-runtime-metrics-session-a-runtime-a-2026-08-20T12-01-00.000.log"), []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 4, "session-a", "runtime-a", "ws", "worker", "provider", "", "tokens"),
		metricRecord("dispatch.duration", 10, "session-a", "runtime-a", "ws", "worker", "provider", "", "ms"),
	}, "")
	writeRuntimeMetricsGZIP(t, filepath.Join(root, "120002.000000000-runtime-metrics-session-a-runtime-a-2026-08-20T12-02-00.000.log.gz"), []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 6, "session-a", "runtime-a", "ws", "worker", "provider", "", "tokens"),
		metricRecord("provider.failed", 1, "session-a", "runtime-a", "ws", "worker", "provider", "timeout", ""),
		metricRecord("dispatch.duration", 30, "session-a", "runtime-a", "ws", "worker", "provider", "", "ms"),
		metricRecord("provider.duration", 20, "session-a", "runtime-a", "ws", "worker", "provider", "", "ms"),
	}, "")

	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(platformmetrics.NewRuntimeMetricsReader(), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}
	if result.Totals.InputTokens != 10 || result.Totals.OutputTokens != 3 || result.Totals.CompletedDispatches != 1 {
		t.Fatalf("totals = %#v, want input 10, output 3, dispatches 1", result.Totals)
	}
	if !reflect.DeepEqual(result.Totals.FailuresByReason, map[string]float64{"timeout": 1}) {
		t.Fatalf("failure totals = %#v", result.Totals.FailuresByReason)
	}
	assertDuration(t, result.Totals.DispatchDuration, 10, 30, 2, "ms")
	assertDuration(t, result.Totals.ProviderDuration, 20, 20, 1, "ms")
}

func TestRuntimeMetricsQueryAggregatesScopesAndBreakdownsDeterministically(t *testing.T) {
	t.Parallel()

	reader := &runtimeMetricsReaderStub{records: []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 10, "session-a", "runtime-a", "zeta", "model", "codex", "", ""),
		metricRecord("provider.output_tokens", 20, "session-a", "runtime-a", "zeta", "model", "codex", "", ""),
		metricRecord("dispatch.completed", 1, "session-a", "runtime-a", "zeta", "model", "codex", "", ""),
		metricRecord("dispatch.duration", 10, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("dispatch.duration", 20, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("dispatch.duration", 30, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("dispatch.duration", 40, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("dispatch.duration", 50, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("provider.duration", 5, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("provider.duration", 15, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("provider.duration", 25, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("provider.duration", 35, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("provider.duration", 45, "session-a", "runtime-a", "zeta", "model", "codex", "", "ms"),
		metricRecord("provider.failed", 1, "session-a", "runtime-a", "zeta", "model", "codex", "timeout", ""),
		metricRecord("provider.input_tokens", 3, "session-a", "runtime-b", "alpha", "script", "claude", "", ""),
		metricRecord("provider.output_tokens", 4, "session-a", "runtime-b", "alpha", "script", "claude", "", ""),
		metricRecord("dispatch.completed", 1, "session-a", "runtime-b", "alpha", "script", "claude", "", ""),
		metricRecord("provider.failed", 2, "session-a", "runtime-b", "alpha", "script", "claude", "provider_error", ""),
		metricRecord("provider.input_tokens", 100, "session-b", "runtime-c", "outside", "other", "other", "", ""),
		metricRecord("dispatch.cost", 999, "session-a", "runtime-a", "zeta", "model", "codex", "", "usd"),
		metricRecord("unknown.metric", 999, "session-a", "runtime-a", "zeta", "model", "codex", "", ""),
	}}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}

	all, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(all) error = %v", err)
	}
	assertRuntimeMetricsAggregateResult(t, all)
	assertRuntimeMetricsQueryDeterministic(t, query, reader, all)
}

func assertRuntimeMetricsAggregateResult(t *testing.T, result factoryvisualization.RuntimeMetricsQueryResult) {
	t.Helper()
	if result.Cost.Availability != factoryvisualization.RuntimeMetricsCostUnavailable {
		t.Fatalf("cost availability = %q, want unavailable", result.Cost.Availability)
	}
	if got := []float64{result.Totals.InputTokens, result.Totals.OutputTokens, result.Totals.CompletedDispatches}; !reflect.DeepEqual(got, []float64{113, 24, 2}) {
		t.Fatalf("totals = %#v, want input 113, output 24, dispatches 2", result.Totals)
	}
	if !reflect.DeepEqual(result.Totals.FailuresByReason, map[string]float64{"timeout": 1, "provider_error": 2}) {
		t.Fatalf("failure totals = %#v", result.Totals.FailuresByReason)
	}
	assertDuration(t, result.Totals.DispatchDuration, 30, 50, 5, "ms")
	assertDuration(t, result.Totals.ProviderDuration, 25, 45, 5, "ms")
	assertBreakdownKeys(t, result.Workstations, []string{"alpha", "outside", "zeta"})
	assertBreakdownInputs(t, result.Workstations, []float64{3, 100, 10})
	assertBreakdownKeys(t, result.WorkerTypes, []string{"model", "other", "script"})
	assertBreakdownKeys(t, result.Providers, []string{"claude", "codex", "other"})
	if result.Providers[1].Aggregate.CompletedDispatches != 1 {
		t.Fatalf("provider dispatch total = %#v, want dispatch label carried into provider group", result.Providers[1])
	}
}

func TestRuntimeMetricsQueryPartitionsRepeatedFailuresAndSkipsMissingDimensionLabels(t *testing.T) {
	t.Parallel()

	reader := &runtimeMetricsReaderStub{records: []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 5, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "", "tokens"),
		metricRecord("provider.output_tokens", 7, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "", "tokens"),
		metricRecord("dispatch.completed", 1, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "", ""),
		metricRecord("provider.failed", 1, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "timeout", ""),
		metricRecord("provider.failed", 2, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "timeout", ""),
		metricRecord("provider.failed", 3, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "quota", ""),
		metricRecord("provider.input_tokens", 11, "session-a", "runtime-a", "", "worker-b", "provider-b", "", "tokens"),
	}}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}

	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}
	if result.Totals.InputTokens != 16 || result.Totals.OutputTokens != 7 || result.Totals.CompletedDispatches != 1 {
		t.Fatalf("totals = %#v, want input 16, output 7, dispatches 1", result.Totals)
	}
	if !reflect.DeepEqual(result.Totals.FailuresByReason, map[string]float64{"quota": 3, "timeout": 3}) {
		t.Fatalf("failure totals = %#v, want repeated reasons partitioned", result.Totals.FailuresByReason)
	}
	assertBreakdownKeys(t, result.Workstations, []string{"workstation-a"})
	assertBreakdownKeys(t, result.WorkerTypes, []string{"worker-a", "worker-b"})
	assertBreakdownKeys(t, result.Providers, []string{"provider-a", "provider-b"})
	if result.Workstations[0].Aggregate.InputTokens != 5 {
		t.Fatalf("workstation aggregate = %#v, want missing workstation label omitted", result.Workstations[0])
	}
	if result.WorkerTypes[1].Aggregate.InputTokens != 11 || result.Providers[1].Aggregate.InputTokens != 11 {
		t.Fatalf("missing workstation record leaked incorrectly: workers=%#v providers=%#v", result.WorkerTypes, result.Providers)
	}
}

func assertBreakdownKeys(t *testing.T, breakdowns []factoryvisualization.RuntimeMetricsBreakdown, want []string) {
	t.Helper()
	got := make([]string, 0, len(breakdowns))
	for _, breakdown := range breakdowns {
		got = append(got, breakdown.Key)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("breakdown keys = %#v, want %#v", got, want)
	}
}

func assertBreakdownInputs(t *testing.T, breakdowns []factoryvisualization.RuntimeMetricsBreakdown, want []float64) {
	t.Helper()
	got := make([]float64, 0, len(breakdowns))
	for _, breakdown := range breakdowns {
		got = append(got, breakdown.Aggregate.InputTokens)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("breakdown input tokens = %#v, want %#v", got, want)
	}
}

func assertRuntimeMetricsQueryDeterministic(
	t *testing.T,
	query factoryvisualization.RuntimeMetricsQuery,
	reader *runtimeMetricsReaderStub,
	want factoryvisualization.RuntimeMetricsQueryResult,
) {
	t.Helper()
	repeated, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(repeated) error = %v", err)
	}
	if !reflect.DeepEqual(repeated, want) {
		t.Fatalf("repeated query = %#v, want deterministic result %#v", repeated, want)
	}
	if reader.calls != 2 {
		t.Fatalf("reader calls = %d, want one fresh read per query", reader.calls)
	}
}

func TestRuntimeMetricsQueryAppliesIndependentAndCombinedFilters(t *testing.T) {
	t.Parallel()

	reader := &runtimeMetricsReaderStub{records: []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 10, "session-a", "runtime-a", "one", "worker-a", "provider-a", "", ""),
		metricRecord("provider.input_tokens", 20, "session-a", "runtime-b", "two", "worker-b", "provider-b", "", ""),
		metricRecord("provider.input_tokens", 30, "session-b", "runtime-a", "three", "worker-c", "provider-c", "", ""),
	}}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	root := t.TempDir()
	checks := []struct {
		name            string
		request         factoryvisualization.RuntimeMetricsQueryRequest
		wantInputTokens float64
		wantGroups      int
	}{
		{name: "session", request: factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, SessionID: "session-a"}, wantInputTokens: 30, wantGroups: 2},
		{name: "runtime", request: factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, RuntimeInstanceID: "runtime-a"}, wantInputTokens: 40, wantGroups: 2},
		{name: "intersection", request: factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, SessionID: "session-a", RuntimeInstanceID: "runtime-a"}, wantInputTokens: 10, wantGroups: 1},
	}
	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			result, err := query.QueryRuntimeMetrics(context.Background(), check.request)
			if err != nil {
				t.Fatalf("QueryRuntimeMetrics() error = %v", err)
			}
			if result.Totals.InputTokens != check.wantInputTokens {
				t.Fatalf("input tokens = %v, want %v", result.Totals.InputTokens, check.wantInputTokens)
			}
			if len(result.Workstations) != check.wantGroups {
				t.Fatalf("workstation groups = %d, want %d", len(result.Workstations), check.wantGroups)
			}
		})
	}
}

func TestRuntimeMetricsQueryLeavesEmptyDurationsAbsentAndReportsReaderFailuresSafely(t *testing.T) {
	t.Parallel()

	readerErr := errors.New("artifact read failed")
	reader := &runtimeMetricsReaderStub{err: readerErr}
	logger := &queryLogger{}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logger)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	_, err = query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
	if !errors.Is(err, readerErr) {
		t.Fatalf("query error = %v, want reader failure %v", err, readerErr)
	}
	for _, message := range logger.messages {
		if strings.Contains(message, "secret-payload") {
			t.Fatalf("query log leaked record payload: %q", message)
		}
	}

	emptyReader := &runtimeMetricsReaderStub{records: []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 1, "other-session", "other-runtime", "", "", "", "", ""),
	}}
	emptyQuery, err := factoryvisualizationwire.NewRuntimeMetricsQuery(emptyReader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery(empty) error = %v", err)
	}
	empty, err := emptyQuery.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: t.TempDir(), SessionID: "missing-session",
	})
	if err != nil {
		t.Fatalf("empty query error = %v", err)
	}
	if empty.Totals.DispatchDuration != nil || empty.Totals.ProviderDuration != nil || len(empty.Workstations) != 0 {
		t.Fatalf("empty query = %#v, want absent durations and groups", empty)
	}
}

func TestRuntimeMetricsQueryValidatesReaderAndRoot(t *testing.T) {
	t.Parallel()

	if query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(nil, logging.NoopLogger{}); query != nil || err == nil {
		t.Fatalf("NewRuntimeMetricsQuery(nil) = (%v, %v), want construction failure", query, err)
	}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(&runtimeMetricsReaderStub{}, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	_, err = query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{})
	var queryErr *factoryvisualization.RuntimeMetricsQueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != factoryvisualization.RuntimeMetricsQueryInvalidInput {
		t.Fatalf("missing root error = %v, want typed invalid input", err)
	}
}

func writeRuntimeMetricsJSONL(
	t *testing.T,
	path string,
	records []factoryvisualization.RuntimeMetricRecord,
	tornTail string,
) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create metrics artifact %q: %v", path, err)
	}
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			_ = file.Close()
			t.Fatalf("encode metrics artifact %q: %v", path, err)
		}
	}
	if tornTail != "" {
		if _, err := file.WriteString(tornTail); err != nil {
			_ = file.Close()
			t.Fatalf("write torn metrics tail %q: %v", path, err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close metrics artifact %q: %v", path, err)
	}
}

func writeRuntimeMetricsGZIP(
	t *testing.T,
	path string,
	records []factoryvisualization.RuntimeMetricRecord,
	tornTail string,
) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create compressed metrics artifact %q: %v", path, err)
	}
	compressed := gzip.NewWriter(file)
	encoder := json.NewEncoder(compressed)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			_ = compressed.Close()
			_ = file.Close()
			t.Fatalf("encode compressed metrics artifact %q: %v", path, err)
		}
	}
	if tornTail != "" {
		if _, err := compressed.Write([]byte(tornTail)); err != nil {
			_ = compressed.Close()
			_ = file.Close()
			t.Fatalf("write torn compressed metrics tail %q: %v", path, err)
		}
	}
	if err := compressed.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("close compressed metrics artifact %q: %v", path, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close metrics artifact %q: %v", path, err)
	}
}

func metricRecord(
	name string,
	value float64,
	sessionID string,
	runtimeID string,
	workstation string,
	workerType string,
	provider string,
	reason string,
	unit string,
) factoryvisualization.RuntimeMetricRecord {
	return factoryvisualization.RuntimeMetricRecord{
		"metric_name":         name,
		"value":               value,
		"session_id":          sessionID,
		"runtime_instance_id": runtimeID,
		"workstation":         workstation,
		"worker_type":         workerType,
		"provider":            provider,
		"reason":              reason,
		"unit":                unit,
	}
}

func assertDuration(t *testing.T, duration *factoryvisualization.RuntimeMetricsDuration, wantP50, wantP95 float64, wantSamples int, wantUnit string) {
	t.Helper()
	if duration == nil || duration.P50 == nil || duration.P95 == nil {
		t.Fatalf("duration = %#v, want populated percentiles", duration)
	}
	if *duration.P50 != wantP50 || *duration.P95 != wantP95 || duration.Samples != wantSamples || duration.Unit != wantUnit {
		t.Fatalf("duration = %#v, want p50=%v p95=%v samples=%d unit=%q", duration, wantP50, wantP95, wantSamples, wantUnit)
	}
}

type runtimeMetricsReaderStub struct {
	records []factoryvisualization.RuntimeMetricRecord
	err     error
	calls   int
}

func (r *runtimeMetricsReaderStub) Read(context.Context, string) ([]factoryvisualization.RuntimeMetricRecord, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return append([]factoryvisualization.RuntimeMetricRecord(nil), r.records...), nil
}

type queryLogger struct {
	messages []string
}

func (l *queryLogger) Debug(message string, _ ...any)   { l.messages = append(l.messages, message) }
func (l *queryLogger) Info(message string, _ ...any)    { l.messages = append(l.messages, message) }
func (l *queryLogger) Warn(message string, _ ...any)    { l.messages = append(l.messages, message) }
func (l *queryLogger) Error(message string, _ ...any)   { l.messages = append(l.messages, message) }
func (l *queryLogger) Verbose(message string, _ ...any) { l.messages = append(l.messages, message) }

var _ logging.Logger = (*queryLogger)(nil)

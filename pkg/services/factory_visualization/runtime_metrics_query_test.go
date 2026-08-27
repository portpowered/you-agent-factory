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

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
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

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
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

func TestRuntimeMetricsQueryProviderProjectionSkipsUsageFamiliesAndOtherDimensions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeMetricsJSONL(t, filepath.Join(root, "120000.000000000-runtime-metrics-session-a-runtime-a.log"), []factoryvisualization.RuntimeMetricRecord{
		usageMetricRecord("provider.input_tokens", 4, "session-a", "runtime-a", "work-a", "dispatch-a", "worker-session-a", "provider-a", "model-a"),
		usageMetricRecord("provider.cached_input_tokens", 2, "session-a", "runtime-a", "work-a", "dispatch-a", "worker-session-a", "provider-a", "model-a"),
		usageMetricRecord("provider.reasoning_output_tokens", 1, "session-a", "runtime-a", "work-a", "dispatch-a", "worker-session-a", "provider-a", "model-a"),
		metricRecord("dispatch.completed", 1, "session-a", "runtime-a", "work-a", "worker-a", "provider-a", "", "count"),
	}, "")
	query := newRuntimeMetricsQueryForTest(t)
	provider := queryRuntimeMetricsForGroup(t, query, root, factoryvisualization.RuntimeMetricsGroupByProvider)
	assertProviderMetricsProjection(t, provider)
	repeatedProvider := queryRuntimeMetricsForGroup(t, query, root, factoryvisualization.RuntimeMetricsGroupByProvider)
	if !reflect.DeepEqual(repeatedProvider, provider) {
		t.Fatalf("provider projection is not deterministic: first=%#v repeated=%#v", provider, repeatedProvider)
	}
	all := queryRuntimeMetricsForGroup(t, query, root, "")
	if len(all.UsageRows) != 1 || all.UsageRows[0].CachedInputTokens == nil || *all.UsageRows[0].CachedInputTokens != 2 {
		t.Fatalf("all projection usage rows = %#v, want cached-input usage preserved", all.UsageRows)
	}
}

func newRuntimeMetricsQueryForTest(t *testing.T) factoryvisualization.RuntimeMetricsQuery {
	t.Helper()
	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	return query
}

func queryRuntimeMetricsForGroup(
	t *testing.T,
	query factoryvisualization.RuntimeMetricsQuery,
	root string,
	groupBy string,
) factoryvisualization.RuntimeMetricsQueryResult {
	t.Helper()
	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: root,
		GroupBy:     groupBy,
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(%q) error = %v", groupBy, err)
	}
	return result
}

func assertProviderMetricsProjection(t *testing.T, provider factoryvisualization.RuntimeMetricsQueryResult) {
	t.Helper()
	if provider.Totals.InputTokens != 4 || provider.Totals.CompletedDispatches != 1 {
		t.Fatalf("provider totals = %#v, want input 4 and one completed dispatch", provider.Totals)
	}
	if len(provider.Providers) != 1 || provider.Providers[0].Key != "provider-a" {
		t.Fatalf("provider breakdown = %#v, want provider-a only", provider.Providers)
	}
	if len(provider.Workstations) != 0 || len(provider.WorkerTypes) != 0 || len(provider.UsageRows) != 0 {
		t.Fatalf("provider projection retained irrelevant output: workstations=%#v workers=%#v usage=%#v", provider.Workstations, provider.WorkerTypes, provider.UsageRows)
	}
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
		{name: "resolved session candidates", request: factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, SessionID: "public-live-id", SessionIDs: []string{"session-a", "session-b"}}, wantInputTokens: 60, wantGroups: 3},
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

func TestRuntimeMetricsQueryFiltersAcrossActiveRotatedAndCompressedScopes(t *testing.T) {
	t.Parallel()

	root := installScopedMetricsFixture(t)

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	cases := []struct {
		name             string
		request          factoryvisualization.RuntimeMetricsQueryRequest
		wantInputTokens  float64
		wantWorkstations []string
		wantDispatchP50  float64
		wantDispatchP95  float64
		wantProviderP50  float64
		wantProviderP95  float64
	}{
		{
			name:             "all scopes",
			request:          factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root},
			wantInputTokens:  7,
			wantWorkstations: []string{"active", "compressed", "rotated"},
			wantDispatchP50:  20,
			wantDispatchP95:  40,
			wantProviderP50:  15,
			wantProviderP95:  35,
		},
		{
			name:             "session filter",
			request:          factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, SessionID: "session-a"},
			wantInputTokens:  3,
			wantWorkstations: []string{"active", "rotated"},
			wantDispatchP50:  10,
			wantDispatchP95:  20,
			wantProviderP50:  5,
			wantProviderP95:  15,
		},
		{
			name:             "runtime filter",
			request:          factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, RuntimeInstanceID: "runtime-a"},
			wantInputTokens:  5,
			wantWorkstations: []string{"active", "compressed"},
			wantDispatchP50:  10,
			wantDispatchP95:  40,
			wantProviderP50:  5,
			wantProviderP95:  35,
		},
		{
			name:             "combined intersection",
			request:          factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, SessionID: "session-a", RuntimeInstanceID: "runtime-a"},
			wantInputTokens:  1,
			wantWorkstations: []string{"active"},
			wantDispatchP50:  10,
			wantDispatchP95:  10,
			wantProviderP50:  5,
			wantProviderP95:  5,
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			result, err := query.QueryRuntimeMetrics(context.Background(), testCase.request)
			if err != nil {
				t.Fatalf("QueryRuntimeMetrics() error = %v", err)
			}
			if result.Totals.InputTokens != testCase.wantInputTokens {
				t.Fatalf("input tokens = %v, want %v", result.Totals.InputTokens, testCase.wantInputTokens)
			}
			assertBreakdownKeys(t, result.Workstations, testCase.wantWorkstations)
			assertDuration(t, result.Totals.DispatchDuration, testCase.wantDispatchP50, testCase.wantDispatchP95, len(testCase.wantWorkstations), "ms")
			assertDuration(t, result.Totals.ProviderDuration, testCase.wantProviderP50, testCase.wantProviderP95, len(testCase.wantWorkstations), "ms")
		})
	}

	noMatch, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: root, SessionID: "missing-session",
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(no match) error = %v", err)
	}
	assertEmptyRuntimeMetricsResult(t, noMatch)
}

func assertEmptyRuntimeMetricsResult(t *testing.T, result factoryvisualization.RuntimeMetricsQueryResult) {
	t.Helper()
	if result.Totals.InputTokens != 0 || result.Totals.DispatchDuration != nil || result.Totals.ProviderDuration != nil ||
		len(result.Workstations) != 0 || len(result.WorkerTypes) != 0 || len(result.Providers) != 0 {
		t.Fatalf("no-match result = %#v, want an empty scoped result", result)
	}
}

func installScopedMetricsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRuntimeMetricsJSONL(t, filepath.Join(root, "120000.000000000-runtime-metrics-session-a-runtime-a.log"), []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 1, "session-a", "runtime-a", "active", "worker-a", "provider-a", "", "tokens"),
		metricRecord("dispatch.duration", 10, "session-a", "runtime-a", "active", "worker-a", "provider-a", "", "ms"),
		metricRecord("provider.duration", 5, "session-a", "runtime-a", "active", "worker-a", "provider-a", "", "ms"),
	}, "")
	writeRuntimeMetricsJSONL(t, filepath.Join(root, "120001.000000000-runtime-metrics-session-a-runtime-b-2026-08-20T12-01-00.000.log"), []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 2, "session-a", "runtime-b", "rotated", "worker-b", "provider-b", "", "tokens"),
		metricRecord("dispatch.duration", 20, "session-a", "runtime-b", "rotated", "worker-b", "provider-b", "", "ms"),
		metricRecord("provider.duration", 15, "session-a", "runtime-b", "rotated", "worker-b", "provider-b", "", "ms"),
	}, "")
	writeRuntimeMetricsGZIP(t, filepath.Join(root, "120002.000000000-runtime-metrics-session-b-runtime-a-2026-08-20T12-02-00.000.log.gz"), []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 4, "session-b", "runtime-a", "compressed", "worker-c", "provider-c", "", "tokens"),
		metricRecord("dispatch.duration", 40, "session-b", "runtime-a", "compressed", "worker-c", "provider-c", "", "ms"),
		metricRecord("provider.duration", 35, "session-b", "runtime-a", "compressed", "worker-c", "provider-c", "", "ms"),
	}, "")
	return root
}

func TestRuntimeMetricsQueryCalculatesIndependentNearestRankPercentilesPerDimension(t *testing.T) {
	t.Parallel()

	reader := &runtimeMetricsReaderStub{}
	addDurations := func(label string, dispatch, provider []float64) {
		for _, value := range dispatch {
			reader.records = append(reader.records, metricRecord("dispatch.duration", value, "session-a", "runtime-a", label, label, label, "", "ms"))
		}
		for _, value := range provider {
			reader.records = append(reader.records, metricRecord("provider.duration", value, "session-a", "runtime-a", label, label, label, "", "ms"))
		}
	}
	addDurations("even", []float64{10, 20, 30, 40}, []float64{5})
	addDurations("repeated", []float64{1, 1, 2}, []float64{7, 7, 9})
	addDurations("dispatch-only", []float64{4, 8}, nil)
	addDurations("provider-only", nil, []float64{6, 10})

	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}

	for _, breakdowns := range []struct {
		name string
		data []factoryvisualization.RuntimeMetricsBreakdown
	}{
		{name: "workstation", data: result.Workstations},
		{name: "worker type", data: result.WorkerTypes},
		{name: "provider", data: result.Providers},
	} {
		breakdowns := breakdowns
		t.Run(breakdowns.name, func(t *testing.T) {
			even := breakdownForKey(t, breakdowns.data, "even")
			assertDuration(t, even.DispatchDuration, 20, 40, 4, "ms")
			assertDuration(t, even.ProviderDuration, 5, 5, 1, "ms")

			repeated := breakdownForKey(t, breakdowns.data, "repeated")
			assertDuration(t, repeated.DispatchDuration, 1, 2, 3, "ms")
			assertDuration(t, repeated.ProviderDuration, 7, 9, 3, "ms")

			dispatchOnly := breakdownForKey(t, breakdowns.data, "dispatch-only")
			assertDuration(t, dispatchOnly.DispatchDuration, 4, 8, 2, "ms")
			if dispatchOnly.ProviderDuration != nil {
				t.Fatalf("dispatch-only provider duration = %#v, want unavailable", dispatchOnly.ProviderDuration)
			}

			providerOnly := breakdownForKey(t, breakdowns.data, "provider-only")
			if providerOnly.DispatchDuration != nil {
				t.Fatalf("provider-only dispatch duration = %#v, want unavailable", providerOnly.DispatchDuration)
			}
			assertDuration(t, providerOnly.ProviderDuration, 6, 10, 2, "ms")
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

func TestRuntimeMetricsQueryDiscardsPartialAggregateAfterStreamFailure(t *testing.T) {
	t.Parallel()

	streamErr := errors.New("artifact stream stopped")
	reader := &runtimeMetricsReaderStub{
		records: []factoryvisualization.RuntimeMetricRecord{
			metricRecord("provider.input_tokens", 9, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "", "tokens"),
		},
		streamErr:      streamErr,
		streamErrAfter: 1,
	}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}

	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
	if err == nil || !errors.Is(err, streamErr) {
		t.Fatalf("QueryRuntimeMetrics() error = %v, want stream failure %v", err, streamErr)
	}
	var queryErr *factoryvisualization.RuntimeMetricsQueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != factoryvisualization.RuntimeMetricsQueryReadFailed {
		t.Fatalf("QueryRuntimeMetrics() error = %v, want typed read failure", err)
	}
	if !reflect.DeepEqual(result, factoryvisualization.RuntimeMetricsQueryResult{}) {
		t.Fatalf("partial query result = %#v, want zero result on stream failure", result)
	}

	cancellationReader := &runtimeMetricsReaderStub{streamErr: context.Canceled}
	cancellationQuery, err := factoryvisualizationwire.NewRuntimeMetricsQuery(cancellationReader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery(cancellation) error = %v", err)
	}
	_, err = cancellationQuery.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("QueryRuntimeMetrics() error = %v, want context.Canceled", err)
	}
	var cancellationQueryErr *factoryvisualization.RuntimeMetricsQueryError
	if errors.As(err, &cancellationQueryErr) {
		t.Fatalf("QueryRuntimeMetrics() error = %v, want cancellation without a read-failure wrapper", err)
	}
}

func assertRuntimeMetricsQueryPrefersIncrementalReaderCapability(t *testing.T) {
	t.Helper()
	reader := &runtimeMetricsReaderStub{records: []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 7, "session-a", "runtime-a", "workstation-a", "worker-a", "provider-a", "", "tokens"),
	}}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}
	if result.Totals.InputTokens != 7 {
		t.Fatalf("query totals = %#v, want input tokens 7", result.Totals)
	}
	if reader.streamCalls != 1 || reader.readCalls != 0 {
		t.Fatalf("reader calls = stream %d, read %d; want one streaming call and no collecting read", reader.streamCalls, reader.readCalls)
	}
	if reader.maxLiveRecords != 1 {
		t.Fatalf("peak records crossing query boundary = %d, want one", reader.maxLiveRecords)
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

func TestRuntimeMetricsQueryFallsBackToReadOnlyReader(t *testing.T) {
	t.Parallel()

	reader := &readOnlyRuntimeMetricsReaderStub{records: []factoryvisualization.RuntimeMetricRecord{
		metricRecord("provider.input_tokens", 4, "session-a", "runtime-a", "work-a", "worker-a", "provider-a", "", "tokens"),
		metricRecord("provider.input_tokens", 9, "session-b", "runtime-b", "work-b", "worker-b", "provider-b", "", "tokens"),
	}}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}

	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: "legacy-reader-root",
		SessionID:   "session-a",
		GroupBy:     factoryvisualization.RuntimeMetricsGroupByProvider,
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("Read calls = %d, want one", reader.calls)
	}
	if result.Totals.InputTokens != 4 || len(result.Providers) != 1 || result.Providers[0].Key != "provider-a" {
		t.Fatalf("legacy reader result = %#v, want session-a/provider-a only", result)
	}
	assertRuntimeMetricsQueryPrefersIncrementalReaderCapability(t)
}

func TestRuntimeMetricsQueryBuildsDeterministicCorrelatedUsageRows(t *testing.T) {
	t.Parallel()

	reader := &runtimeMetricsReaderStub{records: []factoryvisualization.RuntimeMetricRecord{
		usageMetricRecord("provider.input_tokens", 10, "session-a", "runtime-a", "work-a", "dispatch-a", "worker-session-a", "provider-a", "model-a"),
		usageMetricRecord("provider.output_tokens", 6, "session-a", "runtime-a", "work-a", "dispatch-a", "worker-session-a", "provider-a", "model-a"),
		usageMetricRecord("provider.cached_input_tokens", 3, "session-a", "runtime-a", "work-a", "dispatch-a", "worker-session-a", "provider-a", "model-a"),
		usageMetricRecord("provider.reasoning_output_tokens", 2, "session-a", "runtime-a", "work-a", "dispatch-a", "worker-session-a", "provider-a", "model-a"),
		usageMetricRecord("provider.input_tokens", 0, "session-b", "runtime-b", "work-b", "dispatch-b", "worker-session-b", "provider-b", "model-b"),
		usageMetricRecord("provider.output_tokens", 4, "session-b", "runtime-b", "work-b", "dispatch-b", "worker-session-b", "provider-b", "model-b"),
		usageMetricRecord("provider.cached_input_tokens", 0, "session-b", "runtime-b", "work-b", "dispatch-b", "worker-session-b", "provider-b", "model-b"),
		usageMetricRecord("provider.reasoning_output_tokens", 1, "session-b", "runtime-b", "work-b", "dispatch-b", "worker-session-b", "provider-b", "model-b"),
		usageMetricRecord("provider.input_tokens", 2, "session-a", "runtime-a", "", "", "", "", ""),
		usageMetricRecord("provider.input_tokens", 99, "session-outside", "runtime-outside", "work-outside", "dispatch-outside", "worker-outside", "provider-outside", "model-outside"),
	}}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}

	all, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(all) error = %v", err)
	}
	if len(all.UsageRows) != 4 {
		t.Fatalf("usage rows = %#v, want four correlated rows", all.UsageRows)
	}
	assertLegacyUsageRow(t, all.UsageRows[0])
	assertFullUsageRow(t, all.UsageRows[1])
	assertZeroUsageRow(t, all.UsageRows[2])

	sessionA, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: t.TempDir(), SessionID: "session-a",
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(session-a) error = %v", err)
	}
	if len(sessionA.UsageRows) != 2 || sessionA.Totals.InputTokens != 12 || sessionA.Totals.OutputTokens != 6 {
		t.Fatalf("session-a result = %#v, want isolated usage rows and totals", sessionA)
	}

	runtimeB, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: t.TempDir(), RuntimeInstanceID: "runtime-b",
	})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(runtime-b) error = %v", err)
	}
	if len(runtimeB.UsageRows) != 1 || runtimeB.UsageRows[0].DispatchID != "dispatch-b" {
		t.Fatalf("runtime-b rows = %#v, want only dispatch-b", runtimeB.UsageRows)
	}
}

func assertLegacyUsageRow(t *testing.T, row factoryvisualization.RuntimeMetricsUsageRow) {
	t.Helper()
	if row.FactorySessionID != "session-a" || row.InputTokens == nil || *row.InputTokens != 2 {
		t.Fatalf("legacy usage row = %#v, want session-a input 2", row)
	}
	if row.WorkID != "" || row.DispatchID != "" || row.WorkerSessionID != "" || row.Provider != "" || row.Model != "" {
		t.Fatalf("legacy usage row = %#v, want identity absence preserved", row)
	}
}

func assertFullUsageRow(t *testing.T, row factoryvisualization.RuntimeMetricsUsageRow) {
	t.Helper()
	if row.FactorySessionID != "session-a" || row.WorkID != "work-a" || row.DispatchID != "dispatch-a" ||
		row.WorkerSessionID != "worker-session-a" || row.Provider != "provider-a" || row.Model != "model-a" {
		t.Fatalf("full usage row identity = %#v, want correlated identity", row)
	}
	if row.InputTokens == nil || *row.InputTokens != 10 || row.OutputTokens == nil || *row.OutputTokens != 6 {
		t.Fatalf("full usage row totals = %#v, want input 10 and output 6", row)
	}
	if row.CachedInputTokens == nil || *row.CachedInputTokens != 3 || row.ReasoningOutputTokens == nil || *row.ReasoningOutputTokens != 2 {
		t.Fatalf("full usage row subclasses = %#v, want cached 3 and reasoning 2", row)
	}
}

func assertZeroUsageRow(t *testing.T, row factoryvisualization.RuntimeMetricsUsageRow) {
	t.Helper()
	if row.InputTokens == nil || *row.InputTokens != 0 || row.OutputTokens == nil || *row.OutputTokens != 4 {
		t.Fatalf("zero usage row totals = %#v, want input zero and output 4", row)
	}
	if row.CachedInputTokens == nil || *row.CachedInputTokens != 0 || row.ReasoningOutputTokens == nil || *row.ReasoningOutputTokens != 1 {
		t.Fatalf("zero usage row subclasses = %#v, want cached zero and reasoning 1", row)
	}
}

func TestRuntimeMetricsQueryRejectsMalformedUsageSubclasses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		records []factoryvisualization.RuntimeMetricRecord
	}{
		{
			name: "cached exceeds input",
			records: []factoryvisualization.RuntimeMetricRecord{
				usageMetricRecord("provider.input_tokens", 2, "session", "runtime", "work", "dispatch", "worker", "provider", "model"),
				usageMetricRecord("provider.cached_input_tokens", 3, "session", "runtime", "work", "dispatch", "worker", "provider", "model"),
			},
		},
		{
			name: "fractional token value",
			records: []factoryvisualization.RuntimeMetricRecord{
				usageMetricRecord("provider.input_tokens", 1.5, "session", "runtime", "work", "dispatch", "worker", "provider", "model"),
			},
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			reader := &runtimeMetricsReaderStub{records: testCase.records}
			query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
			if err != nil {
				t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
			}
			_, err = query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()})
			var queryErr *factoryvisualization.RuntimeMetricsQueryError
			if !errors.As(err, &queryErr) || queryErr.Kind != factoryvisualization.RuntimeMetricsQueryInvalidUsage {
				t.Fatalf("query error = %v, want typed invalid usage", err)
			}
		})
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

func usageMetricRecord(
	name string,
	value float64,
	sessionID string,
	runtimeID string,
	workID string,
	dispatchID string,
	workerSessionID string,
	provider string,
	model string,
) factoryvisualization.RuntimeMetricRecord {
	record := metricRecord(name, value, sessionID, runtimeID, "", "", provider, "", "tokens")
	record["work_id"] = workID
	record["dispatch_id"] = dispatchID
	record["worker_session_id"] = workerSessionID
	record["model"] = model
	return record
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
	records        []factoryvisualization.RuntimeMetricRecord
	err            error
	streamErr      error
	streamErrAfter int
	calls          int
	readCalls      int
	streamCalls    int
	liveRecords    int
	maxLiveRecords int
}

func (r *runtimeMetricsReaderStub) Read(ctx context.Context, _ string) ([]factoryvisualization.RuntimeMetricRecord, error) {
	r.calls++
	r.readCalls++
	if r.err != nil {
		return nil, r.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]factoryvisualization.RuntimeMetricRecord(nil), r.records...), nil
}

func (r *runtimeMetricsReaderStub) Stream(ctx context.Context, _ string, visit func(factoryvisualization.RuntimeMetricRecord) error) error {
	r.calls++
	r.streamCalls++
	if r.err != nil {
		return r.err
	}
	for index, record := range r.records {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.liveRecords++
		if r.liveRecords > r.maxLiveRecords {
			r.maxLiveRecords = r.liveRecords
		}
		err := visit(record)
		r.liveRecords--
		if err != nil {
			return err
		}
		if r.streamErr != nil && index+1 >= r.streamErrAfter {
			return r.streamErr
		}
	}
	if r.streamErr != nil {
		return r.streamErr
	}
	return nil
}

type readOnlyRuntimeMetricsReaderStub struct {
	records []factoryvisualization.RuntimeMetricRecord
	calls   int
}

func (r *readOnlyRuntimeMetricsReaderStub) Read(context.Context, string) ([]factoryvisualization.RuntimeMetricRecord, error) {
	r.calls++
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

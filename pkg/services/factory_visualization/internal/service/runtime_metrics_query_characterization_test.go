package service_test

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

func TestRuntimeMetricsQueryCharacterizesFixedArtifactCorpus(t *testing.T) {
	t.Parallel()

	root := installFixedCharacterizationCorpus(t)

	query := newFixedCharacterizationQuery(t)

	got, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root})
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}
	want := fixedCharacterizationResult()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRuntimeMetrics() = %#v, want %#v", got, want)
	}

	repeated, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root})
	if err != nil {
		t.Fatalf("repeated QueryRuntimeMetrics() error = %v", err)
	}
	if !reflect.DeepEqual(repeated, got) {
		t.Fatalf("repeated QueryRuntimeMetrics() = %#v, want deterministic result %#v", repeated, got)
	}

	assertFixedCharacterizationFilters(t, query, root)
}

func installFixedCharacterizationCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRuntimeMetricsJSONL(
		t,
		filepath.Join(root, "120000.000000000-runtime-metrics-session-a-runtime-a.log"),
		append(
			fixedCharacterizationRecords(
				"session-a", "runtime-a", "zeta", "model", "work-a", "dispatch-a", "worker-a", "model-a",
				[]characterizationMetricSpec{
					{name: "dispatch.completed", value: 1, provider: "codex"},
					{name: "provider.input_tokens", value: 10, provider: "${branchProvider}", unit: "tokens"},
					{name: "provider.output_tokens", value: 6, provider: "${branchProvider}", unit: "tokens"},
					{name: "provider.cached_input_tokens", value: 3, provider: "${branchProvider}", unit: "tokens"},
					{name: "provider.reasoning_output_tokens", value: 2, provider: "${branchProvider}", unit: "tokens"},
					{name: "provider.failed", value: 1, provider: "${plannerProvider}", reason: "timeout"},
					{name: "dispatch.duration", value: 10, provider: "${executorProvider}", unit: "ms"},
					{name: "provider.duration", value: 5, provider: "${reviewerProvider}", unit: "ms"},
				},
			),
			fixedCharacterizationRecords(
				"session-a", "runtime-a", "zeta", "model", "", "", "", "",
				[]characterizationMetricSpec{{name: "provider.input_tokens", value: 2, unit: "tokens"}},
			)...,
		),
		`{"metric_name":"provider.output_tokens"`,
	)
	writeRuntimeMetricsJSONL(
		t,
		filepath.Join(root, "120001.000000000-runtime-metrics-session-a-runtime-b-2026-08-20T12-01-00.000.log"),
		fixedCharacterizationRecords(
			"session-a", "runtime-b", "alpha", "script", "work-b", "dispatch-b", "worker-b", "model-b",
			[]characterizationMetricSpec{
				{name: "dispatch.completed", value: 1, provider: "claude"},
				{name: "provider.input_tokens", value: 4, provider: "${workerProvider}", unit: "tokens"},
				{name: "provider.output_tokens", value: 5, provider: "${workerProvider}", unit: "tokens"},
				{name: "provider.cached_input_tokens", value: 0, provider: "${workerProvider}", unit: "tokens"},
				{name: "provider.reasoning_output_tokens", value: 1, provider: "${workerProvider}", unit: "tokens"},
				{name: "provider.failed", value: 2, provider: "${workerProvider}", reason: "quota"},
				{name: "dispatch.duration", value: 20, provider: "${workerProvider}", unit: "ms"},
				{name: "provider.duration", value: 15, provider: "${workerProvider}", unit: "ms"},
			},
		),
		"",
	)
	writeRuntimeMetricsGZIP(
		t,
		filepath.Join(root, "120002.000000000-runtime-metrics-session-b-runtime-a-2026-08-20T12-02-00.000.log.gz"),
		fixedCharacterizationRecords(
			"session-b", "runtime-a", "beta", "agent", "work-c", "dispatch-c", "worker-c", "model-c",
			[]characterizationMetricSpec{
				{name: "dispatch.completed", value: 1},
				{name: "provider.input_tokens", value: 7, provider: "provider-b", unit: "tokens"},
				{name: "provider.output_tokens", value: 8, provider: "provider-b", unit: "tokens"},
				{name: "provider.cached_input_tokens", value: 2, provider: "provider-b", unit: "tokens"},
				{name: "provider.reasoning_output_tokens", value: 3, provider: "provider-b", unit: "tokens"},
				{name: "provider.failed", value: 1, provider: "provider-b", reason: "lost"},
				{name: "dispatch.duration", value: 40, provider: "provider-a", unit: "ms"},
				{name: "provider.duration", value: 35, provider: "provider-b", unit: "ms"},
			},
		),
		"",
	)
	return root
}

func newFixedCharacterizationQuery(t *testing.T) factoryvisualization.RuntimeMetricsQuery {
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

type characterizationScopeCase struct {
	name         string
	request      factoryvisualization.RuntimeMetricsQueryRequest
	inputTokens  float64
	outputTokens float64
	completed    float64
	workstations []string
	usageRows    int
	dispatchP50  float64
	dispatchP95  float64
	providerP50  float64
	providerP95  float64
}

func assertFixedCharacterizationFilters(t *testing.T, query factoryvisualization.RuntimeMetricsQuery, root string) {
	t.Helper()
	cases := []characterizationScopeCase{
		{
			name: "session filter", request: factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, SessionID: "session-a"},
			inputTokens: 16, outputTokens: 11, completed: 2, workstations: []string{"alpha", "zeta"}, usageRows: 3,
			dispatchP50: 10, dispatchP95: 20, providerP50: 5, providerP95: 15,
		},
		{
			name: "runtime filter", request: factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, RuntimeInstanceID: "runtime-a"},
			inputTokens: 19, outputTokens: 14, completed: 2, workstations: []string{"beta", "zeta"}, usageRows: 3,
			dispatchP50: 10, dispatchP95: 40, providerP50: 5, providerP95: 35,
		},
		{
			name: "combined filter", request: factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root, SessionID: "session-a", RuntimeInstanceID: "runtime-a"},
			inputTokens: 12, outputTokens: 6, completed: 1, workstations: []string{"zeta"}, usageRows: 2,
			dispatchP50: 10, dispatchP95: 10, providerP50: 5, providerP95: 5,
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			result, err := query.QueryRuntimeMetrics(context.Background(), testCase.request)
			if err != nil {
				t.Fatalf("QueryRuntimeMetrics() error = %v", err)
			}
			if result.Totals.InputTokens != testCase.inputTokens || result.Totals.OutputTokens != testCase.outputTokens || result.Totals.CompletedDispatches != testCase.completed {
				t.Fatalf("totals = %#v, want input %v, output %v, completed %v", result.Totals, testCase.inputTokens, testCase.outputTokens, testCase.completed)
			}
			assertBreakdownKeys(t, result.Workstations, testCase.workstations)
			if len(result.UsageRows) != testCase.usageRows {
				t.Fatalf("usage rows = %#v, want %d rows", result.UsageRows, testCase.usageRows)
			}
			assertDuration(t, result.Totals.DispatchDuration, testCase.dispatchP50, testCase.dispatchP95, len(testCase.workstations), "ms")
			assertDuration(t, result.Totals.ProviderDuration, testCase.providerP50, testCase.providerP95, len(testCase.workstations), "ms")
		})
	}
}

func TestRuntimeMetricsQueryCharacterizationRejectsMalformedCompleteLine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "120000.000000000-runtime-metrics-session-a-runtime-a.log")
	writeRuntimeMetricsJSONL(
		t,
		path,
		[]factoryvisualization.RuntimeMetricRecord{
			metricRecord("provider.input_tokens", 1, "session-a", "runtime-a", "zeta", "model", "codex", "", "tokens"),
		},
		"{\"metric_name\":\"provider.input_tokens\",\"value\":2,\"secret-payload\":\"do-not-leak\"\n",
	)

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}

	result, err := query.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: root})
	var queryErr *factoryvisualization.RuntimeMetricsQueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != factoryvisualization.RuntimeMetricsQueryReadFailed {
		t.Fatalf("QueryRuntimeMetrics() error = %v, want typed read failure", err)
	}
	if !reflect.DeepEqual(result, factoryvisualization.RuntimeMetricsQueryResult{}) {
		t.Fatalf("QueryRuntimeMetrics() result = %#v, want no partial result", result)
	}
	if strings.Contains(err.Error(), "secret-payload") || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("QueryRuntimeMetrics() error leaked record contents: %q", err)
	}
}

type characterizationMetricSpec struct {
	name     string
	value    float64
	provider string
	reason   string
	unit     string
}

func fixedCharacterizationRecords(
	sessionID, runtimeID, workstation, workerType, workID, dispatchID, workerSessionID, model string,
	specs []characterizationMetricSpec,
) []factoryvisualization.RuntimeMetricRecord {
	records := make([]factoryvisualization.RuntimeMetricRecord, 0, len(specs))
	for _, spec := range specs {
		record := metricRecord(spec.name, spec.value, sessionID, runtimeID, workstation, workerType, spec.provider, spec.reason, spec.unit)
		record["work_id"] = workID
		record["dispatch_id"] = dispatchID
		record["worker_session_id"] = workerSessionID
		record["model"] = model
		if workID == "" {
			delete(record, "work_id")
		}
		if dispatchID == "" {
			delete(record, "dispatch_id")
		}
		if workerSessionID == "" {
			delete(record, "worker_session_id")
		}
		if model == "" {
			delete(record, "model")
		}
		records = append(records, record)
	}
	return records
}

func fixedCharacterizationResult() factoryvisualization.RuntimeMetricsQueryResult {
	return factoryvisualization.RuntimeMetricsQueryResult{
		Cost: factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostUnavailable},
		Totals: characterizationAggregate(
			23, 19, 3,
			map[string]float64{"lost": 1, "quota": 2, "timeout": 1},
			characterizationDuration(20, 40, 3), characterizationDuration(15, 35, 3),
		),
		Workstations: []factoryvisualization.RuntimeMetricsBreakdown{
			{Key: "alpha", Aggregate: characterizationAggregate(4, 5, 1, map[string]float64{"quota": 2}, characterizationDuration(20, 20, 1), characterizationDuration(15, 15, 1))},
			{Key: "beta", Aggregate: characterizationAggregate(7, 8, 1, map[string]float64{"lost": 1}, characterizationDuration(40, 40, 1), characterizationDuration(35, 35, 1))},
			{Key: "zeta", Aggregate: characterizationAggregate(12, 6, 1, map[string]float64{"timeout": 1}, characterizationDuration(10, 10, 1), characterizationDuration(5, 5, 1))},
		},
		WorkerTypes: []factoryvisualization.RuntimeMetricsBreakdown{
			{Key: "agent", Aggregate: characterizationAggregate(7, 8, 1, map[string]float64{"lost": 1}, characterizationDuration(40, 40, 1), characterizationDuration(35, 35, 1))},
			{Key: "model", Aggregate: characterizationAggregate(12, 6, 1, map[string]float64{"timeout": 1}, characterizationDuration(10, 10, 1), characterizationDuration(5, 5, 1))},
			{Key: "script", Aggregate: characterizationAggregate(4, 5, 1, map[string]float64{"quota": 2}, characterizationDuration(20, 20, 1), characterizationDuration(15, 15, 1))},
		},
		Providers: []factoryvisualization.RuntimeMetricsBreakdown{
			{Key: "claude", Aggregate: characterizationAggregate(4, 5, 1, map[string]float64{"quota": 2}, characterizationDuration(20, 20, 1), characterizationDuration(15, 15, 1))},
			{Key: "codex", Aggregate: characterizationAggregate(10, 6, 1, map[string]float64{"timeout": 1}, characterizationDuration(10, 10, 1), characterizationDuration(5, 5, 1))},
			// Current main's provider projection preserves the unattributed
			// standalone usage fact under the stable unavailable key as well as
			// the provider-conflicted compressed dispatch.
			{Key: "unavailable", Aggregate: characterizationAggregate(9, 8, 1, map[string]float64{"lost": 1}, characterizationDuration(40, 40, 1), characterizationDuration(35, 35, 1))},
		},
		UsageRows: []factoryvisualization.RuntimeMetricsUsageRow{
			{FactorySessionID: "session-a", InputTokens: characterizationInt64(2)},
			{FactorySessionID: "session-a", WorkID: "work-a", DispatchID: "dispatch-a", WorkerSessionID: "worker-a", Provider: "codex", Model: "model-a", InputTokens: characterizationInt64(10), OutputTokens: characterizationInt64(6), CachedInputTokens: characterizationInt64(3), ReasoningOutputTokens: characterizationInt64(2)},
			{FactorySessionID: "session-a", WorkID: "work-b", DispatchID: "dispatch-b", WorkerSessionID: "worker-b", Provider: "claude", Model: "model-b", InputTokens: characterizationInt64(4), OutputTokens: characterizationInt64(5), CachedInputTokens: characterizationInt64(0), ReasoningOutputTokens: characterizationInt64(1)},
			{FactorySessionID: "session-b", WorkID: "work-c", DispatchID: "dispatch-c", WorkerSessionID: "worker-c", Provider: "unavailable", Model: "model-c", InputTokens: characterizationInt64(7), OutputTokens: characterizationInt64(8), CachedInputTokens: characterizationInt64(2), ReasoningOutputTokens: characterizationInt64(3)},
		},
	}
}

func characterizationAggregate(
	inputTokens, outputTokens, completedDispatches float64,
	failures map[string]float64,
	dispatchDuration, providerDuration *factoryvisualization.RuntimeMetricsDuration,
) factoryvisualization.RuntimeMetricsAggregate {
	return factoryvisualization.RuntimeMetricsAggregate{
		InputTokens: inputTokens, OutputTokens: outputTokens, CompletedDispatches: completedDispatches,
		FailuresByReason: failures, DispatchDuration: dispatchDuration, ProviderDuration: providerDuration,
	}
}

func characterizationDuration(p50, p95 float64, samples int) *factoryvisualization.RuntimeMetricsDuration {
	return &factoryvisualization.RuntimeMetricsDuration{
		Unit: "ms", Samples: samples, P50: characterizationFloat64(p50), P95: characterizationFloat64(p95),
	}
}

func characterizationFloat64(value float64) *float64 {
	return &value
}

func characterizationInt64(value int64) *int64 {
	return &value
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
			t.Fatalf("encode metrics artifact %q: %v", path, err)
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

func assertBreakdownKeys(t *testing.T, breakdowns []factoryvisualization.RuntimeMetricsBreakdown, want []string) {
	t.Helper()
	if len(breakdowns) != len(want) {
		t.Fatalf("breakdown keys = %#v, want %#v", breakdowns, want)
	}
	for index, key := range want {
		if breakdowns[index].Key != key {
			t.Fatalf("breakdown[%d].Key = %q, want %q", index, breakdowns[index].Key, key)
		}
	}
}

func assertDuration(
	t *testing.T,
	duration *factoryvisualization.RuntimeMetricsDuration,
	wantP50, wantP95 float64,
	wantSamples int,
	wantUnit string,
) {
	t.Helper()
	if duration == nil || duration.P50 == nil || duration.P95 == nil {
		t.Fatalf("duration = %#v, want populated percentiles", duration)
	}
	if *duration.P50 != wantP50 || *duration.P95 != wantP95 || duration.Samples != wantSamples || duration.Unit != wantUnit {
		t.Fatalf("duration = %#v, want p50=%v p95=%v samples=%d unit=%q", duration, wantP50, wantP95, wantSamples, wantUnit)
	}
}

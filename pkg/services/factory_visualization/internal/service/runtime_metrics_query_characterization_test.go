package service

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

func runRuntimeMetricsQueryCharacterizesFixedArtifactCorpus(t *testing.T) {
	t.Helper()
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
	query, err := NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
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
		result, err := query.QueryRuntimeMetrics(context.Background(), testCase.request)
		if err != nil {
			t.Fatalf("%s: QueryRuntimeMetrics() error = %v", testCase.name, err)
		}
		if result.Totals.InputTokens != testCase.inputTokens || result.Totals.OutputTokens != testCase.outputTokens || result.Totals.CompletedDispatches != testCase.completed {
			t.Fatalf("%s: totals = %#v, want input %v, output %v, completed %v", testCase.name, result.Totals, testCase.inputTokens, testCase.outputTokens, testCase.completed)
		}
		assertBreakdownKeys(t, result.Workstations, testCase.workstations)
		if len(result.UsageRows) != testCase.usageRows {
			t.Fatalf("%s: usage rows = %#v, want %d rows", testCase.name, result.UsageRows, testCase.usageRows)
		}
		assertDuration(t, result.Totals.DispatchDuration, testCase.dispatchP50, testCase.dispatchP95, len(testCase.workstations), "ms")
		assertDuration(t, result.Totals.ProviderDuration, testCase.providerP50, testCase.providerP95, len(testCase.workstations), "ms")
	}
}

func runRuntimeMetricsQueryCharacterizationRejectsMalformedCompleteLine(t *testing.T) {
	t.Helper()
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
	query, err := NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
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

// Each leaf owns its query, controlled reader and diagnostic capture. No real
// reader, transport or process is assembled for these operation witnesses.
func TestRuntimeMetricsQuerySelectedLoggerPreservesOutcomes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  func(*testing.T)
	}{
		{"MQ-U01 selected success", testSelectedMetricsSuccess},
		{"MQ-U02 empty", testSelectedMetricsEmpty},
		{"MQ-U03 reader precedence", testSelectedMetricsReaders},
		{"MQ-U04 read failure", testSelectedMetricsReadFailure},
		{"MQ-U05 partial stream failure", testSelectedMetricsPartialFailure},
		{"MQ-U06 invalid usage", testSelectedMetricsInvalidUsage},
		{"MQ-U07 invalid input", testSelectedMetricsInvalidInput},
		{"MQ-U08 cancellation", testSelectedMetricsCancellation},
		{"MQ-U09 typed reader error", testSelectedMetricsTypedError},
		{"MQ-U10 independent scopes", testSelectedMetricsScopes},
		{"MQ-U11 explicit Noop", testSelectedMetricsNoop},
		{"MQ-U12 payload privacy", testSelectedMetricsPrivacy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); tc.run(t) })
	}
}

type selectedMetricsLog struct {
	level, message string
	fields         []any
}

type selectedMetricsCapture struct {
	entries []selectedMetricsLog
}

func (l *selectedMetricsCapture) Debug(msg string, fields ...any) {
	l.entries = append(l.entries, selectedMetricsLog{"Debug", msg, append([]any(nil), fields...)})
}
func (l *selectedMetricsCapture) Verbose(msg string, fields ...any) {
	l.entries = append(l.entries, selectedMetricsLog{"Verbose", msg, append([]any(nil), fields...)})
}
func (l *selectedMetricsCapture) Info(msg string, fields ...any) {
	l.entries = append(l.entries, selectedMetricsLog{"Info", msg, append([]any(nil), fields...)})
}
func (l *selectedMetricsCapture) Warn(msg string, fields ...any) {
	l.entries = append(l.entries, selectedMetricsLog{"Warn", msg, append([]any(nil), fields...)})
}
func (l *selectedMetricsCapture) Error(msg string, fields ...any) {
	l.entries = append(l.entries, selectedMetricsLog{"Error", msg, append([]any(nil), fields...)})
}

type selectedMetricsRead struct {
	records     []RuntimeMetricRecord
	err         error
	calls       []string
	beforeVisit func()
}

func (r *selectedMetricsRead) Read(_ context.Context, root string) ([]RuntimeMetricRecord, error) {
	r.calls = append(r.calls, "Read:"+root)
	return r.records, r.err
}

type selectedMetricsStream struct{ *selectedMetricsRead }

func (r *selectedMetricsStream) Stream(_ context.Context, root string, visit func(RuntimeMetricRecord) error) error {
	r.calls = append(r.calls, "Stream:"+root)
	for _, record := range r.records {
		if r.beforeVisit != nil {
			r.beforeVisit()
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return r.err
}

type selectedMetricsReader struct{ *selectedMetricsStream }

func (r *selectedMetricsReader) StreamSelected(ctx context.Context, root string, selection platformmetrics.StreamSelection, visit func(RuntimeMetricRecord) error) error {
	r.calls = append(r.calls, "Selected:"+root)
	for _, record := range r.records {
		fields := make(map[string]string)
		for _, key := range selection.EnvelopeFields {
			fields[key], _ = record[key].(string)
		}
		if selection.IncludeEnvelope != nil && !selection.IncludeEnvelope(platformmetrics.RuntimeMetricRecordEnvelope{Fields: fields}) {
			continue
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return r.err
}

func selectedMetricsFixture(scope string) (RuntimeMetricsQueryRequest, []RuntimeMetricRecord) {
	request := RuntimeMetricsQueryRequest{MetricsRoot: "metrics-" + scope, SessionID: "session-" + scope, RuntimeInstanceID: "runtime-" + scope}
	record := metricRecord("provider.input_tokens", 7, request.SessionID, request.RuntimeInstanceID, "station", "model", "provider-"+scope, "", "tokens")
	record["payload"] = "private-metrics-payload-sentinel"
	return request, []RuntimeMetricRecord{record}
}

func selectedMetricsConstruct(t *testing.T, reader RuntimeMetricsReader, logger logging.Logger) RuntimeMetricsQuery {
	t.Helper()
	query, err := NewRuntimeMetricsQuery(reader, logger)
	if err != nil {
		t.Fatal(err)
	}
	return query
}

func selectedMetricsStart(request RuntimeMetricsQueryRequest) selectedMetricsLog {
	return selectedMetricsLog{"Info", "Factory Runtime metrics query started", []any{"metrics_root", request.MetricsRoot, "session_id", request.SessionID, "runtime_instance_id", request.RuntimeInstanceID, "group_by", ""}}
}

func assertSelectedMetricsLogs(t *testing.T, capture *selectedMetricsCapture, want ...selectedMetricsLog) {
	t.Helper()
	if !reflect.DeepEqual(capture.entries, want) {
		t.Fatalf("diagnostics = %#v, want %#v", capture.entries, want)
	}
}

func assertSelectedMetricsSuccess(t *testing.T, result RuntimeMetricsQueryResult, request RuntimeMetricsQueryRequest) {
	t.Helper()
	want := RuntimeMetricsQueryResult{
		Cost:         factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostUnavailable},
		Totals:       factoryvisualization.RuntimeMetricsAggregate{InputTokens: 7},
		Workstations: []factoryvisualization.RuntimeMetricsBreakdown{{Key: "station", Aggregate: factoryvisualization.RuntimeMetricsAggregate{InputTokens: 7}}},
		WorkerTypes:  []factoryvisualization.RuntimeMetricsBreakdown{{Key: "model", Aggregate: factoryvisualization.RuntimeMetricsAggregate{InputTokens: 7}}},
		Providers:    []factoryvisualization.RuntimeMetricsBreakdown{{Key: "provider-" + strings.TrimPrefix(request.SessionID, "session-"), Aggregate: factoryvisualization.RuntimeMetricsAggregate{InputTokens: 7}}},
		UsageRows:    []factoryvisualization.RuntimeMetricsUsageRow{{FactorySessionID: request.SessionID, Provider: "provider-" + strings.TrimPrefix(request.SessionID, "session-"), InputTokens: characterizationInt64(7)}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %#v, want %#v", result, want)
	}
}

func assertSelectedMetricsCompletion(t *testing.T, capture *selectedMetricsCapture, request RuntimeMetricsQueryRequest, count, groups int) {
	t.Helper()
	assertSelectedMetricsLogs(t, capture, selectedMetricsStart(request), selectedMetricsLog{"Info", "Factory Runtime metrics query completed", []any{
		"metrics_root", request.MetricsRoot, "session_id", request.SessionID, "runtime_instance_id", request.RuntimeInstanceID,
		"records_considered", count, "workstation_groups", groups, "worker_type_groups", groups, "provider_groups", groups,
		"cost_availability", factoryvisualization.RuntimeMetricsCostUnavailable,
	}})
}

func testSelectedMetricsSuccess(t *testing.T) {
	request, records := selectedMetricsFixture("success")
	ignored := metricRecord("provider.input_tokens", 99, "other", request.RuntimeInstanceID, "other", "other", "other", "", "tokens")
	otherRuntime := metricRecord("provider.input_tokens", 99, request.SessionID, "other", "other", "other", "other", "", "tokens")
	reader := &selectedMetricsRead{records: append(records, ignored, otherRuntime)}
	capture := &selectedMetricsCapture{}
	query := selectedMetricsConstruct(t, &selectedMetricsReader{&selectedMetricsStream{reader}}, capture)
	if len(reader.calls) != 0 || len(capture.entries) != 0 {
		t.Fatal("construction performed reads or logging")
	}
	result, err := query.QueryRuntimeMetrics(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertSelectedMetricsSuccess(t, result, request)
	assertSelectedMetricsCompletion(t, capture, request, 1, 1)
	if !reflect.DeepEqual(reader.calls, []string{"Selected:" + request.MetricsRoot}) {
		t.Fatalf("reader calls = %v", reader.calls)
	}
}

func testSelectedMetricsEmpty(t *testing.T) {
	request, _ := selectedMetricsFixture("empty")
	capture := &selectedMetricsCapture{}
	query := selectedMetricsConstruct(t, &selectedMetricsRead{}, capture)
	result, err := query.QueryRuntimeMetrics(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := RuntimeMetricsQueryResult{
		Cost:         factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostUnavailable},
		Workstations: []factoryvisualization.RuntimeMetricsBreakdown{},
		WorkerTypes:  []factoryvisualization.RuntimeMetricsBreakdown{},
		Providers:    []factoryvisualization.RuntimeMetricsBreakdown{},
		UsageRows:    []factoryvisualization.RuntimeMetricsUsageRow{},
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("empty result = %#v", result)
	}
	assertSelectedMetricsCompletion(t, capture, request, 0, 0)
}

func testSelectedMetricsReaders(t *testing.T) {
	for _, mode := range []string{"Read", "Stream"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			request, records := selectedMetricsFixture(mode)
			base := &selectedMetricsRead{records: records}
			var reader RuntimeMetricsReader = base
			if mode == "Stream" {
				reader = &selectedMetricsStream{base}
			}
			capture := &selectedMetricsCapture{}
			result, err := selectedMetricsConstruct(t, reader, capture).QueryRuntimeMetrics(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			assertSelectedMetricsSuccess(t, result, request)
			assertSelectedMetricsCompletion(t, capture, request, 1, 1)
			if !reflect.DeepEqual(base.calls, []string{mode + ":" + request.MetricsRoot}) {
				t.Fatalf("reader calls = %v", base.calls)
			}
		})
	}
}

func assertSelectedMetricsFailure(t *testing.T, result RuntimeMetricsQueryResult, err error, kind factoryvisualization.RuntimeMetricsQueryErrorKind, cause error) *RuntimeMetricsQueryError {
	t.Helper()
	var typed *RuntimeMetricsQueryError
	if !errors.As(err, &typed) || typed.Kind != kind || typed.Cause == nil {
		t.Fatalf("error = %v, want %s with cause", err, kind)
	}
	if !errors.Is(err, typed.Cause) || (cause != nil && !errors.Is(err, cause)) {
		t.Fatalf("error cause not preserved: %v", err)
	}
	if !reflect.DeepEqual(result, RuntimeMetricsQueryResult{}) {
		t.Fatalf("partial result = %#v", result)
	}
	return typed
}

func testSelectedMetricsReadFailure(t *testing.T)    { testSelectedMetricsReaderFailure(t, false) }
func testSelectedMetricsPartialFailure(t *testing.T) { testSelectedMetricsReaderFailure(t, true) }
func testSelectedMetricsReaderFailure(t *testing.T, partial bool) {
	request, records := selectedMetricsFixture("failure")
	cause := errors.New("controlled read failure")
	base := &selectedMetricsRead{err: cause}
	var reader RuntimeMetricsReader = base
	if partial {
		base.records = records
		reader = &selectedMetricsStream{base}
	}
	capture := &selectedMetricsCapture{}
	result, err := selectedMetricsConstruct(t, reader, capture).QueryRuntimeMetrics(context.Background(), request)
	_ = assertSelectedMetricsFailure(t, result, err, factoryvisualization.RuntimeMetricsQueryReadFailed, cause)
	assertSelectedMetricsLogs(t, capture, selectedMetricsStart(request), selectedMetricsLog{"Error", "Factory Runtime metrics query failed", []any{
		"metrics_root", request.MetricsRoot, "session_id", request.SessionID, "runtime_instance_id", request.RuntimeInstanceID, "error", cause,
	}})
}

func testSelectedMetricsInvalidUsage(t *testing.T) {
	for _, rows := range []bool{false, true} {
		t.Run(map[bool]string{false: "record", true: "rows"}[rows], func(t *testing.T) {
			t.Parallel()
			request, records := selectedMetricsFixture("invalid")
			records[0]["value"] = 1.5
			if rows {
				records[0]["value"] = 2
				cached := metricRecord("provider.cached_input_tokens", 3, request.SessionID, request.RuntimeInstanceID, "station", "model", "provider-invalid", "", "tokens")
				records = append(records, cached)
			}
			capture := &selectedMetricsCapture{}
			result, err := selectedMetricsConstruct(t, &selectedMetricsStream{&selectedMetricsRead{records: records}}, capture).QueryRuntimeMetrics(context.Background(), request)
			typed := assertSelectedMetricsFailure(t, result, err, factoryvisualization.RuntimeMetricsQueryInvalidUsage, nil)
			warning := selectedMetricsLog{"Warn", "Factory Runtime metrics usage record rejected", []any{"metrics_root", request.MetricsRoot, "metric_name", "provider.input_tokens", "error", typed.Cause}}
			if rows {
				warning = selectedMetricsLog{"Warn", "Factory Runtime metrics usage rows rejected", []any{"metrics_root", request.MetricsRoot, "error", typed.Cause}}
			}
			assertSelectedMetricsLogs(t, capture, selectedMetricsStart(request), warning)
		})
	}
}

func testSelectedMetricsInvalidInput(t *testing.T) {
	for _, mode := range []string{"reader", "root", "group", "window"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			capture := &selectedMetricsCapture{}
			if mode == "reader" {
				if q, err := NewRuntimeMetricsQuery(nil, capture); q != nil || err == nil {
					t.Fatal("missing reader accepted")
				}
				return
			}
			request, _ := selectedMetricsFixture(mode)
			switch mode {
			case "root":
				request.MetricsRoot = " "
			case "group":
				request.GroupBy = "invalid"
			case "window":
				start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
				end := start.Add(-time.Hour)
				request.StartTimeUTC = start
				request.EndTimeUTC = end
			}
			reader := &selectedMetricsRead{}
			result, err := selectedMetricsConstruct(t, reader, capture).QueryRuntimeMetrics(context.Background(), request)
			var typed *RuntimeMetricsQueryError
			if !errors.As(err, &typed) || typed.Kind != factoryvisualization.RuntimeMetricsQueryInvalidInput || !reflect.DeepEqual(result, RuntimeMetricsQueryResult{}) {
				t.Fatalf("invalid input result = %#v, error = %v", result, err)
			}
			if len(reader.calls) != 0 || len(capture.entries) != 0 {
				t.Fatal("invalid input started operation")
			}
		})
	}
}

func testSelectedMetricsCancellation(t *testing.T) {
	for _, mode := range []string{"pre-cancelled", "cancelled stream", "deadline stream", "callback cancelled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			request, records := selectedMetricsFixture(mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := context.Canceled
			if mode == "deadline stream" {
				cause = context.DeadlineExceeded
			}
			if mode == "pre-cancelled" {
				cancel()
			}
			capture := &selectedMetricsCapture{}
			reader := &selectedMetricsRead{records: records, err: cause}
			if mode == "callback cancelled" {
				reader.beforeVisit = cancel
			}
			result, err := selectedMetricsConstruct(t, &selectedMetricsStream{reader}, capture).QueryRuntimeMetrics(ctx, request)
			if err != cause || !reflect.DeepEqual(result, RuntimeMetricsQueryResult{}) { //nolint:errorlint // The query must return the original context sentinel without wrapping.
				t.Fatalf("cancel result = %#v, error = %v", result, err)
			}
			if mode == "pre-cancelled" {
				if len(reader.calls) != 0 || len(capture.entries) != 0 {
					t.Fatal("cancelled operation started")
				}
				return
			}
			assertSelectedMetricsLogs(t, capture, selectedMetricsStart(request))
		})
	}
}

func testSelectedMetricsTypedError(t *testing.T) {
	request, _ := selectedMetricsFixture("typed")
	cause := errors.New("typed cause")
	want := &RuntimeMetricsQueryError{Kind: factoryvisualization.RuntimeMetricsQueryReadFailed, Message: "controlled typed error", Cause: cause}
	capture := &selectedMetricsCapture{}
	result, err := selectedMetricsConstruct(t, &selectedMetricsRead{err: want}, capture).QueryRuntimeMetrics(context.Background(), request)
	_ = assertSelectedMetricsFailure(t, result, err, want.Kind, cause)
	if err != want { //nolint:errorlint // Already typed reader errors must retain pointer identity.
		t.Fatalf("typed error identity lost: %v", err)
	}
	assertSelectedMetricsLogs(t, capture, selectedMetricsStart(request))
}

func testSelectedMetricsScopes(t *testing.T) {
	for _, scope := range []string{"left", "right"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			testSelectedMetricsScopedSuccess(t, scope, &selectedMetricsCapture{})
		})
	}
}
func testSelectedMetricsScopedSuccess(t *testing.T, scope string, logger logging.Logger) {
	request, records := selectedMetricsFixture(scope)
	result, err := selectedMetricsConstruct(t, &selectedMetricsRead{records: records}, logger).QueryRuntimeMetrics(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertSelectedMetricsSuccess(t, result, request)
	if capture, ok := logger.(*selectedMetricsCapture); ok {
		assertSelectedMetricsCompletion(t, capture, request, 1, 1)
	}
}
func testSelectedMetricsNoop(t *testing.T) {
	testSelectedMetricsScopedSuccess(t, "noop", &selectedMetricsCapture{})
	testSelectedMetricsScopedSuccess(t, "noop", logging.NoopLogger{})
	request, _ := selectedMetricsFixture("noop")
	cause := errors.New("quiet failure")
	result, err := selectedMetricsConstruct(t, &selectedMetricsRead{err: cause}, logging.NoopLogger{}).QueryRuntimeMetrics(context.Background(), request)
	_ = assertSelectedMetricsFailure(t, result, err, factoryvisualization.RuntimeMetricsQueryReadFailed, cause)
}
func testSelectedMetricsPrivacy(t *testing.T) {
	request, records := selectedMetricsFixture("privacy")
	records[0]["value"] = 1.5
	capture := &selectedMetricsCapture{}
	result, err := selectedMetricsConstruct(t, &selectedMetricsRead{records: records}, capture).QueryRuntimeMetrics(context.Background(), request)
	_ = assertSelectedMetricsFailure(t, result, err, factoryvisualization.RuntimeMetricsQueryInvalidUsage, nil)
	if strings.Contains(fmt.Sprint(err, capture.entries), "private-metrics-payload-sentinel") || strings.Contains(fmt.Sprint(capture.entries), "payload") {
		t.Fatalf("payload leaked in diagnostics or public error: %v, %#v", err, capture.entries)
	}
}

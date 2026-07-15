package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	factorymetrics "github.com/portpowered/infinite-you/pkg/factory/metrics"
)

func TestNormalizeRuntimeMetricsConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    RuntimeMetricsConfig
		expected RuntimeMetricsConfig
	}{
		{
			name:     "zero uses defaults",
			input:    RuntimeMetricsConfig{},
			expected: DefaultRuntimeMetricsConfig(),
		},
		{
			name:     "negative values are clamped",
			input:    RuntimeMetricsConfig{MaxSize: 0, MaxBackups: -1, MaxAge: -2},
			expected: DefaultRuntimeMetricsConfig(),
		},
		{
			name:     "explicit values preserved",
			input:    RuntimeMetricsConfig{MaxSize: 5, MaxBackups: 7, MaxAge: 14, Compress: true},
			expected: RuntimeMetricsConfig{MaxSize: 5, MaxBackups: 7, MaxAge: 14, Compress: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeRuntimeMetricsConfig(tc.input)
			if got != tc.expected {
				t.Fatalf("normalizeRuntimeMetricsConfig(%#v) = %#v, want %#v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestBuildRuntimeMetricsSinkUsesConfiguredRollingPolicy(t *testing.T) {
	sink, err := BuildRuntimeMetricsSink(
		"session-configured",
		"runtime-configured",
		"/factory",
		"/factory/current",
		t.TempDir(),
		RuntimeMetricsConfig{
			MaxSize:    3,
			MaxBackups: 4,
			MaxAge:     15,
			Compress:   true,
		},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer sink.Close()

	metricsWriter, ok := sink.writer.(*runtimeMetricsWriter)
	if !ok {
		t.Fatalf("expected runtime metrics sink to use wrapped writer, got %T", sink.writer)
	}
	rollingWriter := metricsWriter.writer
	if rollingWriter == nil {
		t.Fatal("wrapped runtime metrics writer = nil, want lumberjack writer")
	}
	if rollingWriter.Filename != sink.Path() {
		t.Fatalf("rolling writer filename = %q, want active metrics path %q", rollingWriter.Filename, sink.Path())
	}
	if rollingWriter.MaxSize != 3 {
		t.Fatalf("rolling MaxSize = %d, want %d", rollingWriter.MaxSize, 3)
	}
	if rollingWriter.MaxBackups != 4 {
		t.Fatalf("rolling MaxBackups = %d, want %d", rollingWriter.MaxBackups, 4)
	}
	if rollingWriter.MaxAge != 15 {
		t.Fatalf("rolling MaxAge = %d, want %d", rollingWriter.MaxAge, 15)
	}
	if !rollingWriter.Compress {
		t.Fatalf("rolling Compress = false, want true")
	}
	if sink.Config() != (RuntimeMetricsConfig{MaxSize: 3, MaxBackups: 4, MaxAge: 15, Compress: true}) {
		t.Fatalf("sink runtime metrics config = %#v, want configured rolling policy", sink.Config())
	}
}

func TestBuildRuntimeMetricsSinkCreatesDatedDirectoriesOnFreshRoot(t *testing.T) {
	metricsDir := t.TempDir()
	assertRuntimeArtifactRootLacksCalendarDirectories(t, metricsDir)
	before := time.Now().UTC()

	sink, err := BuildRuntimeMetricsSink(
		"session-fresh-root",
		"runtime-fresh-root",
		"/folder",
		"/factory",
		metricsDir,
		RuntimeMetricsConfig{},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeArtifactDatedDirPresent(t, filepath.Dir(sink.Path()))
	assertPathUsesPlatformSeparators(t, sink.Path())
	assertRuntimeMetricsPathFormat(
		t,
		sink.Path(),
		metricsDir,
		"session-fresh-root",
		"runtime-fresh-root",
		before,
		after,
	)

	if err := sink.Counter(context.Background(), "runtime.started", 1, factorymetrics.Fields{}); err != nil {
		t.Fatalf("Counter on fresh root metrics sink: %v", err)
	}
	if _, err := os.Stat(sink.Path()); err != nil {
		t.Fatalf("active runtime metrics file %q should remain open after write: %v", sink.Path(), err)
	}
}

func TestBuildRuntimeMetricsSinkCreatesUTCPathUnderConfiguredRoot(t *testing.T) {
	metricsDir := t.TempDir()
	before := time.Now().UTC()

	sink, err := BuildRuntimeMetricsSink("session-one", "runtime-path-format", "/folder", "/factory", metricsDir, RuntimeMetricsConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeMetricsPathFormat(t, sink.Path(), metricsDir, "session-one", "runtime-path-format", before, after)
	if sink.RootDir() != metricsDir {
		t.Fatalf("RootDir() = %q, want %q", sink.RootDir(), metricsDir)
	}
	if sink.StartTimeUTC().IsZero() {
		t.Fatal("StartTimeUTC() is zero")
	}
	if sink.StartTimeUTC().Location() != time.UTC {
		t.Fatalf("StartTimeUTC() location = %s, want UTC", sink.StartTimeUTC().Location())
	}
}

func TestBuildRuntimeMetricsSinkUsesCanonicalDefaultMetricsDirectoryAsRoot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	before := time.Now().UTC()

	sink, err := BuildRuntimeMetricsSink("session-default", "runtime-default", "", "", "", RuntimeMetricsConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeMetricsPathFormat(
		t,
		sink.Path(),
		defaultpaths.RuntimeMetricsRoot(homeDir),
		"session-default",
		"runtime-default",
		before,
		after,
	)
}

func TestRuntimeMetricsSinkDoesNotRecreateFileAfterClose(t *testing.T) {
	metricsDir := t.TempDir()
	sink, err := BuildRuntimeMetricsSink("session-close", "runtime-close", "/folder", "/factory", metricsDir, RuntimeMetricsConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}

	if err := sink.Counter(context.Background(), "runtime.started", 1, factorymetrics.Fields{}); err != nil {
		t.Fatalf("Counter before close: %v", err)
	}
	record := readSingleRuntimeMetricsRecord(t, sink.Path())
	if record["metric_name"] != "runtime.started" {
		t.Fatalf("metric_name = %#v, want runtime.started", record["metric_name"])
	}
	if record["session_id"] != "session-close" || record["runtime_instance_id"] != "runtime-close" {
		t.Fatalf("correlation fields = %#v", record)
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("Close(runtime metrics sink): %v", err)
	}
	if err := os.Remove(sink.Path()); err != nil {
		t.Fatalf("remove runtime metrics path after close: %v", err)
	}

	if err := sink.Counter(context.Background(), "runtime.after_close", 1, factorymetrics.Fields{}); !errors.Is(err, errRuntimeMetricsSinkClosed) {
		t.Fatalf("Counter after close error = %v, want %v", err, errRuntimeMetricsSinkClosed)
	}
	if _, err := os.Stat(sink.Path()); !os.IsNotExist(err) {
		t.Fatalf("runtime metrics path exists after close and late write, stat err = %v", err)
	}
}

func TestRuntimeMetricsSinkWritesStableJSONLEnvelope(t *testing.T) {
	metricsDir := t.TempDir()
	sink, err := BuildRuntimeMetricsSink(
		"session-envelope",
		"runtime-envelope",
		"/factory/folder",
		"/factory",
		metricsDir,
		RuntimeMetricsConfig{},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer sink.Close()

	fields := factorymetrics.Fields{
		DispatchID:  "dispatch-123",
		WorkID:      "work-456",
		TraceID:     "trace-789",
		Workstation: "provider",
		WorkerType:  "llm",
		Provider:    "openai",
		Outcome:     "completed",
		Reason:      "ok",
	}
	if err := sink.Sample(context.Background(), "dispatch.duration", 42.5, "ms", fields); err != nil {
		t.Fatalf("Sample: %v", err)
	}

	records := readRuntimeMetricsRecords(t, sink.Path())
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]

	assertRuntimeMetricStringField(t, record, "metric_name", "dispatch.duration")
	assertRuntimeMetricStringField(t, record, "metric_type", metricsMetricTypeSample)
	assertRuntimeMetricNumberField(t, record, "value", 42.5)
	assertRuntimeMetricStringField(t, record, "unit", "ms")
	assertRuntimeMetricStringField(t, record, "session_id", "session-envelope")
	assertRuntimeMetricStringField(t, record, "runtime_instance_id", "runtime-envelope")
	assertRuntimeMetricStringField(t, record, "folder_path", "/factory/folder")
	assertRuntimeMetricStringField(t, record, "factory_dir", "/factory")
	assertRuntimeMetricStringField(t, record, "dispatch_id", "dispatch-123")
	assertRuntimeMetricStringField(t, record, "work_id", "work-456")
	assertRuntimeMetricStringField(t, record, "trace_id", "trace-789")
	assertRuntimeMetricStringField(t, record, "workstation", "provider")
	assertRuntimeMetricStringField(t, record, "worker_type", "llm")
	assertRuntimeMetricStringField(t, record, "provider", "openai")
	assertRuntimeMetricStringField(t, record, "outcome", "completed")
	assertRuntimeMetricStringField(t, record, "reason", "ok")
	assertRuntimeMetricTimestampField(t, record, "ts")
}

func TestRuntimeMetricsSinkCounterAndGaugeKeepEnvelopeStableWithoutOptionalFields(t *testing.T) {
	metricsDir := t.TempDir()
	sink, err := BuildRuntimeMetricsSink(
		"session-stable",
		"runtime-stable",
		"/folder",
		"/factory",
		metricsDir,
		RuntimeMetricsConfig{},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer sink.Close()

	if err := sink.Counter(context.Background(), "runtime.started", 1, factorymetrics.Fields{}); err != nil {
		t.Fatalf("Counter: %v", err)
	}
	if err := sink.Gauge(context.Background(), "queue.depth", 3, factorymetrics.Fields{}); err != nil {
		t.Fatalf("Gauge: %v", err)
	}

	records := readRuntimeMetricsRecords(t, sink.Path())
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}

	assertRuntimeMetricStringField(t, records[0], "metric_name", "runtime.started")
	assertRuntimeMetricStringField(t, records[0], "metric_type", metricsMetricTypeCounter)
	assertRuntimeMetricNumberField(t, records[0], "value", 1)
	assertRuntimeMetricStringField(t, records[0], "unit", "")
	assertRuntimeMetricTimestampField(t, records[0], "ts")
	assertRuntimeMetricFieldAbsent(t, records[0], "dispatch_id")
	assertRuntimeMetricFieldAbsent(t, records[0], "provider")

	assertRuntimeMetricStringField(t, records[1], "metric_name", "queue.depth")
	assertRuntimeMetricStringField(t, records[1], "metric_type", metricsMetricTypeGauge)
	assertRuntimeMetricNumberField(t, records[1], "value", 3)
	assertRuntimeMetricStringField(t, records[1], "unit", "")
	assertRuntimeMetricTimestampField(t, records[1], "ts")
	assertRuntimeMetricFieldAbsent(t, records[1], "work_id")
	assertRuntimeMetricFieldAbsent(t, records[1], "reason")
}

func TestBuildRuntimeMetricsSinkAvoidsConcurrentPathCollisions(t *testing.T) {
	metricsDir := t.TempDir()
	const workers = 16
	paths := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sink, err := BuildRuntimeMetricsSink("session-concurrent", "runtime-concurrent", "", "", metricsDir, RuntimeMetricsConfig{})
			if err != nil {
				errs <- err
				return
			}
			defer sink.Close()
			paths <- sink.Path()
		}()
	}
	wg.Wait()
	close(paths)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("BuildRuntimeMetricsSink concurrent: %v", err)
		}
	}
	seen := map[string]struct{}{}
	for path := range paths {
		assertPathContainsRuntimeMetricsIDs(t, path, "session-concurrent", "runtime-concurrent")
		if _, ok := seen[path]; ok {
			t.Fatalf("duplicate concurrent runtime metrics path %q", path)
		}
		seen[path] = struct{}{}
	}
	if len(seen) != workers {
		t.Fatalf("created %d unique runtime metrics paths, want %d", len(seen), workers)
	}
}

func TestBuildRuntimeMetricsSinkManyConcurrentStartsKeepCorrelationIsolated(t *testing.T) {
	metricsDir := t.TempDir()
	const workers = 12
	type sinkResult struct {
		path              string
		sessionID         string
		runtimeInstanceID string
		folderPath        string
		factoryDir        string
	}
	results := make(chan sinkResult, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sessionID := "session-many-" + strconv.Itoa(index)
			runtimeInstanceID := "runtime-many-" + strconv.Itoa(index)
			folderPath := filepath.Join("/factory", "sessions", sessionID)
			factoryDir := filepath.Join("/factory", "roots", runtimeInstanceID)
			sink, err := BuildRuntimeMetricsSink(sessionID, runtimeInstanceID, folderPath, factoryDir, metricsDir, RuntimeMetricsConfig{})
			if err != nil {
				errs <- err
				return
			}
			if err := sink.Counter(context.Background(), "runtime.started", 1, factorymetrics.Fields{}); err != nil {
				_ = sink.Close()
				errs <- err
				return
			}
			path := sink.Path()
			if err := sink.Close(); err != nil {
				errs <- err
				return
			}
			results <- sinkResult{
				path:              path,
				sessionID:         sessionID,
				runtimeInstanceID: runtimeInstanceID,
				folderPath:        folderPath,
				factoryDir:        factoryDir,
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent runtime metrics startup: %v", err)
		}
	}

	seen := make(map[string]struct{}, workers)
	for result := range results {
		if _, ok := seen[result.path]; ok {
			t.Fatalf("duplicate concurrent runtime metrics path %q", result.path)
		}
		seen[result.path] = struct{}{}
		record := readSingleRuntimeMetricsRecord(t, result.path)
		assertRuntimeMetricStringField(t, record, "session_id", result.sessionID)
		assertRuntimeMetricStringField(t, record, "runtime_instance_id", result.runtimeInstanceID)
		assertRuntimeMetricStringField(t, record, "folder_path", result.folderPath)
		assertRuntimeMetricStringField(t, record, "factory_dir", result.factoryDir)
	}
	if len(seen) != workers {
		t.Fatalf("created %d unique runtime metrics paths, want %d", len(seen), workers)
	}
}

func TestRuntimeMetricsSinkRotatesUnderLoad(t *testing.T) {
	metricsDir := t.TempDir()
	sink, err := BuildRuntimeMetricsSink(
		"session-rotate",
		"runtime-rotate",
		"/folder",
		"/factory",
		metricsDir,
		RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 3, MaxAge: 1},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer sink.Close()

	largeReason := strings.Repeat("rotation-payload-", 8192)
	for i := 0; i < 16; i++ {
		if err := sink.Sample(context.Background(), "dispatch.duration", float64(i+1), "ms", factorymetrics.Fields{
			DispatchID: "dispatch-rotate",
			Reason:     largeReason,
		}); err != nil {
			t.Fatalf("Sample #%d: %v", i, err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close(runtime metrics sink): %v", err)
	}

	ext := filepath.Ext(sink.Path())
	base := strings.TrimSuffix(filepath.Base(sink.Path()), ext)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(sink.Path()), base+"*"+ext))
	if err != nil {
		t.Fatalf("Glob(runtime metrics rotation): %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("rotation files = %v, want active file plus at least one rotated segment", matches)
	}
	for _, path := range matches {
		if strings.HasSuffix(path, ".gz") {
			continue
		}
		records := readRuntimeMetricsRecords(t, path)
		if len(records) == 0 {
			t.Fatalf("runtime metrics rotation segment %q contained no records", path)
		}
	}
}

func assertRuntimeMetricsPathFormat(t *testing.T, path, rootDir, sessionID, runtimeInstanceID string, earliest, latest time.Time) {
	t.Helper()

	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		t.Fatalf("runtime metrics path %q is not below root %q: %v", path, rootDir, err)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 4 {
		t.Fatalf("runtime metrics relative path = %q, want <yyyy>/<mm>/<dd>/<time-session-runtime-unique>.log", rel)
	}
	if ok, err := regexp.MatchString(`^\d{4}$`, parts[0]); err != nil || !ok {
		t.Fatalf("year directory = %q, want yyyy", parts[0])
	}
	if ok, err := regexp.MatchString(`^\d{2}$`, parts[1]); err != nil || !ok {
		t.Fatalf("month directory = %q, want mm", parts[1])
	}
	if ok, err := regexp.MatchString(`^\d{2}$`, parts[2]); err != nil || !ok {
		t.Fatalf("day directory = %q, want dd", parts[2])
	}
	filenamePattern := regexp.MustCompile(`^(\d{6}\.\d{9})-runtime-metrics-` + regexp.QuoteMeta(sessionID) + `-` + regexp.QuoteMeta(runtimeInstanceID) + `-[A-Za-z0-9_.-]+\.log$`)
	matches := filenamePattern.FindStringSubmatch(parts[3])
	if matches == nil {
		t.Fatalf("runtime metrics filename = %q, want sortable UTC time, session ID, runtime ID, and uniqueness suffix", parts[3])
	}
	startedAt, err := time.ParseInLocation("2006 01 02 150405.000000000", parts[0]+" "+parts[1]+" "+parts[2]+" "+matches[1], time.UTC)
	if err != nil {
		t.Fatalf("parse runtime metrics timestamp from %q: %v", rel, err)
	}
	if startedAt.Before(earliest.Add(-time.Second)) || startedAt.After(latest.Add(time.Second)) {
		t.Fatalf("runtime metrics timestamp = %s, want between %s and %s", startedAt, earliest, latest)
	}
}

func assertPathContainsRuntimeMetricsIDs(t *testing.T, path, sessionID, runtimeInstanceID string) {
	t.Helper()

	base := filepath.Base(path)
	if !strings.Contains(base, "-runtime-metrics-"+sessionID+"-"+runtimeInstanceID+"-") {
		t.Fatalf("runtime metrics path %q does not include session ID %q and runtime instance ID %q", path, sessionID, runtimeInstanceID)
	}
}

func readSingleRuntimeMetricsRecord(t *testing.T, path string) map[string]any {
	t.Helper()

	records := readRuntimeMetricsRecords(t, path)
	if len(records) != 1 {
		t.Fatalf("runtime metrics record count = %d, want 1", len(records))
	}
	return records[0]
}

func readRuntimeMetricsRecords(t *testing.T, path string) []map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatalf("runtime metrics line count = %d, want at least 1:\n%s", len(lines), data)
	}
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime metrics line is not JSON: %v\nline: %s", err, line)
		}
		records = append(records, record)
	}
	return records
}

func assertRuntimeMetricStringField(t *testing.T, record map[string]any, field string, want string) {
	t.Helper()

	got, ok := record[field]
	if !ok {
		t.Fatalf("record missing %q: %#v", field, record)
	}
	gotString, ok := got.(string)
	if !ok {
		t.Fatalf("%s type = %T, want string", field, got)
	}
	if gotString != want {
		t.Fatalf("%s = %q, want %q", field, gotString, want)
	}
}

func assertRuntimeMetricNumberField(t *testing.T, record map[string]any, field string, want float64) {
	t.Helper()

	got, ok := record[field]
	if !ok {
		t.Fatalf("record missing %q: %#v", field, record)
	}
	gotNumber, ok := got.(float64)
	if !ok {
		t.Fatalf("%s type = %T, want float64", field, got)
	}
	if gotNumber != want {
		t.Fatalf("%s = %v, want %v", field, gotNumber, want)
	}
}

func assertRuntimeMetricTimestampField(t *testing.T, record map[string]any, field string) {
	t.Helper()

	value, ok := record[field]
	if !ok {
		t.Fatalf("record missing %q: %#v", field, record)
	}
	timestamp, ok := value.(string)
	if !ok {
		t.Fatalf("%s type = %T, want string", field, value)
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		t.Fatalf("%s = %q, want RFC3339Nano timestamp: %v", field, timestamp, err)
	}
}

func assertRuntimeMetricFieldAbsent(t *testing.T, record map[string]any, field string) {
	t.Helper()

	if _, ok := record[field]; ok {
		t.Fatalf("record unexpectedly includes %q: %#v", field, record)
	}
}

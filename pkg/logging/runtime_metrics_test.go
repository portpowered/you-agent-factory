package logging

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/internal/metrics"
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
	fixture := newRuntimeLogHomeFixture(t)
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
		filepath.Join(fixture.homeDir, ".you-agent-factory", "metrics"),
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

	if err := sink.Counter(context.Background(), "runtime.started", 1, metrics.Fields{}); err != nil {
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

	if err := sink.Counter(context.Background(), "runtime.after_close", 1, metrics.Fields{}); !errors.Is(err, errRuntimeMetricsSinkClosed) {
		t.Fatalf("Counter after close error = %v, want %v", err, errRuntimeMetricsSinkClosed)
	}
	if _, err := os.Stat(sink.Path()); !os.IsNotExist(err) {
		t.Fatalf("runtime metrics path exists after close and late write, stat err = %v", err)
	}
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
	filenamePattern := regexp.MustCompile(`^(\d{6}\.\d{9})-` + regexp.QuoteMeta(sessionID) + `-` + regexp.QuoteMeta(runtimeInstanceID) + `-[A-Za-z0-9_.-]+\.log$`)
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
	if !strings.Contains(base, "-"+sessionID+"-"+runtimeInstanceID+"-") {
		t.Fatalf("runtime metrics path %q does not include session ID %q and runtime instance ID %q", path, sessionID, runtimeInstanceID)
	}
}

func readSingleRuntimeMetricsRecord(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("runtime metrics line count = %d, want 1:\n%s", len(lines), data)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("runtime metrics line is not JSON: %v\nline: %s", err, lines[0])
	}
	return record
}

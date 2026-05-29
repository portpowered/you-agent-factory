package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"
)

type runtimeLogHomeFixture struct {
	t       *testing.T
	homeDir string
}

func newRuntimeLogHomeFixture(t *testing.T) runtimeLogHomeFixture {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	return runtimeLogHomeFixture{
		t:       t,
		homeDir: homeDir,
	}
}

func (f runtimeLogHomeFixture) canonicalLogDir() string {
	return filepath.Join(f.homeDir, ".you-agent-factory", "logs")
}

func (f runtimeLogHomeFixture) canonicalLogPath(name string) string {
	return filepath.Join(f.canonicalLogDir(), name)
}

func (f runtimeLogHomeFixture) legacyLogDir() string {
	return filepath.Join(f.homeDir, ".agent-factory", "logs")
}

func (f runtimeLogHomeFixture) legacyLogPath(name string) string {
	return filepath.Join(f.legacyLogDir(), name)
}

func (f runtimeLogHomeFixture) writeLegacyLog(name, contents string) string {
	f.t.Helper()

	path := f.legacyLogPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		f.t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func (f runtimeLogHomeFixture) writeCanonicalLog(name, contents string) string {
	f.t.Helper()

	path := f.canonicalLogPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		f.t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func (f runtimeLogHomeFixture) buildDefaultRuntimeLogger(runtimeInstanceID string) *RuntimeLogSink {
	f.t.Helper()

	sink, err := BuildRuntimeLogger(zap.NewNop(), runtimeInstanceID, "", RuntimeLogConfig{})
	if err != nil {
		f.t.Fatalf("BuildRuntimeLogger: %v", err)
	}
	return sink
}

func TestNormalizeRuntimeLogConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    RuntimeLogConfig
		expected RuntimeLogConfig
	}{
		{
			name:     "zero uses defaults",
			input:    RuntimeLogConfig{},
			expected: DefaultRuntimeLogConfig(),
		},
		{
			name:     "negative values are clamped",
			input:    RuntimeLogConfig{MaxSize: 0, MaxBackups: -1, MaxAge: -2},
			expected: DefaultRuntimeLogConfig(),
		},
		{
			name:     "explicit values preserved",
			input:    RuntimeLogConfig{MaxSize: 5, MaxBackups: 7, MaxAge: 14, Compress: true},
			expected: RuntimeLogConfig{MaxSize: 5, MaxBackups: 7, MaxAge: 14, Compress: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeRuntimeLogConfig(tc.input)
			if got != tc.expected {
				t.Fatalf("normalizeRuntimeLogConfig(%#v) = %#v, want %#v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestBuildRuntimeLoggerUsesConfiguredRollingPolicy(t *testing.T) {
	sink, err := BuildRuntimeLogger(
		zap.NewNop(),
		"runtime-configured",
		t.TempDir(),
		RuntimeLogConfig{
			MaxSize:    3,
			MaxBackups: 4,
			MaxAge:     15,
			Compress:   true,
		},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeLogger: %v", err)
	}
	defer sink.Close()

	rollingWriter, ok := sink.writer.(*lumberjack.Logger)
	if !ok {
		t.Fatalf("expected runtime logger to use lumberjack writer, got %T", sink.writer)
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
	if sink.config != (RuntimeLogConfig{MaxSize: 3, MaxBackups: 4, MaxAge: 15, Compress: true}) {
		t.Fatalf("sink runtime log config = %#v, want configured rolling policy", sink.config)
	}
}

func TestBuildRuntimeLoggerRotatesLogFiles(t *testing.T) {
	logDir := t.TempDir()
	before := time.Now().UTC()
	sink, err := BuildRuntimeLogger(
		zap.NewNop(),
		"runtime-rotates",
		logDir,
		RuntimeLogConfig{
			MaxSize:    1,
			MaxBackups: 2,
			MaxAge:     7,
		},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeLogger: %v", err)
	}
	defer sink.Close()

	payload := strings.Repeat("x", 200*1024)
	rollingWriter, ok := sink.writer.(*lumberjack.Logger)
	if !ok {
		t.Fatalf("expected runtime logger to use lumberjack writer, got %T", sink.writer)
	}
	if rollingWriter.Filename != sink.Path() {
		t.Fatalf("rolling writer filename = %q, want active runtime log path %q", rollingWriter.Filename, sink.Path())
	}
	assertRuntimeLogPathFormat(t, sink.Path(), logDir, "runtime-rotates", before, time.Now().UTC())

	for i := 0; i < 20; i++ {
		if _, err := rollingWriter.Write([]byte(payload)); err != nil {
			t.Fatalf("write rotated log data: %v", err)
		}
	}
	if err := rollingWriter.Rotate(); err != nil {
		t.Fatalf("rotate runtime logger: %v", err)
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("close runtime logger: %v", err)
	}

	matches, err := filepath.Glob(strings.TrimSuffix(sink.Path(), runtimeLogExtension) + "*.log*")
	if err != nil {
		t.Fatalf("glob runtime log files: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected rotated runtime logs, got %d files: %v", len(matches), matches)
	}

	basePath := sink.Path()
	baseDir := filepath.Dir(basePath)
	basePrefix := strings.TrimSuffix(filepath.Base(basePath), runtimeLogExtension)
	sawBackup := false
	for _, path := range matches {
		if filepath.Dir(path) != baseDir {
			t.Fatalf("rotated runtime log %q is outside active log directory %q", path, baseDir)
		}
		base := filepath.Base(path)
		if base == filepath.Base(basePath) {
			continue
		}
		sawBackup = true
		if !strings.HasPrefix(base, basePrefix+"-") {
			t.Fatalf("expected backup file name with timestamp suffix, got %q", base)
		}
	}
	if !sawBackup {
		t.Fatalf("expected at least one rotated backup next to %s, got %v", basePath, matches)
	}
}

func TestBuildRuntimeLoggerCreatesUTCSeparatedPathUnderConfiguredRoot(t *testing.T) {
	logDir := t.TempDir()
	before := time.Now().UTC()

	sink, err := BuildRuntimeLogger(zap.NewNop(), "runtime-path-format", logDir, RuntimeLogConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeLogger: %v", err)
	}
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeLogPathFormat(t, sink.Path(), logDir, "runtime-path-format", before, after)
	if sink.RootDir() != logDir {
		t.Fatalf("RootDir() = %q, want %q", sink.RootDir(), logDir)
	}
	if sink.StartTimeUTC().IsZero() {
		t.Fatal("StartTimeUTC() is zero")
	}
	if sink.StartTimeUTC().Location() != time.UTC {
		t.Fatalf("StartTimeUTC() location = %s, want UTC", sink.StartTimeUTC().Location())
	}
	if sink.StartTimeUTC().Before(before.Add(-time.Second)) || sink.StartTimeUTC().After(after.Add(time.Second)) {
		t.Fatalf("StartTimeUTC() = %s, want between %s and %s", sink.StartTimeUTC(), before, after)
	}
}

func TestBuildRuntimeLoggerUsesCanonicalDefaultLogDirectoryAsRoot(t *testing.T) {
	fixture := newRuntimeLogHomeFixture(t)
	before := time.Now().UTC()

	sink := fixture.buildDefaultRuntimeLogger("runtime-default-path")
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeLogPathFormat(t, sink.Path(), fixture.canonicalLogDir(), "runtime-default-path", before, after)
}

func TestBuildRuntimeLoggerMigratesLegacyDefaultLogDirectory(t *testing.T) {
	fixture := newRuntimeLogHomeFixture(t)

	legacyLogPath := fixture.writeLegacyLog("legacy.log", "legacy runtime log\n")
	before := time.Now().UTC()

	sink := fixture.buildDefaultRuntimeLogger("runtime-migrated")
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeLogPathFormat(t, sink.Path(), fixture.canonicalLogDir(), "runtime-migrated", before, after)
	if _, err := os.Stat(fixture.canonicalLogPath("legacy.log")); err != nil {
		t.Fatalf("expected migrated legacy log in canonical dir: %v", err)
	}
	if _, err := os.Stat(legacyLogPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy runtime log path to disappear after migration, stat err = %v", err)
	}
}

func TestBuildRuntimeLoggerLeavesCanonicalFlatHistoricalLogsUntouched(t *testing.T) {
	fixture := newRuntimeLogHomeFixture(t)
	historicalContents := "historical flat runtime log\n"
	historicalPath := fixture.writeCanonicalLog("historical.log", historicalContents)
	before := time.Now().UTC()

	sink := fixture.buildDefaultRuntimeLogger("runtime-flat-history")
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeLogPathFormat(t, sink.Path(), fixture.canonicalLogDir(), "runtime-flat-history", before, after)
	assertFileContents(t, historicalPath, historicalContents)
	if filepath.Dir(sink.Path()) == fixture.canonicalLogDir() {
		t.Fatalf("active runtime log path %q should use a UTC subdirectory below canonical root %q", sink.Path(), fixture.canonicalLogDir())
	}
}

func TestBuildRuntimeLoggerKeepsCanonicalDirWhenLegacyDirAlsoExists(t *testing.T) {
	fixture := newRuntimeLogHomeFixture(t)
	legacyLogPath := fixture.writeLegacyLog("legacy.log", "legacy runtime log\n")
	canonicalExistingPath := fixture.writeCanonicalLog("existing.log", "canonical runtime log\n")
	before := time.Now().UTC()

	sink := fixture.buildDefaultRuntimeLogger("runtime-canonical-wins")
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeLogPathFormat(t, sink.Path(), fixture.canonicalLogDir(), "runtime-canonical-wins", before, after)
	if _, err := os.Stat(legacyLogPath); err != nil {
		t.Fatalf("expected legacy runtime log to remain when canonical dir already exists: %v", err)
	}
	if _, err := os.Stat(canonicalExistingPath); err != nil {
		t.Fatalf("expected canonical runtime log to remain untouched: %v", err)
	}
}

func TestBuildRuntimeLoggerKeepsConfiguredRootFlatHistoricalLogsUntouched(t *testing.T) {
	logDir := t.TempDir()
	historicalPath := filepath.Join(logDir, "historical.log")
	historicalContents := "configured root flat runtime log\n"
	if err := os.WriteFile(historicalPath, []byte(historicalContents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", historicalPath, err)
	}
	before := time.Now().UTC()

	sink, err := BuildRuntimeLogger(zap.NewNop(), "runtime-configured-flat-history", logDir, RuntimeLogConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeLogger: %v", err)
	}
	defer sink.Close()
	after := time.Now().UTC()

	assertRuntimeLogPathFormat(t, sink.Path(), logDir, "runtime-configured-flat-history", before, after)
	assertFileContents(t, historicalPath, historicalContents)
	if filepath.Dir(sink.Path()) == logDir {
		t.Fatalf("active runtime log path %q should use a UTC subdirectory below configured root %q", sink.Path(), logDir)
	}
}

func TestBuildRuntimeLoggerSeparatesReusedRuntimeInstanceIDs(t *testing.T) {
	logDir := t.TempDir()

	first, err := BuildRuntimeLogger(zap.NewNop(), "runtime-reused", logDir, RuntimeLogConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeLogger first: %v", err)
	}
	defer first.Close()

	second, err := BuildRuntimeLogger(zap.NewNop(), "runtime-reused", logDir, RuntimeLogConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeLogger second: %v", err)
	}
	defer second.Close()

	if first.Path() == second.Path() {
		t.Fatalf("reused runtime instance ID selected duplicate path %q", first.Path())
	}
	assertPathContainsRuntimeInstanceID(t, first.Path(), "runtime-reused")
	assertPathContainsRuntimeInstanceID(t, second.Path(), "runtime-reused")
}

func TestBuildRuntimeLoggerAvoidsConcurrentPathCollisions(t *testing.T) {
	logDir := t.TempDir()
	const workers = 16
	paths := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sink, err := BuildRuntimeLogger(zap.NewNop(), "runtime-concurrent", logDir, RuntimeLogConfig{})
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
			t.Fatalf("BuildRuntimeLogger concurrent: %v", err)
		}
	}
	seen := map[string]struct{}{}
	for path := range paths {
		assertPathContainsRuntimeInstanceID(t, path, "runtime-concurrent")
		if _, ok := seen[path]; ok {
			t.Fatalf("duplicate concurrent runtime log path %q", path)
		}
		seen[path] = struct{}{}
	}
	if len(seen) != workers {
		t.Fatalf("created %d unique runtime log paths, want %d", len(seen), workers)
	}
}

func assertRuntimeLogPathFormat(t *testing.T, path, rootDir, runtimeInstanceID string, earliest, latest time.Time) {
	t.Helper()

	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		t.Fatalf("runtime log path %q is not below root %q: %v", path, rootDir, err)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 3 {
		t.Fatalf("runtime log relative path = %q, want <yyyy-mm>/<yyyy-mm-dd>/<time-id-unique>.log", rel)
	}
	if ok, err := regexp.MatchString(`^\d{4}-\d{2}$`, parts[0]); err != nil || !ok {
		t.Fatalf("month directory = %q, want yyyy-mm", parts[0])
	}
	if ok, err := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, parts[1]); err != nil || !ok {
		t.Fatalf("date directory = %q, want yyyy-mm-dd", parts[1])
	}
	filenamePattern := regexp.MustCompile(`^(\d{6}\.\d{9})-` + regexp.QuoteMeta(runtimeInstanceID) + `-[A-Za-z0-9_.-]+\.log$`)
	matches := filenamePattern.FindStringSubmatch(parts[2])
	if matches == nil {
		t.Fatalf("runtime log filename = %q, want sortable UTC time, runtime ID, and uniqueness suffix", parts[2])
	}
	if parts[0] != parts[1][:7] {
		t.Fatalf("month directory = %q, want prefix of date directory %q", parts[0], parts[1])
	}
	startedAt, err := time.ParseInLocation("2006-01-02 150405.000000000", parts[1]+" "+matches[1], time.UTC)
	if err != nil {
		t.Fatalf("parse runtime log timestamp from %q: %v", rel, err)
	}
	if startedAt.Before(earliest.Add(-time.Second)) || startedAt.After(latest.Add(time.Second)) {
		t.Fatalf("runtime log timestamp = %s, want between %s and %s", startedAt, earliest, latest)
	}
}

func assertPathContainsRuntimeInstanceID(t *testing.T, path, runtimeInstanceID string) {
	t.Helper()

	if !strings.Contains(filepath.Base(path), "-"+runtimeInstanceID+"-") {
		t.Fatalf("runtime log path %q does not include runtime instance ID %q", path, runtimeInstanceID)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s contents = %q, want %q", path, string(data), want)
	}
}

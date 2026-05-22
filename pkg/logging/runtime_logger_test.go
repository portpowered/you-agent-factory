package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	matches, err := filepath.Glob(filepath.Join(logDir, "runtime-rotates*.log*"))
	if err != nil {
		t.Fatalf("glob runtime log files: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected rotated runtime logs, got %d files: %v", len(matches), matches)
	}

	basePath := filepath.Join(logDir, "runtime-rotates.log")
	for _, path := range matches {
		base := filepath.Base(path)
		if base == filepath.Base(basePath) {
			continue
		}
		if !strings.HasPrefix(base, "runtime-rotates-") {
			t.Fatalf("expected backup file name with timestamp suffix, got %q", base)
		}
	}
}

func TestBuildRuntimeLoggerUsesCanonicalDefaultLogDirectory(t *testing.T) {
	fixture := newRuntimeLogHomeFixture(t)

	sink := fixture.buildDefaultRuntimeLogger("runtime-default-path")
	defer sink.Close()

	wantPath := fixture.canonicalLogPath("runtime-default-path.log")
	if sink.Path() != wantPath {
		t.Fatalf("sink.Path() = %q, want %q", sink.Path(), wantPath)
	}
}

func TestBuildRuntimeLoggerMigratesLegacyDefaultLogDirectory(t *testing.T) {
	fixture := newRuntimeLogHomeFixture(t)

	legacyLogPath := fixture.writeLegacyLog("legacy.log", "legacy runtime log\n")

	sink := fixture.buildDefaultRuntimeLogger("runtime-migrated")
	defer sink.Close()

	if got := sink.Path(); got != fixture.canonicalLogPath("runtime-migrated.log") {
		t.Fatalf("sink.Path() = %q, want canonical runtime log path", got)
	}
	if _, err := os.Stat(fixture.canonicalLogPath("legacy.log")); err != nil {
		t.Fatalf("expected migrated legacy log in canonical dir: %v", err)
	}
	if _, err := os.Stat(legacyLogPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy runtime log path to disappear after migration, stat err = %v", err)
	}
}

func TestBuildRuntimeLoggerKeepsCanonicalDirWhenLegacyDirAlsoExists(t *testing.T) {
	fixture := newRuntimeLogHomeFixture(t)
	legacyLogPath := fixture.writeLegacyLog("legacy.log", "legacy runtime log\n")
	canonicalExistingPath := fixture.writeCanonicalLog("existing.log", "canonical runtime log\n")

	sink := fixture.buildDefaultRuntimeLogger("runtime-canonical-wins")
	defer sink.Close()

	if got := sink.Path(); got != fixture.canonicalLogPath("runtime-canonical-wins.log") {
		t.Fatalf("sink.Path() = %q, want canonical runtime log path", got)
	}
	if _, err := os.Stat(legacyLogPath); err != nil {
		t.Fatalf("expected legacy runtime log to remain when canonical dir already exists: %v", err)
	}
	if _, err := os.Stat(canonicalExistingPath); err != nil {
		t.Fatalf("expected canonical runtime log to remain untouched: %v", err)
	}
}

package runtimeartifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeArtifactPathUsesInjectedClockLayout(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "tmp", "artifacts")
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	logPath := RuntimeArtifactPath(rootDir, at, RuntimeArtifactKindLog, "")
	wantLog := filepath.Join(rootDir, "2026", "05", "29", "044503.000000000-runtime-log.log")
	if logPath != wantLog {
		t.Fatalf("RuntimeArtifactPath(log) = %q, want %q", logPath, wantLog)
	}

	metricsPath := RuntimeArtifactPath(
		rootDir,
		at,
		RuntimeArtifactKindMetrics,
		RuntimeArtifactPathComponents("session-one", "runtime-one", "unique"),
	)
	wantMetrics := filepath.Join(
		rootDir,
		"2026",
		"05",
		"29",
		"044503.000000000-runtime-metrics-session-one-runtime-one-unique.log",
	)
	if metricsPath != wantMetrics {
		t.Fatalf("RuntimeArtifactPath(metrics) = %q, want %q", metricsPath, wantMetrics)
	}
}

func TestRuntimeArtifactPathSharesCalendarDirectoryAcrossKinds(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "tmp", "artifacts")
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	logPath := RuntimeArtifactPath(rootDir, at, RuntimeArtifactKindLog, "runtime-a")
	metricsPath := RuntimeArtifactPath(rootDir, at, RuntimeArtifactKindMetrics, "session-a-runtime-a")

	logRel, err := filepath.Rel(rootDir, logPath)
	if err != nil {
		t.Fatalf("Rel(logPath): %v", err)
	}
	metricsRel, err := filepath.Rel(rootDir, metricsPath)
	if err != nil {
		t.Fatalf("Rel(metricsPath): %v", err)
	}

	logParts := strings.Split(logRel, string(os.PathSeparator))
	metricsParts := strings.Split(metricsRel, string(os.PathSeparator))
	if len(logParts) != 4 || len(metricsParts) != 4 {
		t.Fatalf("relative paths = %q and %q, want four YYYY/MM/DD/filename segments", logRel, metricsRel)
	}
	if logParts[0] != metricsParts[0] || logParts[1] != metricsParts[1] || logParts[2] != metricsParts[2] {
		t.Fatalf("dated directories = %q vs %q, want shared YYYY/MM/DD prefix", logRel, metricsRel)
	}
	if got, want := logParts[0]+"/"+logParts[1]+"/"+logParts[2], "2026/05/29"; got != want {
		t.Fatalf("shared dated directory = %q, want %q", got, want)
	}
}

func TestRuntimeArtifactPathUsesPlatformSeparators(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join("tmp", "runtime-artifacts")
	at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	path := RuntimeArtifactPath(rootDir, at, RuntimeArtifactKindLog, "runtime")
	want := filepath.Join(rootDir, "2026", "01", "02", "030405.000000000-runtime-log-runtime.log")
	if path != want {
		t.Fatalf("RuntimeArtifactPath() = %q, want %q", path, want)
	}
}

func TestRuntimeArtifactPathComponentsSanitizesValues(t *testing.T) {
	t.Parallel()

	got := RuntimeArtifactPathComponents(" session ", "", "runtime/id")
	want := "session-unknown-runtime_id"
	if got != want {
		t.Fatalf("RuntimeArtifactPathComponents() = %q, want %q", got, want)
	}
}

func TestRuntimeArtifactPathWithCollisionPreservesTimeKindPrefix(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join("tmp", "runtime-artifacts")
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)
	suffix := RuntimeArtifactPathComponents("runtime-one", "collision-token")

	basePath := RuntimeArtifactPathWithCollision(rootDir, at, RuntimeArtifactKindLog, suffix, 0)
	collisionPath := RuntimeArtifactPathWithCollision(rootDir, at, RuntimeArtifactKindLog, suffix, 1)

	if basePath == collisionPath {
		t.Fatalf("collision path must differ from base path: %q", basePath)
	}

	baseRel, err := filepath.Rel(rootDir, basePath)
	if err != nil {
		t.Fatalf("Rel(basePath): %v", err)
	}
	collisionRel, err := filepath.Rel(rootDir, collisionPath)
	if err != nil {
		t.Fatalf("Rel(collisionPath): %v", err)
	}

	baseParts := strings.Split(baseRel, string(os.PathSeparator))
	collisionParts := strings.Split(collisionRel, string(os.PathSeparator))
	if len(baseParts) != 4 || len(collisionParts) != 4 {
		t.Fatalf("relative paths = %q and %q, want four YYYY/MM/DD/filename segments", baseRel, collisionRel)
	}
	if baseParts[0] != collisionParts[0] || baseParts[1] != collisionParts[1] || baseParts[2] != collisionParts[2] {
		t.Fatalf("dated directories = %q vs %q, want shared YYYY/MM/DD prefix", baseRel, collisionRel)
	}
	if !strings.HasPrefix(collisionParts[3], "044503.000000000-runtime-log-runtime-one-collision-token-") || !strings.HasSuffix(collisionParts[3], "-1.log") {
		t.Fatalf("collision filename = %q, want time-kind prefix with -1 uniqueness suffix", collisionParts[3])
	}
}

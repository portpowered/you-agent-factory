package defaultpaths

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSharedScopedPaths(t *testing.T) {
	t.Parallel()

	homeDir := filepath.Join(string(filepath.Separator), "tmp", "user-home")

	if got, want := SharedRoot(homeDir), filepath.Join(homeDir, ".you-agent-factory"); got != want {
		t.Fatalf("SharedRoot() = %q, want %q", got, want)
	}
	if got, want := NamedFactoriesRoot(homeDir), filepath.Join(homeDir, ".you-agent-factory", "factories"); got != want {
		t.Fatalf("NamedFactoriesRoot() = %q, want %q", got, want)
	}
	if got, want := OperatorConfigPath(homeDir), filepath.Join(homeDir, ".you-agent-factory", "config.json"); got != want {
		t.Fatalf("OperatorConfigPath() = %q, want %q", got, want)
	}
	if got, want := RecordingsRoot(homeDir), filepath.Join(homeDir, ".you-agent-factory", "recordings"); got != want {
		t.Fatalf("RecordingsRoot() = %q, want %q", got, want)
	}
	if got, want := RuntimeLogsRoot(homeDir), filepath.Join(homeDir, ".you-agent-factory", "logs"); got != want {
		t.Fatalf("RuntimeLogsRoot() = %q, want %q", got, want)
	}
	if got, want := RuntimeMetricsRoot(homeDir), filepath.Join(homeDir, ".you-agent-factory", "metrics"); got != want {
		t.Fatalf("RuntimeMetricsRoot() = %q, want %q", got, want)
	}
}

func TestRecordingsDatedDirPreservesCurrentLayout(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "tmp", "recordings")
	at := time.Date(2026, time.May, 23, 18, 45, 12, 0, time.FixedZone("UTC+7", 7*60*60))

	if got, want := RecordingsDatedDir(rootDir, at), filepath.Join(rootDir, "2026-05", "2026-05-23"); got != want {
		t.Fatalf("RecordingsDatedDir() = %q, want %q", got, want)
	}
}

func TestRuntimeLogsDatedDirUsesUTCDayLayout(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "tmp", "logs")
	at := time.Date(2026, time.May, 23, 0, 15, 0, 0, time.FixedZone("UTC+7", 7*60*60))

	if got, want := RuntimeLogsDatedDir(rootDir, at), filepath.Join(rootDir, "2026", "05", "22"); got != want {
		t.Fatalf("RuntimeLogsDatedDir() = %q, want %q", got, want)
	}
}

func TestRuntimeLogsAndMetricsDatedDirsShareCalendarLayout(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "tmp", "artifacts")
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	logsDir := RuntimeLogsDatedDir(rootDir, at)
	metricsDir := RuntimeMetricsDatedDir(rootDir, at)
	if logsDir != metricsDir {
		t.Fatalf("RuntimeLogsDatedDir() = %q, RuntimeMetricsDatedDir() = %q, want same YYYY/MM/DD layout", logsDir, metricsDir)
	}
	if got, want := logsDir, filepath.Join(rootDir, "2026", "05", "29"); got != want {
		t.Fatalf("shared dated dir = %q, want %q", got, want)
	}
}

func TestRuntimeMetricsDatedDirUsesCurrentYearMonthDayLayoutInUTC(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "tmp", "metrics")
	at := time.Date(2026, time.January, 1, 1, 15, 0, 0, time.FixedZone("UTC+7", 7*60*60))

	if got, want := RuntimeMetricsDatedDir(rootDir, at), filepath.Join(rootDir, "2025", "12", "31"); got != want {
		t.Fatalf("RuntimeMetricsDatedDir() = %q, want %q", got, want)
	}
}

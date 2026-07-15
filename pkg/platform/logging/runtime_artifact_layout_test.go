package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
)

// Regression guard for you-goal-b03-date-layout: runtime logs and metrics must
// share YYYY/MM/DD calendar partitioning, time-kind.ext filenames, collision
// uniqueness, and platform path separators when paths are built from an
// injected clock and reserved on disk.
func TestRuntimeArtifactInjectedClockSharedCalendarLayout(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)
	logRoot := filepath.Join(string(filepath.Separator), "tmp", "runtime-logs")
	metricsRoot := filepath.Join(string(filepath.Separator), "tmp", "runtime-metrics")

	tests := []struct {
		name   string
		root   string
		kind   defaultpaths.RuntimeArtifactKind
		suffix string
	}{
		{
			name:   "runtime log",
			root:   logRoot,
			kind:   defaultpaths.RuntimeArtifactKindLog,
			suffix: defaultpaths.RuntimeArtifactPathComponents("runtime-injected"),
		},
		{
			name: "runtime metrics",
			root: metricsRoot,
			kind: defaultpaths.RuntimeArtifactKindMetrics,
			suffix: defaultpaths.RuntimeArtifactPathComponents(
				"session-injected",
				"runtime-injected",
				"layout-token",
			),
		},
	}

	type datedDirCase struct {
		root string
		dir  string
	}
	var datedDirs []datedDirCase
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			path := defaultpaths.RuntimeArtifactPath(tc.root, at, tc.kind, tc.suffix)
			assertInjectedClockSharedLayoutPath(t, tc.root, path, at, tc.kind)
			assertPathUsesPlatformSeparators(t, path)

			datedDir := defaultpaths.RuntimeLogsDatedDir(tc.root, at)
			if tc.kind == defaultpaths.RuntimeArtifactKindMetrics {
				datedDir = defaultpaths.RuntimeMetricsDatedDir(tc.root, at)
			}
			if datedDir != filepath.Dir(path) {
				t.Fatalf("dated dir = %q, want parent of path %q", datedDir, path)
			}
			datedDirs = append(datedDirs, datedDirCase{root: tc.root, dir: datedDir})
		})
	}

	wantDate := at.UTC().Format("2006/01/02")
	for i, dated := range datedDirs {
		rel, err := filepath.Rel(dated.root, dated.dir)
		if err != nil {
			t.Fatalf("Rel(datedDir[%d]): %v", i, err)
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) != 3 {
			t.Fatalf("dated dir relative layout = %q, want YYYY/MM/DD", rel)
		}
		if got := parts[0] + "/" + parts[1] + "/" + parts[2]; got != wantDate {
			t.Fatalf("dated dir[%d] calendar suffix = %q, want %q", i, got, wantDate)
		}
	}
}

func TestRuntimeArtifactInjectedClockReservesFilesOnFreshRoot(t *testing.T) {
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	tests := []struct {
		name   string
		kind   defaultpaths.RuntimeArtifactKind
		suffix string
	}{
		{
			name:   "runtime log",
			kind:   defaultpaths.RuntimeArtifactKindLog,
			suffix: defaultpaths.RuntimeArtifactPathComponents("runtime-fresh-layout"),
		},
		{
			name: "runtime metrics",
			kind: defaultpaths.RuntimeArtifactKindMetrics,
			suffix: defaultpaths.RuntimeArtifactPathComponents(
				"session-fresh-layout",
				"runtime-fresh-layout",
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()
			assertRuntimeArtifactRootLacksCalendarDirectories(t, rootDir)

			path, err := runtimeartifact.ReserveAvailablePath(rootDir, at, tc.kind, tc.suffix)
			if err != nil {
				t.Fatalf("runtimeartifact.ReserveAvailablePath: %v", err)
			}

			assertInjectedClockSharedLayoutPath(t, rootDir, path, at, tc.kind)
			assertPathUsesPlatformSeparators(t, path)
			assertRuntimeArtifactDatedDirPresent(t, filepath.Dir(path))
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("reserved file %q should exist: %v", path, err)
			}
		})
	}
}

func TestRuntimeArtifactInjectedClockCollisionPreservesTimeKindShape(t *testing.T) {
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	tests := []struct {
		name   string
		kind   defaultpaths.RuntimeArtifactKind
		suffix string
	}{
		{
			name:   "runtime log",
			kind:   defaultpaths.RuntimeArtifactKindLog,
			suffix: defaultpaths.RuntimeArtifactPathComponents("runtime-collision-layout"),
		},
		{
			name: "runtime metrics",
			kind: defaultpaths.RuntimeArtifactKindMetrics,
			suffix: defaultpaths.RuntimeArtifactPathComponents(
				"session-collision-layout",
				"runtime-collision-layout",
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()

			firstPath, err := runtimeartifact.ReserveAvailablePath(rootDir, at, tc.kind, tc.suffix)
			if err != nil {
				t.Fatalf("runtimeartifact.ReserveAvailablePath first: %v", err)
			}
			if err := os.WriteFile(firstPath, []byte("preserved"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", firstPath, err)
			}

			secondPath, err := runtimeartifact.ReserveAvailablePath(rootDir, at, tc.kind, tc.suffix)
			if err != nil {
				t.Fatalf("runtimeartifact.ReserveAvailablePath second: %v", err)
			}
			if firstPath == secondPath {
				t.Fatalf("collision paths must differ, both %q", firstPath)
			}

			assertInjectedClockSharedLayoutPath(t, rootDir, firstPath, at, tc.kind)
			assertInjectedClockSharedLayoutPath(t, rootDir, secondPath, at, tc.kind)
			assertRuntimeArtifactCollisionPath(t, secondPath, rootDir, at, tc.kind, tc.suffix, 1)
			assertFileContents(t, firstPath, "preserved")
		})
	}
}

func assertInjectedClockSharedLayoutPath(
	t *testing.T,
	rootDir string,
	path string,
	at time.Time,
	kind defaultpaths.RuntimeArtifactKind,
) {
	t.Helper()

	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		t.Fatalf("Rel(%q) from root %q: %v", path, rootDir, err)
	}

	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 4 {
		t.Fatalf("relative path = %q, want YYYY/MM/DD/<time-kind[...]>.log", rel)
	}

	wantDate := at.UTC().Format("2006/01/02")
	if got := parts[0] + "/" + parts[1] + "/" + parts[2]; got != wantDate {
		t.Fatalf("dated directory = %q, want %q", got, wantDate)
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

	timeToken := at.UTC().Format(defaultpaths.RuntimeArtifactTimeLayout)
	filenamePattern := regexp.MustCompile(
		`^` + regexp.QuoteMeta(timeToken) + `-` + regexp.QuoteMeta(string(kind)) + `(-[A-Za-z0-9_.-]+)?(-\d+)?\.log$`,
	)
	if !filenamePattern.MatchString(parts[3]) {
		t.Fatalf("filename = %q, want %s-<kind>[suffix][-N].log shape", parts[3], timeToken)
	}
}

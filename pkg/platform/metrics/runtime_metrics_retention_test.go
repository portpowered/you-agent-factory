package metrics

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

func TestRuntimeMetricsRootRetentionPrunesDifferentCurrentFilenamesAcrossDates(t *testing.T) {
	root := t.TempDir()
	expired := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "historical-session-runtime-old-collision", 11)
	recent := writeRetentionArtifact(t, root, "2026/08/20", "020000.000000000", "new-session-runtime-new-collision", 17)
	unrecognized := filepath.Join(root, "2026", "07", "01", "keep.txt")
	if err := os.WriteFile(unrecognized, []byte("leave me"), 0o600); err != nil {
		t.Fatalf("WriteFile(unrecognized): %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 10, MaxBackups: 1, MaxAge: 30},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}

	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired artifact exists after root sweep, stat error = %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent artifact was removed by root sweep: %v", err)
	}
	if _, err := os.Stat(unrecognized); err != nil {
		t.Fatalf("unrecognized file was changed by root sweep: %v", err)
	}

	if report.Before != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 28}) {
		t.Fatalf("Before = %#v, want two recognized files and 28 bytes", report.Before)
	}
	if report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 17}) {
		t.Fatalf("After = %#v, want recent artifact only", report.After)
	}
	if report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 11}) {
		t.Fatalf("Removed = %#v, want expired artifact totals", report.Removed)
	}
	if report.Protected != (RuntimeMetricsRetentionTotals{}) || report.Failed != (RuntimeMetricsRetentionTotals{}) {
		t.Fatalf("unexpected protected/failed totals: protected=%#v failed=%#v", report.Protected, report.Failed)
	}
}

func TestRuntimeMetricsRootRetentionAppliesAgeThenOldestFirstAggregateSize(t *testing.T) {
	root := t.TempDir()
	oldest := writeRetentionArtifact(t, root, "2026/08/01", "010000.000000000", "session-a-runtime-a-unique-a", 600_000)
	middle := writeRetentionArtifact(t, root, "2026/08/10", "020000.000000000", "session-b-runtime-b-unique-b", 600_000)
	recent := writeRetentionArtifact(t, root, "2026/08/20", "030000.000000000", "session-c-runtime-c-unique-c", 600_000)

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 365},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}

	for name, path := range map[string]string{"oldest": oldest, "middle": middle} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s artifact remains after aggregate-size pruning, stat error = %v", name, err)
		}
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("newest artifact was not retained within aggregate budget: %v", err)
	}
	if report.Before != (RuntimeMetricsRetentionTotals{Files: 3, Bytes: 1_800_000}) {
		t.Fatalf("Before = %#v, want three artifacts and 1,800,000 bytes", report.Before)
	}
	if report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 600_000}) {
		t.Fatalf("After = %#v, want newest artifact only", report.After)
	}
	if report.Removed != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 1_200_000}) {
		t.Fatalf("Removed = %#v, want oldest two artifacts", report.Removed)
	}
}

func TestRuntimeMetricsRootRetentionDoesNotRemoveUnknownEntriesRecursively(t *testing.T) {
	root := t.TempDir()
	unsafeDir := filepath.Join(root, "2026", "07", "01")
	if err := os.MkdirAll(unsafeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unsafeDir): %v", err)
	}
	unsafeArtifact := filepath.Join(unsafeDir, "010000.000000000-runtime-metrics-session-unsafe-runtime-unsafe.log")
	if err := os.WriteFile(unsafeArtifact, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile(unsafeArtifact): %v", err)
	}
	unknown := filepath.Join(unsafeDir, "operator-note.txt")
	if err := os.WriteFile(unknown, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("WriteFile(unknown): %v", err)
	}

	safeArtifact := writeRetentionArtifact(t, root, "2026/07/02", "020000.000000000", "session-safe-runtime-safe-collision", 3)
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	_, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 10, MaxBackups: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}

	if _, err := os.Stat(unsafeArtifact); !os.IsNotExist(err) {
		t.Fatalf("eligible artifact in mixed directory remains, stat error = %v", err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown file was removed from mixed directory: %v", err)
	}
	if _, err := os.Stat(unsafeDir); err != nil {
		t.Fatalf("mixed date directory should remain for unknown file: %v", err)
	}
	if _, err := os.Stat(safeArtifact); !os.IsNotExist(err) {
		t.Fatalf("eligible complete date directory artifact remains, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(safeArtifact)); !os.IsNotExist(err) {
		t.Fatalf("complete eligible date directory remains, stat error = %v", err)
	}
}

func TestRuntimeMetricsRootRetentionDoesNotFollowRecognizedSymlink(t *testing.T) {
	root := t.TempDir()
	target := writeRetentionArtifact(t, t.TempDir(), "2026/07/01", "010000.000000000", "outside-runtime-target", 5)
	linkDir := filepath.Join(root, "2026", "07", "01")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir): %v", err)
	}
	link := filepath.Join(linkDir, "010000.000000000-runtime-metrics-session-link-runtime-link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("recognized symlink was removed or followed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target was changed by root sweep: %v", err)
	}
	if report.Protected.Files != 1 || report.Protected.Bytes != 5 {
		t.Fatalf("Protected = %#v, want recognized symlink counted as protected", report.Protected)
	}
}

func TestRuntimeMetricsRootRetentionReportsDeterministicTotals(t *testing.T) {
	firstRoot := installRetentionReportFixture(t)
	secondRoot := installRetentionReportFixture(t)
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	first := newTestRuntimeMetricsRetention(t, now)
	second := newTestRuntimeMetricsRetention(t, now)
	request := func(root string) RuntimeMetricsRetentionRequest {
		return RuntimeMetricsRetentionRequest{
			RootDirectory: root,
			Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 30},
		}
	}
	firstReport, err := first.Sweep(t.Context(), request(firstRoot))
	if err != nil {
		t.Fatalf("first Sweep(): %v", err)
	}
	secondReport, err := second.Sweep(t.Context(), request(secondRoot))
	if err != nil {
		t.Fatalf("second Sweep(): %v", err)
	}
	firstReport.RootDirectory = ""
	secondReport.RootDirectory = ""
	if !reflect.DeepEqual(firstReport, secondReport) {
		t.Fatalf("reports differ for equivalent root states:\nfirst=%#v\nsecond=%#v", firstReport, secondReport)
	}
}

func newTestRuntimeMetricsRetention(t *testing.T, now time.Time) *RuntimeMetricsRetention {
	t.Helper()
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	return retention
}

func writeRetentionArtifact(t *testing.T, root, date, clock, suffix string, size int) string {
	t.Helper()
	datePath := filepath.Join(root, filepath.FromSlash(date))
	if err := os.MkdirAll(datePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", datePath, err)
	}
	path := filepath.Join(datePath, clock+"-runtime-metrics-"+suffix+".log")
	if err := os.WriteFile(path, bytes.Repeat([]byte("m"), size), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}

func installRetentionReportFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "session-old-runtime-old-collision", 600_000)
	writeRetentionArtifact(t, root, "2026/08/20", "020000.000000000", "session-recent-runtime-recent-collision", 600_000)
	return root
}

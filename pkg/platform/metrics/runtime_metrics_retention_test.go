package metrics

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func TestRuntimeMetricsRootRetentionProtectsOpenWriterAcrossFactoryIdentities(t *testing.T) {
	root := t.TempDir()
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver(): %v", err)
	}
	firstOpener := newRetentionTestOpener(t, paths)
	secondOpener := newRetentionTestOpener(t, paths)
	started := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	live := openRetentionTestSink(t, firstOpener, RuntimeMetricsOpeningRequest{
		SessionID:         "factory-one-session",
		RuntimeInstanceID: "factory-one-runtime",
		RootDirectory:     root,
		StartTimeUTC:      started,
		CollisionID:       "live",
		Config:            RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	defer live.Close()
	writeRetentionMetric(t, live, "live.before_sweep")
	claimPath := runtimeMetricsClaimPath(live.Path())
	assertRetentionPathExists(t, claimPath, "active claim before sweep")

	retention, err := NewRuntimeMetricsRetention(
		platformfilesystem.Local{},
		func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	// The second opener models another factory/process sharing the user-global
	// root; the sweep must respect the first opener's OS-held claim.
	other := openRetentionTestSink(t, secondOpener, RuntimeMetricsOpeningRequest{
		SessionID:         "factory-two-session",
		RuntimeInstanceID: "factory-two-runtime",
		RootDirectory:     root,
		StartTimeUTC:      time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC),
		CollisionID:       "other",
		Config:            RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	defer other.Close()
	writeRetentionMetric(t, other, "other.factory")
	report, err := retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	assertOpenWriterSweep(t, report, err, live.Path())
	writeRetentionMetric(t, live, "live.after_sweep")
	assertRetentionRecordCount(t, live.Path(), 2)

	if err := live.Close(); err != nil {
		t.Fatalf("close live metrics sink: %v", err)
	}
	assertRetentionPathExists(t, claimPath, "stable claim marker after close")
	report, err = retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	assertClosedRetentionSweep(t, report, err, live.Path())
}

func newRetentionTestOpener(t *testing.T, paths platformartifact.Reserver) *RuntimeMetricsOpener {
	t.Helper()
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	scheduler, err := NewRuntimeMetricsRetentionScheduler(retention, nil, nil)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetentionScheduler(): %v", err)
	}
	opener, err := NewRuntimeMetricsOpener(paths, scheduler, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsOpener(): %v", err)
	}
	return opener
}

func openRetentionTestSink(
	t *testing.T,
	opener *RuntimeMetricsOpener,
	request RuntimeMetricsOpeningRequest,
) *RuntimeMetricsSink {
	t.Helper()
	sink, err := opener.Open(request)
	if err != nil {
		t.Fatalf("open runtime metrics sink: %v", err)
	}
	return sink
}

func writeRetentionMetric(t *testing.T, sink *RuntimeMetricsSink, name string) {
	t.Helper()
	if err := sink.WriteMetric(context.Background(), map[string]any{"metric_name": name}); err != nil {
		t.Fatalf("write metric %q: %v", name, err)
	}
}

func assertRetentionPathExists(t *testing.T, path, description string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s %q unavailable: %v", description, path, err)
	}
}

func assertRetentionPathAbsent(t *testing.T, path, description string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s %q remains, stat error = %v", description, path, err)
	}
}

func assertRetentionRecordCount(t *testing.T, path string, want int) {
	t.Helper()
	if got := len(readRuntimeMetricsRecords(t, path)); got != want {
		t.Fatalf("metrics record count = %d, want %d", got, want)
	}
}

func assertOpenWriterSweep(t *testing.T, report RuntimeMetricsRetentionReport, err error, livePath string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Sweep(open writer): %v", err)
	}
	if report.Protected.Files != 1 {
		t.Fatalf("Protected.Files = %d, want one active writer", report.Protected.Files)
	}
	assertRetentionPathExists(t, livePath, "live metrics path during sweep")
}

func assertClosedRetentionSweep(t *testing.T, report RuntimeMetricsRetentionReport, err error, livePath string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Sweep(after close): %v", err)
	}
	if report.Removed.Files == 0 {
		t.Fatalf("Sweep(after close) = %#v, want closed artifact removal", report)
	}
	assertRetentionPathAbsent(t, livePath, "closed expired metrics path")
}

func TestRuntimeMetricsRootRetentionYieldsWhenRootSweepIsAlreadyClaimed(t *testing.T) {
	root := t.TempDir()
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	rootLock, err := coordination.LockRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("LockRoot(): %v", err)
	}
	defer rootLock.Close()

	retention, err := NewRuntimeMetricsRetention(
		platformfilesystem.Local{},
		func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
		coordination,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	report, err := retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(held root): %v", err)
	}
	if !report.Skipped {
		t.Fatal("Sweep(held root) skipped = false, want safe yield")
	}
}

func TestRuntimeMetricsRootRetentionReclaimsReleasedStaleClaim(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "stale-claim-runtime-stale-collision", 9)
	claimPath := runtimeMetricsClaimPath(artifact)
	if err := os.MkdirAll(filepath.Dir(claimPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(claim directory): %v", err)
	}
	if err := os.WriteFile(claimPath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile(stale claim): %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	if _, err := retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	}); err != nil {
		t.Fatalf("Sweep(stale claim): %v", err)
	}
	if _, err := os.Stat(artifact); !os.IsNotExist(err) {
		t.Fatalf("artifact protected by released stale claim, stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("stable stale claim marker was not reusable, stat error = %v", err)
	}
}

func TestRuntimeMetricsCoordinationClaimClosePreservesConcurrentReplacement(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(root, "2026", "08", "22", "010000.000000000-runtime-metrics-session-runtime-collision.log")
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	initial, err := coordination.Claim(artifact)
	if err != nil {
		t.Fatalf("Claim(initial): %v", err)
	}
	claimPath := runtimeMetricsClaimPath(artifact)
	assertRetentionPathExists(t, claimPath, "initial stable claim marker")

	replacementReady := make(chan io.Closer, 1)
	replacementErr := make(chan error, 1)
	startReplacement := make(chan struct{})
	go func() {
		<-startReplacement
		for {
			replacement, claimErr := coordination.TryClaim(artifact)
			if claimErr == nil {
				replacementReady <- replacement
				return
			}
			if !errors.Is(claimErr, ErrRuntimeMetricsArtifactBusy) {
				replacementErr <- claimErr
				return
			}
			runtime.Gosched()
		}
	}()
	close(startReplacement)
	if err := initial.Close(); err != nil {
		t.Fatalf("Close(initial claim): %v", err)
	}

	var replacement io.Closer
	select {
	case err := <-replacementErr:
		t.Fatalf("TryClaim(replacement): %v", err)
	case replacement = <-replacementReady:
	}
	if err := replacement.Close(); err != nil {
		t.Fatalf("Close(replacement claim): %v", err)
	}
	assertRetentionPathExists(t, claimPath, "replacement stable claim marker")
}

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
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(recognized symlink): %v", err)
	}
	if report.Protected.Files != 1 || report.Protected.Bytes != linkInfo.Size() {
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

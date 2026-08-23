package metrics

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func TestRuntimeMetricsRetentionSchedulerRunsStartupAndOneSharedPeriodicLoop(t *testing.T) {
	root := t.TempDir()
	writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "startup-old-runtime-old-collision", 11)
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 4)

	firstLease := harness.start(t, root)
	assertRetentionReportRemoved(t, harness.next(t), 1)

	secondLease := harness.start(t, root)
	assertNoRetentionReport(t, harness.reports)
	assertHourlyTicker(t, harness.intervals)

	harness.ticker.Tick(time.Now())
	assertEmptyRetentionReport(t, harness.next(t))

	if err := firstLease.Close(); err != nil {
		t.Fatalf("close first lease: %v", err)
	}
	if harness.ticker.Stopped() {
		t.Fatal("shared ticker stopped while second sink lease remained")
	}
	if err := secondLease.Close(); err != nil {
		t.Fatalf("close second lease: %v", err)
	}
	if !harness.ticker.Stopped() {
		t.Fatal("ticker remained active after final sink lease closed")
	}
	if err := harness.scheduler.Close(context.Background()); err != nil {
		t.Fatalf("scheduler.Close(): %v", err)
	}
}

func TestRuntimeMetricsOpenerRunsStartupSweepBeforeFreshUniqueFile(t *testing.T) {
	root := t.TempDir()
	old := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "historical-runtime-historical-collision", 13)
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 2)
	opener := newRuntimeMetricsTestOpener(t, harness.scheduler)

	sink, err := opener.Open(RuntimeMetricsOpeningRequest{
		SessionID:         "new-session",
		RuntimeInstanceID: "new-runtime",
		RootDirectory:     root,
		StartTimeUTC:      time.Date(2026, 7, 1, 11, 0, 0, 0, time.UTC),
		CollisionID:       "new-collision",
		Config:            RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	assertRetentionReportBefore(t, harness.next(t), RuntimeMetricsRetentionTotals{Files: 1, Bytes: 13})
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("historical artifact stat error = %v, want removed", err)
	}
	if _, err := os.Stat(sink.Path()); err != nil {
		t.Fatalf("fresh metrics path missing after startup sweep: %v", err)
	}
	if err := sink.WriteMetric(context.Background(), map[string]any{"metric_name": "before_periodic"}); err != nil {
		t.Fatalf("write before periodic sweep: %v", err)
	}
	harness.ticker.Tick(time.Now())
	assertRetentionReportProtected(t, harness.next(t), 1)
	if err := sink.WriteMetric(context.Background(), map[string]any{"metric_name": "after_periodic"}); err != nil {
		t.Fatalf("write after periodic sweep: %v", err)
	}
	if records := readRuntimeMetricsRecords(t, sink.Path()); len(records) != 2 {
		t.Fatalf("active sink records = %d, want continued writes across periodic sweep", len(records))
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("sink.Close(): %v", err)
	}
	if err := opener.Close(context.Background()); err != nil {
		t.Fatalf("opener.Close(): %v", err)
	}
}

func TestRuntimeMetricsRetentionSchedulerRetriesFailedCandidateOnNextTick(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "retry-runtime-retry-collision", 19)
	filesystem := &retryRuntimeMetricsRetentionFileSystem{
		Local:        platformfilesystem.Local{},
		failPath:     artifact,
		failuresLeft: 1,
	}
	retention, err := NewRuntimeMetricsRetention(
		filesystem,
		func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 4)
	lease := harness.start(t, root)
	assertRetentionReportFailed(t, harness.next(t))
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("failed candidate disappeared before retry: %v", err)
	}

	harness.ticker.Tick(time.Now())
	assertRetentionReportRemovedAfterRetry(t, harness.next(t))
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retried artifact stat error = %v, want removed", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("lease.Close(): %v", err)
	}
	if err := harness.scheduler.Close(context.Background()); err != nil {
		t.Fatalf("scheduler.Close(): %v", err)
	}
}

type runtimeMetricsRetentionSchedulerHarness struct {
	scheduler *RuntimeMetricsRetentionScheduler
	ticker    *manualRuntimeMetricsRetentionTicker
	reports   chan runtimeMetricsRetentionObservation
	intervals []time.Duration
}

func newRuntimeMetricsRetentionSchedulerHarness(
	t *testing.T,
	retention *RuntimeMetricsRetention,
	reportCapacity int,
) *runtimeMetricsRetentionSchedulerHarness {
	t.Helper()
	harness := runtimeMetricsRetentionSchedulerHarness{
		ticker:  newManualRuntimeMetricsRetentionTicker(),
		reports: make(chan runtimeMetricsRetentionObservation, reportCapacity),
	}
	var err error
	harness.scheduler, err = NewRuntimeMetricsRetentionScheduler(
		retention,
		func(interval time.Duration) RuntimeMetricsRetentionTicker {
			harness.intervals = append(harness.intervals, interval)
			return harness.ticker
		},
		func(report RuntimeMetricsRetentionReport, sweepErr error) {
			harness.reports <- runtimeMetricsRetentionObservation{report: report, err: sweepErr}
		},
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetentionScheduler(): %v", err)
	}
	return &harness
}

func (harness *runtimeMetricsRetentionSchedulerHarness) start(t *testing.T, root string) io.Closer {
	t.Helper()
	lease, err := harness.scheduler.Start(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Start(%q): %v", root, err)
	}
	return lease
}

func (harness *runtimeMetricsRetentionSchedulerHarness) next(t *testing.T) runtimeMetricsRetentionObservation {
	t.Helper()
	return receiveRetentionReport(t, harness.reports)
}

func newRuntimeMetricsTestOpener(
	t *testing.T,
	scheduler *RuntimeMetricsRetentionScheduler,
) *RuntimeMetricsOpener {
	t.Helper()
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver(): %v", err)
	}
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	opener, err := NewRuntimeMetricsOpenerWithRetention(paths, scheduler, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsOpenerWithRetention(): %v", err)
	}
	return opener
}

func assertRetentionReportRemoved(
	t *testing.T,
	observation runtimeMetricsRetentionObservation,
	wantFiles int,
) {
	t.Helper()
	if observation.err != nil || observation.report.Removed.Files != wantFiles {
		t.Fatalf("retention report = %#v, want %d removed files", observation, wantFiles)
	}
}

func assertRetentionReportBefore(
	t *testing.T,
	observation runtimeMetricsRetentionObservation,
	want RuntimeMetricsRetentionTotals,
) {
	t.Helper()
	if observation.err != nil || observation.report.Before != want {
		t.Fatalf("retention report = %#v, want before totals %#v", observation, want)
	}
}

func assertRetentionReportProtected(
	t *testing.T,
	observation runtimeMetricsRetentionObservation,
	wantFiles int,
) {
	t.Helper()
	if observation.err != nil || observation.report.Protected.Files != wantFiles {
		t.Fatalf("periodic retention report = %#v, want %d protected files", observation, wantFiles)
	}
}

func assertRetentionReportFailed(t *testing.T, observation runtimeMetricsRetentionObservation) {
	t.Helper()
	if observation.err != nil || observation.report.Failed.Files != 1 || observation.report.Removed.Files != 0 {
		t.Fatalf("failed retention report = %#v, want one failure and no removal", observation)
	}
}

func assertRetentionReportRemovedAfterRetry(t *testing.T, observation runtimeMetricsRetentionObservation) {
	t.Helper()
	if observation.err != nil || observation.report.Removed.Files != 1 || observation.report.After.Files != 0 {
		t.Fatalf("retry retention report = %#v, want one removal and no remaining files", observation)
	}
}

func assertEmptyRetentionReport(t *testing.T, observation runtimeMetricsRetentionObservation) {
	t.Helper()
	if observation.err != nil {
		t.Fatalf("periodic retention report error = %v", observation.err)
	}
	if observation.report.Before != (RuntimeMetricsRetentionTotals{}) || observation.report.After != (RuntimeMetricsRetentionTotals{}) {
		t.Fatalf("periodic retention report = %#v, want empty totals", observation.report)
	}
}

func assertNoRetentionReport(t *testing.T, reports <-chan runtimeMetricsRetentionObservation) {
	t.Helper()
	select {
	case unexpected := <-reports:
		t.Fatalf("shared start emitted duplicate startup report: %#v", unexpected)
	default:
	}
}

func assertHourlyTicker(t *testing.T, intervals []time.Duration) {
	t.Helper()
	if len(intervals) != 1 || intervals[0] != time.Hour {
		t.Fatalf("ticker intervals = %v, want one hourly ticker", intervals)
	}
}

type runtimeMetricsRetentionObservation struct {
	report RuntimeMetricsRetentionReport
	err    error
}

func receiveRetentionReport(
	t *testing.T,
	reports <-chan runtimeMetricsRetentionObservation,
) runtimeMetricsRetentionObservation {
	t.Helper()
	select {
	case report := <-reports:
		return report
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime metrics retention report")
		return runtimeMetricsRetentionObservation{}
	}
}

type manualRuntimeMetricsRetentionTicker struct {
	ticks   chan time.Time
	mu      sync.Mutex
	stopped bool
}

func newManualRuntimeMetricsRetentionTicker() *manualRuntimeMetricsRetentionTicker {
	return &manualRuntimeMetricsRetentionTicker{ticks: make(chan time.Time, 1)}
}

func (ticker *manualRuntimeMetricsRetentionTicker) C() <-chan time.Time {
	return ticker.ticks
}

func (ticker *manualRuntimeMetricsRetentionTicker) Stop() {
	ticker.mu.Lock()
	ticker.stopped = true
	ticker.mu.Unlock()
}

func (ticker *manualRuntimeMetricsRetentionTicker) Stopped() bool {
	ticker.mu.Lock()
	defer ticker.mu.Unlock()
	return ticker.stopped
}

func (ticker *manualRuntimeMetricsRetentionTicker) Tick(at time.Time) {
	ticker.ticks <- at
}

type retryRuntimeMetricsRetentionFileSystem struct {
	platformfilesystem.Local
	failPath     string
	mu           sync.Mutex
	failuresLeft int
}

func (filesystem *retryRuntimeMetricsRetentionFileSystem) Remove(path string) error {
	filesystem.mu.Lock()
	if path == filesystem.failPath && filesystem.failuresLeft > 0 {
		filesystem.failuresLeft--
		filesystem.mu.Unlock()
		return errors.New("transient retention remove failure")
	}
	filesystem.mu.Unlock()
	return filesystem.Local.Remove(path)
}

package metrics

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func TestRuntimeMetricsTickerDeliversSelectedDeadlinesAndStops(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	source := &retentionTickerTimerSource{
		Deterministic: platformclock.NewDeterministic(base, time.Minute),
		created:       make(chan platformclock.Timer, 4),
	}
	ticker := newRuntimeMetricsRetentionTicker(time.Hour, source)
	t.Cleanup(ticker.Stop)
	first := <-source.created
	source.SetTick(59)
	select {
	case <-ticker.C():
		t.Fatal("retention tick arrived before the selected hourly deadline")
	default:
	}
	source.SetTick(60)
	assertSelectedRetentionTick(t, ticker, base.Add(time.Hour))
	second := <-source.created
	if first.Stop() {
		t.Fatal("delivered retention timer remained active after rearm")
	}
	source.SetTick(120)
	assertSelectedRetentionTick(t, ticker, base.Add(2*time.Hour))
	last := <-source.created
	if second.Stop() {
		t.Fatal("second delivered retention timer remained active after rearm")
	}
	ticker.Stop()
	ticker.Stop()
	if last.Stop() {
		t.Fatal("ticker shutdown left the pending process timer active")
	}
	source.SetTick(180)
	select {
	case <-ticker.C():
		t.Fatal("stopped retention ticker delivered another tick")
	default:
	}
}

type retentionTickerTimerSource struct {
	*platformclock.Deterministic
	created chan platformclock.Timer
}

func (source *retentionTickerTimerSource) NewTimer(duration time.Duration) platformclock.Timer {
	timer := source.Deterministic.NewTimer(duration)
	source.created <- timer
	return timer
}

func assertSelectedRetentionTick(t *testing.T, ticker RuntimeMetricsRetentionTicker, want time.Time) {
	t.Helper()
	// The deadline is logical; wall time only bounds a failed synchronization.
	select {
	case got := <-ticker.C():
		if !got.Equal(want) {
			t.Fatalf("retention tick = %v, want selected deadline %v", got, want)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("selected retention deadline was not delivered")
	}
}

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

func TestRuntimeMetricsRetentionSchedulerValidatesConfiguration(t *testing.T) {
	if scheduler, err := NewRuntimeMetricsRetentionScheduler(nil, nil, nil, nil); scheduler != nil || err == nil || !strings.Contains(err.Error(), "retention is required") {
		t.Fatalf("NewRuntimeMetricsRetentionScheduler(nil) = (%#v, %v)", scheduler, err)
	}
	var nilScheduler *RuntimeMetricsRetentionScheduler
	if err := nilScheduler.Close(context.Background()); err != nil {
		t.Fatalf("nil scheduler Close() = %v, want nil", err)
	}
	if _, err := nilScheduler.Start(context.Background(), RuntimeMetricsRetentionRequest{RootDirectory: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "scheduler is not configured") {
		t.Fatalf("nil scheduler Start() = %v, want configuration error", err)
	}
}

// Whole-sweep failures are reported without retiring the root's lease. A
// manually delivered tick proves recovery without waiting for wall time.
func TestRuntimeMetricsRetentionSchedulerRecoversRootInspectionFailures(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"startup", "periodic"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			root, artifact, marker, unknown := installOrphanMarkerRecoveryFixture(t)
			cause := errors.New("root inspection rejected")
			filesystem := &retentionFailureFileSystem{Local: platformfilesystem.Local{}, lstatErr: cause}
			retention, err := NewRuntimeMetricsRetention(filesystem, func() time.Time {
				return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			})
			if err != nil {
				t.Fatal(err)
			}
			harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 3)
			t.Cleanup(func() {
				if err := harness.scheduler.Close(t.Context()); err != nil {
					t.Errorf("scheduler cleanup: %v", err)
				}
			})
			if stage == "startup" {
				filesystem.failPath = root
			}
			lease := harness.start(t, root)
			if stage == "periodic" {
				assertRetentionReportBefore(t, harness.next(t), RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9})
				filesystem.failPath = root
				harness.ticker.Tick(time.Now())
			}
			failed := harness.next(t)
			if !errors.Is(failed.err, cause) || failed.report.RootDirectory != root || failed.report.Removed.Files != 0 {
				t.Fatalf("failed %s sweep = %#v, want selected root and inspection cause", stage, failed)
			}
			if stage == "startup" {
				assertRetentionPathExists(t, marker, "orphan marker after failed startup")
			}
			assertRetentionPreservedContent(t, artifact, "mmmmmmmmm")
			assertRetentionPreservedContent(t, unknown, "preserve customer content")
			assertHourlyTicker(t, harness.intervals)
			filesystem.failPath = ""
			harness.ticker.Tick(time.Now())
			recovered := harness.next(t)
			if recovered.err != nil || len(recovered.report.Failures) != 0 || recovered.report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) {
				t.Fatalf("recovered %s sweep = %#v", stage, recovered)
			}
			assertRetentionPathAbsent(t, marker, "orphan marker after recovery")
			assertRetentionPreservedContent(t, artifact, "mmmmmmmmm")
			assertRetentionPreservedContent(t, unknown, "preserve customer content")
			if err := lease.Close(); err != nil || !harness.ticker.Stopped() {
				t.Fatalf("recovered lease close = %v, stopped=%v", err, harness.ticker.Stopped())
			}
		})
	}
}

func TestRuntimeMetricsRetentionSchedulerCanceledStartupCanRetry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 3)
	t.Cleanup(func() {
		if err := harness.scheduler.Close(t.Context()); err != nil {
			t.Errorf("scheduler cleanup: %v", err)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Startup publication is synchronous and precedes timer construction. This
	// observed sweep boundary cancels the caller without a production hook.
	reporter := harness.scheduler.reporter
	harness.scheduler.reporter = func(report RuntimeMetricsRetentionReport, err error) {
		reporter(report, err)
		cancel()
	}
	lease, err := harness.scheduler.Start(ctx, RuntimeMetricsRetentionRequest{RootDirectory: root})
	if lease != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v, %v, want no lease and cancellation", lease, err)
	}
	assertEmptyRetentionReport(t, harness.next(t))
	if len(harness.intervals) != 0 {
		t.Fatalf("canceled startup created timers: %v", harness.intervals)
	}
	harness.scheduler.reporter = reporter
	lease = harness.start(t, root)
	assertEmptyRetentionReport(t, harness.next(t))
	assertHourlyTicker(t, harness.intervals)
	harness.ticker.Tick(time.Now())
	assertEmptyRetentionReport(t, harness.next(t))
	if err := lease.Close(); err != nil || !harness.ticker.Stopped() {
		t.Fatalf("retried lease close = %v, stopped=%v", err, harness.ticker.Stopped())
	}
}

func TestRuntimeMetricsRetentionSchedulerInvalidTickerCanRetry(t *testing.T) {
	t.Parallel()
	root, artifact, marker, unknown := installOrphanMarkerRecoveryFixture(t)
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 3)
	t.Cleanup(func() {
		if err := harness.scheduler.Close(t.Context()); err != nil {
			t.Errorf("scheduler cleanup: %v", err)
		}
	})
	invalid := newManualRuntimeMetricsRetentionTicker()
	invalid.ticks = nil
	factory := harness.scheduler.tickerFactory
	harness.scheduler.tickerFactory = func(interval time.Duration) RuntimeMetricsRetentionTicker {
		if !invalid.Stopped() {
			return invalid
		}
		return factory(interval)
	}
	lease, err := harness.scheduler.Start(t.Context(), RuntimeMetricsRetentionRequest{RootDirectory: root})
	if lease != nil || err == nil || !strings.Contains(err.Error(), "ticker is not configured") || !invalid.Stopped() {
		t.Fatalf("invalid ticker start = %v, %v, stopped=%v", lease, err, invalid.Stopped())
	}
	assertRetentionReportBefore(t, harness.next(t), RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9})
	assertRetentionPathAbsent(t, marker, "safe startup cleanup before ticker rejection")
	assertRetentionPreservedContent(t, artifact, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
	lease = harness.start(t, root)
	assertRetentionReportBefore(t, harness.next(t), RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9})
	assertHourlyTicker(t, harness.intervals)
	harness.ticker.Tick(time.Now())
	assertRetentionReportBefore(t, harness.next(t), RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9})
	if err := lease.Close(); err != nil || !harness.ticker.Stopped() {
		t.Fatalf("retried ticker lease close = %v, stopped=%v", err, harness.ticker.Stopped())
	}
	assertRetentionPreservedContent(t, artifact, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
}

func TestRuntimeMetricsRetentionSchedulerStartsAndClosesDeterministically(t *testing.T) {
	root := t.TempDir()
	coordination := &metricsTestCoordination{
		rootLock:    &metricsTestCloser{},
		tryRootLock: &metricsTestCloser{},
	}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	manualTicker := newManualRuntimeMetricsRetentionTicker()
	var intervals []time.Duration
	scheduler, err := NewRuntimeMetricsRetentionScheduler(
		retention,
		func(interval time.Duration) RuntimeMetricsRetentionTicker {
			intervals = append(intervals, interval)
			return manualTicker
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetentionScheduler(): %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scheduler.Start(canceled, RuntimeMetricsRetentionRequest{RootDirectory: root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start(canceled) = %v, want context.Canceled", err)
	}
	if _, err := scheduler.Start(context.Background(), RuntimeMetricsRetentionRequest{RootDirectory: " "}); err == nil || !strings.Contains(err.Error(), "root is required") {
		t.Fatalf("Start(blank root) = %v, want root validation", err)
	}

	lease, err := scheduler.Start(nil, RuntimeMetricsRetentionRequest{RootDirectory: root})
	if err != nil {
		t.Fatalf("Start(valid): %v", err)
	}
	if len(intervals) != 1 || intervals[0] != time.Hour {
		t.Fatalf("ticker intervals = %v, want one hourly interval", intervals)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("lease.Close(): %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("lease.Close() second call: %v", err)
	}
	if err := scheduler.Close(context.Background()); err != nil {
		t.Fatalf("scheduler.Close(): %v", err)
	}
	if err := scheduler.Close(context.Background()); err != nil {
		t.Fatalf("scheduler.Close() second call: %v", err)
	}
	if _, err := scheduler.Start(context.Background(), RuntimeMetricsRetentionRequest{RootDirectory: root}); err == nil || !strings.Contains(err.Error(), "scheduler is closed") {
		t.Fatalf("Start(after close) = %v, want closed validation", err)
	}
	assertSchedulerReleaseMissingWorker(t, scheduler, root)
}

func assertSchedulerReleaseMissingWorker(t *testing.T, scheduler *RuntimeMetricsRetentionScheduler, root string) {
	t.Helper()
	if err := scheduler.release(filepath.Clean(root)); err != nil {
		t.Fatalf("release(missing worker) = %v, want nil", err)
	}
}

func TestRuntimeMetricsRetentionSchedulerRejectsMissingTickers(t *testing.T) {
	tickerCases := []struct {
		name    string
		factory RuntimeMetricsRetentionTickerFactory
		want    string
	}{
		{name: "nil ticker", factory: func(time.Duration) RuntimeMetricsRetentionTicker { return nil }, want: "ticker is not configured"},
		{name: "nil channel", factory: func(time.Duration) RuntimeMetricsRetentionTicker { return nilChannelRetentionTicker{} }, want: "ticker is not configured"},
	}
	for _, test := range tickerCases {
		t.Run(test.name, func(t *testing.T) {
			caseCoordination := &metricsTestCoordination{
				rootLock:    &metricsTestCloser{},
				tryRootLock: &metricsTestCloser{},
			}
			caseRetention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, caseCoordination)
			if err != nil {
				t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
			}
			caseScheduler, err := NewRuntimeMetricsRetentionScheduler(caseRetention, test.factory, nil, nil)
			if err != nil {
				t.Fatalf("NewRuntimeMetricsRetentionScheduler(): %v", err)
			}
			if _, err := caseScheduler.Start(context.Background(), RuntimeMetricsRetentionRequest{RootDirectory: t.TempDir()}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Start(%s) = %v, want %q", test.name, err, test.want)
			}
			if err := caseScheduler.Close(context.Background()); err != nil {
				t.Fatalf("Close(%s) = %v, want nil", test.name, err)
			}
		})
	}
}

func TestRuntimeMetricsRetentionSchedulerRetriesRejectedRootAcquisition(t *testing.T) {
	t.Parallel()
	root, artifact, marker, unknown := installOrphanMarkerRecoveryFixture(t)
	cause := errors.New("root acquisition rejected")
	rootLock := &metricsTestCloser{}
	coordination := &metricsTestCoordination{
		rootLock: rootLock, lockRootErr: cause, tryRootLock: &metricsTestCloser{},
		tryClaimMarker: &metricsTestCloser{},
	}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time {
		return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	}, coordination)
	if err != nil {
		t.Fatal(err)
	}
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 2)
	t.Cleanup(func() {
		if err := harness.scheduler.Close(t.Context()); err != nil {
			t.Errorf("scheduler cleanup: %v", err)
		}
	})
	request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}}
	lease, err := harness.scheduler.Start(t.Context(), request)
	if lease != nil || !errors.Is(err, cause) || !strings.Contains(err.Error(), "prepare runtime metrics root") {
		t.Fatalf("rejected root acquisition = %v, %v, want preparation context and original cause", lease, err)
	}
	assertNoRetentionReport(t, harness.reports)
	if len(harness.intervals) != 0 || rootLock.closed != 0 {
		t.Fatalf("failed preparation created timers %v or closed an unacquired lock %d", harness.intervals, rootLock.closed)
	}
	assertRetentionPreservedContent(t, artifact, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
	assertRetentionPathExists(t, marker, "marker after rejected startup")
	coordination.lockRootErr = nil
	lease, err = harness.scheduler.Start(t.Context(), request)
	if err != nil || lease == nil {
		t.Fatalf("recovered startup = %v, %v", lease, err)
	}
	assertRecoveredRootAcquisition(t, harness, rootLock, artifact, marker, unknown)
	if err := lease.Close(); err != nil || !harness.ticker.Stopped() {
		t.Fatalf("recovered lease close = %v, ticker stopped = %v", err, harness.ticker.Stopped())
	}
}

func assertRecoveredRootAcquisition(
	t *testing.T, harness *runtimeMetricsRetentionSchedulerHarness,
	rootLock *metricsTestCloser, artifact, marker, unknown string,
) {
	t.Helper()
	observation := harness.next(t)
	if observation.err != nil || len(observation.report.Failures) != 0 || observation.report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) {
		t.Fatalf("recovered startup observation = %#v", observation)
	}
	assertHourlyTicker(t, harness.intervals)
	if rootLock.closed != 1 {
		t.Fatalf("recovered preparation released lock %d times, want 1", rootLock.closed)
	}
	assertRetentionPathAbsent(t, marker, "recovered orphan marker")
	assertRetentionPreservedContent(t, artifact, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
}

func TestRuntimeMetricsRetentionSchedulerReportsPreparationAndCanceledSweep(t *testing.T) {
	root := t.TempDir()
	workerCoordination := &metricsTestCoordination{
		rootLock:    &metricsTestCloser{err: errors.New("ensure root close failed")},
		tryRootLock: &metricsTestCloser{},
	}
	workerRetention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, workerCoordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(ensure root): %v", err)
	}
	workerScheduler, err := NewRuntimeMetricsRetentionScheduler(workerRetention, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetentionScheduler(ensure root): %v", err)
	}
	if _, err := workerScheduler.Start(context.Background(), RuntimeMetricsRetentionRequest{RootDirectory: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "prepare runtime metrics root") {
		t.Fatalf("Start(ensure root failure) = %v, want preparation context", err)
	}

	sweepRetention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, &metricsTestCoordination{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(sweep): %v", err)
	}
	sweepScheduler, err := NewRuntimeMetricsRetentionScheduler(sweepRetention, nil, func(RuntimeMetricsRetentionReport, error) {
		t.Fatal("canceled worker sweep published an observation")
	}, nil)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetentionScheduler(sweep): %v", err)
	}
	sweepContext, sweepCancel := context.WithCancel(context.Background())
	sweepCancel()
	sweepScheduler.sweep(sweepContext, RuntimeMetricsRetentionRequest{RootDirectory: root})
	if err := sweepScheduler.Close(context.Background()); err != nil {
		t.Fatalf("sweepScheduler.Close(): %v", err)
	}
}

func TestRuntimeMetricsRetentionSchedulerHandlesNilLifecycleValues(t *testing.T) {
	var nilLease *runtimeMetricsRetentionLease
	if err := nilLease.Close(); err != nil {
		t.Fatalf("nil lease Close() = %v, want nil", err)
	}
	if err := (&runtimeMetricsRetentionLease{}).Close(); err != nil {
		t.Fatalf("lease without scheduler Close() = %v, want nil", err)
	}
	var zeroTicker runtimeMetricsRetentionTicker
	if zeroTicker.C() != nil {
		t.Fatal("zero runtime ticker C() = non-nil, want nil")
	}
	zeroTicker.Stop()
}

func TestRuntimeMetricsRetentionSchedulerRejectsDotRootAndRecovers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 2)
	t.Cleanup(func() {
		if err := harness.scheduler.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	lease, err := harness.scheduler.Start(t.Context(), RuntimeMetricsRetentionRequest{RootDirectory: " . "})
	if lease != nil || err == nil || err.Error() != "start runtime metrics retention: root is required" {
		t.Fatalf("dot root = %v, %v", lease, err)
	}
	if len(harness.intervals) != 0 {
		t.Fatalf("rejected root started tickers: %v", harness.intervals)
	}
	assertNoRetentionReport(t, harness.reports)
	lease = harness.start(t, root)
	assertEmptyRetentionReport(t, harness.next(t))
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if !harness.ticker.Stopped() {
		t.Fatal("recovered root retained its ticker after lease close")
	}
}

func TestRuntimeMetricsRetentionSchedulerCloseStopsActiveLeaseAndRejectsRestart(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	harness := newRuntimeMetricsRetentionSchedulerHarness(t, retention, 2)
	t.Cleanup(func() {
		if err := harness.scheduler.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	lease := harness.start(t, root)
	assertEmptyRetentionReport(t, harness.next(t))
	if harness.ticker.Stopped() {
		t.Fatal("live lease started with a stopped ticker")
	}
	if err := harness.scheduler.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !harness.ticker.Stopped() {
		t.Fatal("process close left the active lease ticker running")
	}
	if next, err := harness.scheduler.Start(t.Context(), RuntimeMetricsRetentionRequest{RootDirectory: root}); next != nil || err == nil || err.Error() != "start runtime metrics retention: scheduler is closed" {
		t.Fatalf("restart after close = %v, %v", next, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("old lease close after process close: %v", err)
	}
	assertNoRetentionReport(t, harness.reports)
}

type nilChannelRetentionTicker struct{}

func (nilChannelRetentionTicker) C() <-chan time.Time { return nil }

func (nilChannelRetentionTicker) Stop() {}

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
		nil,
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

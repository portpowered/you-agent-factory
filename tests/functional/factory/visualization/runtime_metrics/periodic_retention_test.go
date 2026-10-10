package runtime_metrics_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// M5-S/F share one process and one logical scheduler. Startup is ordered to
// associate each registered hourly timer with its session. The success and
// missing-root phases advance both roots together on the shared timeline.
func TestPeriodicMetricsRetentionPreservesWorkAndReleasesClaims(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	facts := &selectedTimeNowOnlySource{clock: platformclock.NewDeterministic(base, 24*time.Hour)}
	scheduler := &periodicRetentionClock{
		Deterministic: platformclock.NewDeterministic(base, time.Hour),
		registered:    make(chan struct{}, 8),
	}
	core, logs := observer.New(zap.DebugLevel)
	reports := make(chan struct{}, 8)
	logger := zap.New(periodicRetentionLogObserver{Core: core, reports: reports})
	routes := &activeMetricsProviderRoutes{}
	process := support.BuildProcess(t, serviceedges.Edges{
		Clock: facts, ProcessScheduler: scheduler, ProcessLogger: logger, ProviderCommandRunner: routes,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return ctx.Value(retainedMetricsServerKey{}).(*support.ProcessAPIServer).Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	root := filepath.Join(t.TempDir(), "metrics")
	peerRoot := filepath.Join(t.TempDir(), "metrics")
	first := startActiveMetricsSession(t, process, routes, root)
	peer := startActiveMetricsSession(t, process, routes, peerRoot)
	for range 2 {
		awaitSelectedTimeSignal(t, scheduler.registered)
		awaitSelectedTimeSignal(t, reports)
	}
	live := functionalMetricArtifactPaths(t, root)[0]
	expired := writeFunctionalMetricsFixture(t, root, "2020/01/01", "000000.000000000-runtime-metrics-expired.log", "expired")
	unknown := filepath.Join(root, "keep.txt")
	writeFunctionalFile(t, unknown, "keep")
	scheduler.SetTick(1)
	for range 2 {
		awaitSelectedTimeSignal(t, reports)
	}
	assertPeriodicRetentionRemoval(t, logs, root)
	if _, err := os.Stat(expired); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("periodic expired artifact stat = %v, want removed", err)
	}
	assertFunctionalFileContents(t, unknown, "keep")
	assertFunctionalMetricPath(t, root, live)
	// These roots share the scheduler timeline, so the failure phase follows
	// the first tick rather than advancing a peer's deadlines independently.
	t.Run("missing root does not change successful Work", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows forbids renaming a metrics root with open writer/claim files; Linux functional CI owns M5-F")
		}
		assertPeriodicMissingRoot(t, scheduler, reports, logs, peerRoot)
	})
	finishPeriodicRetentionSessions(t, first, peer)
	if scheduler.active.Load() != 0 {
		t.Fatalf("retention timers survived session stop: %d", scheduler.active.Load())
	}
	// Reopening the selected root after its artifacts expire must reclaim the
	// stopped writer's file. This observes released claims via customer file effects.
	facts.clock.SetTick(2)
	reopened := startActiveMetricsSession(t, process, routes, root)
	if _, err := os.Stat(live); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stopped session artifact stat = %v, want startup retention to reclaim it", err)
	}
	finishPeriodicRetentionSessions(t, reopened)
}

func TestMetricsCoveragePeriodicSharedRootSurvivesOwnerCancellation(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := &periodicRetentionClock{
		Deterministic: platformclock.NewDeterministic(base, time.Hour),
		registered:    make(chan struct{}, 8),
	}
	core, logs := observer.New(zap.DebugLevel)
	reports := make(chan struct{}, 8)
	routes := &activeMetricsProviderRoutes{}
	process := support.BuildProcess(t, serviceedges.Edges{
		Clock:            &selectedTimeNowOnlySource{clock: platformclock.NewDeterministic(base, time.Hour)},
		ProcessScheduler: scheduler, ProcessLogger: zap.New(periodicRetentionLogObserver{Core: core, reports: reports}),
		ProviderCommandRunner: routes,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return ctx.Value(retainedMetricsServerKey{}).(*support.ProcessAPIServer).Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	root := filepath.Join(t.TempDir(), "metrics")
	first := startActiveMetricsSession(t, process, routes, root)
	awaitSelectedTimeSignal(t, reports)
	awaitSelectedTimeSignal(t, scheduler.registered)
	peer := startActiveMetricsSession(t, process, routes, root)
	live := functionalMetricArtifactPaths(t, root)
	control := selectedCancellationControl(t, first.url, first.id)
	if string(control.Outcome) != "ACCEPTED" {
		t.Fatalf("cancel outcome = %s", control.Outcome)
	}
	awaitSelectedTimeSignal(t, first.runner.cancelled)
	support.WaitForSessionStopped(t, first.url, first.id, 30*time.Second)
	assertSelectedCancellationState(t, first.url, first.id)
	first.command.Stop(t)
	expired := writeFunctionalMetricsFixture(t, root, "2020/01/01", "000000.000000000-runtime-metrics-expired.log", "expired")
	unsafe := writeFunctionalMetricsFixture(t, root, "2020/02/30", "000000.000000000-runtime-metrics-unsafe.log", "unsafe")
	blocked := writeFunctionalMetricsFixture(t, root, "2020/01/02", "000000.000000000-runtime-metrics-blocked.log", "blocked")
	restore := protectPeriodicMetricsCandidate(t, blocked)
	scheduler.SetTick(1)
	awaitSelectedTimeSignal(t, reports)
	awaitSelectedTimeSignal(t, scheduler.registered)
	assertPeriodicCandidateFailure(t, logs, root)
	if _, err := os.Stat(expired); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("safe periodic peer stat = %v, want removed", err)
	}
	assertFunctionalFileContents(t, unsafe, "{\"metric_name\":\"unsafe\",\"value\":1}\n")
	assertFunctionalFileContents(t, blocked, "{\"metric_name\":\"blocked\",\"value\":1}\n")
	for _, path := range live {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("periodic cleanup removed a current session artifact: %v", err)
		}
	}
	assertRetainedMetricsTokens(t, t.TempDir(), peer.url, peer.id, 0)
	repaired := filepath.Join(root, "2020", "01", "01", filepath.Base(unsafe))
	if err := os.MkdirAll(filepath.Dir(repaired), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(unsafe, repaired); err != nil {
		t.Fatal(err)
	}
	restore()
	scheduler.SetTick(2)
	awaitSelectedTimeSignal(t, reports)
	if _, err := os.Stat(repaired); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrected periodic candidate stat = %v, want removed", err)
	}
	if _, err := os.Stat(blocked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repaired permission candidate stat = %v, want removed", err)
	}
	close(peer.runner.release)
	support.WaitForSessionTerminalStatus(t, peer.url, peer.id, 30*time.Second)
	assertCompletedSessionMetrics(t, peer.url, peer.id)
	assertSelectedTimeWork(t, selectedTimeFixture{url: peer.url, session: peer.id})
	peer.command.Stop(t)
	reopened := startActiveMetricsSession(t, process, routes, root)
	close(reopened.runner.release)
	support.WaitForSessionTerminalStatus(t, reopened.url, reopened.id, 30*time.Second)
	assertCompletedSessionMetrics(t, reopened.url, reopened.id)
	reopened.command.Stop(t)
}

// Customers can make a historical candidate undeletable using ordinary
// filesystem ownership. Windows protects an open file from deletion; Unix
// deletion permission belongs to its parent directory. Restore before teardown.
func protectPeriodicMetricsCandidate(t *testing.T, path string) func() {
	t.Helper()
	selected, mode := filepath.Dir(path), os.FileMode(0o500)
	if runtime.GOOS == "windows" {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var once sync.Once
		restore := func() {
			once.Do(func() {
				if err := file.Close(); err != nil {
					t.Error(err)
				}
			})
		}
		t.Cleanup(restore)
		return restore
	}
	if err := os.Chmod(selected, mode); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if err := os.Chmod(selected, 0o700); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Error(err)
		}
	}
	t.Cleanup(restore)
	return restore
}

func assertPeriodicCandidateFailure(t *testing.T, logs *observer.ObservedLogs, root string) {
	t.Helper()
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "runtime metrics retention sweep completed with failures" &&
			fields["root"] == root && fields["failed_files"] == int64(1) && fields["removed_files"] == int64(1) {
			return
		}
	}
	t.Fatalf("candidate failure and safe peer cleanup diagnostic missing: %#v", logs.FilterMessageSnippet("runtime metrics retention sweep").All())
}

// The existing ProcessLogger edge observes completed retention diagnostics.
// Signal only after the observer has recorded the fields used by readback.
type periodicRetentionLogObserver struct {
	zapcore.Core
	reports chan<- struct{}
}

func (core periodicRetentionLogObserver) With(fields []zapcore.Field) zapcore.Core {
	return periodicRetentionLogObserver{Core: core.Core.With(fields), reports: core.reports}
}

func (core periodicRetentionLogObserver) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if core.Enabled(entry.Level) {
		return checked.AddCore(entry, core)
	}
	return checked
}

func (core periodicRetentionLogObserver) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if err := core.Core.Write(entry, fields); err != nil {
		return err
	}
	if strings.HasPrefix(entry.Message, "runtime metrics retention sweep") {
		core.reports <- struct{}{}
	}
	return nil
}

func finishPeriodicRetentionSessions(t *testing.T, sessions ...activeMetricsSession) {
	t.Helper()
	for _, session := range sessions {
		close(session.runner.release)
		support.WaitForSessionTerminalStatus(t, session.url, session.id, 30*time.Second)
		assertSelectedTimeWork(t, selectedTimeFixture{url: session.url, session: session.id})
		assertCompletedSessionMetrics(t, session.url, session.id)
		session.command.Stop(t)
	}
}

func mustRelativeMetricPath(t *testing.T, root, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return relative
}

func assertPeriodicRetentionRemoval(t *testing.T, logs *observer.ObservedLogs, root string) {
	t.Helper()
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "runtime metrics retention sweep completed" && fields["root"] == root && fields["removed_files"] == int64(1) {
			return
		}
	}
	t.Fatalf("periodic removal diagnostic missing; logs=%#v", logs.All())
}

func assertPeriodicMissingRoot(t *testing.T, scheduler *periodicRetentionClock, reports <-chan struct{}, logs *observer.ObservedLogs, root string) {
	t.Helper()
	expired := writeFunctionalMetricsFixture(t, root, "2020/01/01", "000000.000000000-runtime-metrics-expired.log", "outside expired")
	moved := root + "-moved"
	if err := os.Rename(root, moved); err != nil {
		t.Fatalf("move selected root before periodic sweep: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Rename(moved, root); err != nil {
			t.Error(err)
		}
	})
	// Both roots still own a live loop; observe both reports before readback.
	scheduler.SetTick(2)
	for range 2 {
		awaitSelectedTimeSignal(t, reports)
	}
	assertFunctionalFileContents(t, filepath.Join(moved, mustRelativeMetricPath(t, root, expired)), "{\"metric_name\":\"outside expired\",\"value\":1}\n")
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "runtime metrics retention sweep failed" && fields["root"] == root {
			failure, ok := fields["error"].(string)
			if ok && strings.Contains(failure, "sweep runtime metrics") {
				return
			}
		}
	}
	t.Fatalf("missing-root sweep failure diagnostic missing; logs=%#v", logs.All())
}

// Hourly retention alone follows logical time. Other process timers use the
// real scheduling edge so host readiness needs no arbitrary clock advances.
type periodicRetentionClock struct {
	*platformclock.Deterministic
	registered chan struct{}
	active     atomic.Int32
}

func (clock *periodicRetentionClock) NewTimer(duration time.Duration) platformclock.Timer {
	if duration != time.Hour {
		return platformclock.Real{}.NewTimer(duration)
	}
	clock.active.Add(1)
	timer := &periodicRetentionTimer{Timer: clock.Deterministic.NewTimer(duration), clock: clock}
	clock.registered <- struct{}{}
	return timer
}

func (clock *periodicRetentionClock) After(duration time.Duration) <-chan time.Time {
	return clock.NewTimer(duration).C()
}

type periodicRetentionTimer struct {
	platformclock.Timer
	clock   *periodicRetentionClock
	stopped atomic.Bool
}

func (timer *periodicRetentionTimer) Stop() bool {
	if timer.stopped.CompareAndSwap(false, true) {
		timer.clock.active.Add(-1)
	}
	return timer.Timer.Stop()
}

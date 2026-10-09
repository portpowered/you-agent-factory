package runtime_metrics_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

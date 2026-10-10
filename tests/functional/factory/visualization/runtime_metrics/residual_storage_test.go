package runtime_metrics_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMetricsResidualReadAccessRecovery(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"missing root", "file root", "artifact permission", "traversal permission"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			if runtime.GOOS == "windows" {
				t.Skip("native unprivileged Linux owns open-root rename and permission proof")
			}
			home, peerHome := t.TempDir(), t.TempDir()
			id, peerID := uuid.NewString(), uuid.NewString()
			root := platformmetrics.RuntimeMetricsRoot(home)
			server := startRetainedMetricsHost(t, home, id)
			peer := startRetainedMetricsHost(t, peerHome, peerID)
			path := filepath.Join(root, "2026", "10", "10", "120000.000000000-runtime-metrics-readable.log")
			writeResidualFacts(t, path, id, 3, false)
			denied := filepath.Join(root, "2026", "10", "10", "130000.000000000-runtime-metrics-readable-2026-10-10T13-01-00.000.log.gz")
			writeResidualFacts(t, denied, id, 4, true)
			writeResidualFacts(t, filepath.Join(platformmetrics.RuntimeMetricsRoot(peerHome), "120000.000000000-runtime-metrics-peer.log"), peerID, 9, false)
			assertMetricsCoverageTotals(t, home, server, id, 7, 7, 2)
			restore := faultResidualRead(t, root, denied, fault)
			query := retainedMetricsInputs(t, home, server, "--session", id)
			assertBoundaryCodedFailure(t, runtimeMetricsProcess(t).Execute(query.Input), query, "METRICS_QUERY_FAILED")
			if strings.Contains(query.Stderr(), "private-residual-content") {
				t.Fatal("artifact contents escaped query diagnostic")
			}
			assertMetricsCoverageTotals(t, peerHome, peer, peerID, 9, 9, 1)
			restore()
			assertMetricsCoverageTotals(t, home, server, id, 7, 7, 2)
			assertMetricsCoverageTotals(t, peerHome, peer, peerID, 9, 9, 1)
		})
	}
}

func writeResidualFacts(t *testing.T, path, id string, tokens int, compressed bool) {
	t.Helper()
	records := make([]map[string]any, 0, 3)
	for _, metric := range []string{runtimeProviderInputTokens, runtimeProviderOutputTokens, runtimeDispatchComplete} {
		value := tokens
		if metric == runtimeDispatchComplete {
			value = 1
		}
		records = append(records, map[string]any{"session_id": id, "metric_name": metric, "value": value, "private": "private-residual-content"})
	}
	writeRuntimeMetricsArtifact(t, path, compressed, records)
}

func TestMetricsResidualReadPlainAndCompressedKeepsPeerSeparate(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	id := uuid.NewString()
	root := platformmetrics.RuntimeMetricsRoot(home)
	server := startRetainedMetricsHost(t, home, id)
	writeResidualFacts(t, filepath.Join(root, "120000.000000000-runtime-metrics-plain.log"), id, 3, false)
	writeResidualFacts(t, filepath.Join(root, "130000.000000000-runtime-metrics-compressed-2026-10-10T13-01-00.000.log.gz"), id, 4, true)
	writeResidualFacts(t, filepath.Join(root, "140000.000000000-runtime-metrics-peer.log"), uuid.NewString(), 9, false)
	assertMetricsCoverageTotals(t, home, server, id, 7, 7, 2)
	assertMetricsCoverageTotals(t, home, server, "", 16, 16, 3)
	unknown := retainedMetricsInputs(t, home, server, "--session", uuid.NewString())
	assertBoundaryCodedFailure(t, runtimeMetricsProcess(t).Execute(unknown.Input), unknown, "METRICS_SESSION_NOT_FOUND")
}

func finishResidualMetricsSession(t *testing.T, session activeMetricsSession) {
	t.Helper()
	close(session.runner.release)
	support.WaitForSessionTerminalStatus(t, session.url, session.id, 30*time.Second)
	assertSelectedTimeWork(t, selectedTimeFixture{url: session.url, session: session.id})
	assertMetricsCoverageTotals(t, t.TempDir(), session.url, session.id, 1, 1, 1)
	session.command.Stop(t)
}

func faultResidualRead(t *testing.T, root, artifact, fault string) func() {
	t.Helper()
	if strings.Contains(fault, "permission") {
		path, mode := artifact, os.FileMode(0o600)
		if fault == "traversal permission" {
			path, mode = filepath.Dir(artifact), 0o755
		}
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		restore := func() {
			if err := os.Chmod(path, mode); err != nil {
				t.Error(err)
			}
		}
		t.Cleanup(restore)
		// A privileged host cannot prove a denied read; a skip is not evidence.
		if file, err := os.Open(artifact); err == nil {
			_ = file.Close()
			restore()
			t.Skip("host can bypass permissions; unprivileged Linux proof required")
		}
		return restore
	}
	backup := root + "-saved"
	if err := os.Rename(root, backup); err != nil {
		t.Fatal(err)
	}
	if fault == "file root" {
		writeFunctionalFile(t, root, "private-residual-content")
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		if fault == "file root" {
			if err := os.Remove(root); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Rename(backup, root); err != nil {
			t.Fatal(err)
		}
		restored = true
	}
	t.Cleanup(restore)
	return restore
}

func TestMetricsResidualRootStartupRecovery(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"coordination directory", "symlink root"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := filepath.Join(home, "selected")
			outside := filepath.Join(home, "outside")
			writeFunctionalFile(t, filepath.Join(outside, "sentinel"), "customer data")
			obstruction := filepath.Join(root, ".runtime-metrics-retention.lock")
			if fault == "symlink root" {
				if err := os.Symlink(outside, root); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("native Linux owns symlink proof: %v", err)
					}
					t.Fatal(err)
				}
				obstruction = root
			} else if err := os.MkdirAll(obstruction, 0o755); err != nil {
				t.Fatal(err)
			}
			process, routes := activeMetricsProcess(t)
			inputs, _ := runtimeMetricsRunInputs(t, home, root)
			runner := &activeMetricsProvider{started: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{})}
			close(runner.release)
			dir := filepath.Clean(inputs.Args[3])
			routes.runners.Store(dir, runner)
			t.Cleanup(func() { routes.runners.Delete(dir) })
			err := process.Execute(inputs.Input)
			if err == nil || !strings.Contains(err.Error(), "runtime metrics") || inputs.Stdout() != "" {
				t.Fatalf("startup = %v, stdout=%q", err, inputs.Stdout())
			}
			select {
			case <-runner.started:
				t.Fatal("unsafe storage dispatched provider")
			default:
			}
			assertFunctionalFileContents(t, filepath.Join(outside, "sentinel"), "customer data")
			if err := os.Remove(obstruction); err != nil {
				t.Fatal(err)
			}
			fresh := startActiveMetricsSession(t, process, routes, root)
			finishResidualMetricsSession(t, fresh)
			assertFunctionalFileContents(t, filepath.Join(outside, "sentinel"), "customer data")
		})
	}
}

func TestMetricsResidualDefaultAndSelectedDestinations(t *testing.T) {
	t.Parallel()
	home, customHome := t.TempDir(), t.TempDir()
	root := platformmetrics.RuntimeMetricsRoot(home)
	custom := filepath.Join(customHome, "selected")
	inputs, id := runtimeMetricsRunInputs(t, home, root)
	// Omit the destination flag so production resolves the profile default.
	inputs.Args = inputs.Args[:len(inputs.Args)-2]
	if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
		t.Fatal(err)
	}
	other, otherID := runtimeMetricsRunInputs(t, customHome, custom)
	if err := runtimeMetricsProcess(t).Execute(other.Input); err != nil {
		t.Fatal(err)
	}
	server := startRetainedMetricsHost(t, home, uuid.NewString())
	peer := startRetainedMetricsHost(t, customHome, uuid.NewString(), "--runtime-metrics-dir", custom)
	assertMetricsCoverageTotals(t, home, server, "", 1, 1, 1)
	assertMetricsCoverageTotals(t, customHome, peer, "", 1, 1, 1)
	for _, test := range []struct{ root, id string }{{root, id}, {custom, otherID}} {
		paths := functionalMetricArtifactPaths(t, test.root)
		found := false
		for _, path := range paths {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			found = found || strings.Contains(string(contents), test.id)
		}
		if !found {
			t.Fatalf("destination %q lost session %q", test.root, test.id)
		}
	}
}

func residualClaimPath(root, artifact string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(artifact)))
	return filepath.Join(root, ".runtime-metrics-retention-claims", hex.EncodeToString(digest[:])+".active")
}

func TestMetricsResidualRetentionAndClaimRecovery(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	facts := &selectedTimeNowOnlySource{clock: platformclock.NewDeterministic(base, 48*time.Hour)}
	scheduler := &periodicRetentionClock{Deterministic: platformclock.NewDeterministic(base, time.Hour), registered: make(chan struct{}, 8)}
	core, _ := observer.New(zap.DebugLevel)
	reports := make(chan struct{}, 8)
	routes := &activeMetricsProviderRoutes{}
	process := support.BuildProcess(t, serviceedges.Edges{
		Clock: facts, ProcessScheduler: scheduler, ProcessLogger: zap.New(periodicRetentionLogObserver{Core: core, reports: reports}), ProviderCommandRunner: routes,
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
	blocked := writeFunctionalMetricsFixture(t, root, "2020/01/01", "000000.000000000-runtime-metrics-blocked.log", "blocked")
	claim := residualClaimPath(root, blocked)
	if err := os.Mkdir(claim, 0o755); err != nil {
		t.Fatal(err)
	}
	safe := writeFunctionalMetricsFixture(t, root, "2020/01/02", "000000.000000000-runtime-metrics-safe.log", "safe")
	orphan := filepath.Join(root, ".runtime-metrics-retention-claims", strings.Repeat("a", 64)+".active")
	writeFunctionalFile(t, orphan, "private-residual-content")
	facts.clock.SetTick(1)
	scheduler.SetTick(1)
	awaitSelectedTimeSignal(t, reports)
	awaitSelectedTimeSignal(t, scheduler.registered)
	assertResidualRemoved(t, safe)
	assertFunctionalFileContents(t, blocked, "{\"metric_name\":\"blocked\",\"value\":1}\n")
	assertFunctionalFileContents(t, orphan, "private-residual-content")
	for _, path := range live {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("age-eligible active writer removed: %v", err)
		}
	}
	assertMetricsCoverageTotals(t, t.TempDir(), first.url, first.id, 0, 0, 0)
	assertMetricsCoverageTotals(t, t.TempDir(), peer.url, peer.id, 0, 0, 0)
	nextTick := assertResidualInventoryRecovery(t, root, peer, scheduler, reports)
	finishResidualMetricsSession(t, first)
	if err := os.Remove(claim); err != nil {
		t.Fatal(err)
	}
	writeFunctionalFile(t, orphan, "")
	scheduler.SetTick(nextTick)
	awaitSelectedTimeSignal(t, reports)
	awaitSelectedTimeSignal(t, scheduler.registered)
	assertResidualRemoved(t, blocked)
	assertResidualRemoved(t, orphan)
	remaining := functionalMetricArtifactPaths(t, root)
	if len(remaining) != 1 {
		t.Fatalf("released writer was not reclaimed: %v", remaining)
	}
	finishResidualMetricsSession(t, peer)
	fresh := startActiveMetricsSession(t, process, routes, root)
	finishResidualMetricsSession(t, fresh)
}

func assertResidualRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("eligible path %q stat = %v, want reclaimed", path, err)
	}
}

func assertResidualInventoryRecovery(t *testing.T, root string, peer activeMetricsSession, scheduler *periodicRetentionClock, reports <-chan struct{}) int {
	t.Helper()
	nextTick := 2
	t.Run("incomplete inventory preserves orphan storage until repair", func(t *testing.T) {
		// Serialized with the same-root sweep: traversal denial and repair are
		// successive customer storage states, while unrelated cases run parallel.
		if runtime.GOOS == "windows" {
			t.Skip("unprivileged native Linux owns traversal permission proof")
		}
		hidden := writeFunctionalMetricsFixture(t, root, "2020/02/01", "000000.000000000-runtime-metrics-hidden.log", "hidden")
		directory := filepath.Dir(hidden)
		if err := os.Chmod(directory, 0); err != nil {
			t.Fatal(err)
		}
		restore := func() {
			if err := os.Chmod(directory, 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Error(err)
			}
		}
		t.Cleanup(restore)
		if _, err := os.ReadDir(directory); err == nil {
			restore()
			t.Skip("privileged host bypasses traversal permission")
		}
		marker := filepath.Join(root, ".runtime-metrics-retention-claims", strings.Repeat("b", 64)+".active")
		writeFunctionalFile(t, marker, "")
		discovered := writeFunctionalMetricsFixture(t, root, "2020/03/01", "000000.000000000-runtime-metrics-discovered.log", "discovered")
		scheduler.SetTick(nextTick)
		nextTick++
		awaitSelectedTimeSignal(t, reports)
		awaitSelectedTimeSignal(t, scheduler.registered)
		assertResidualRemoved(t, discovered)
		assertFunctionalFileContents(t, marker, "")
		restore()
		scheduler.SetTick(nextTick)
		nextTick++
		awaitSelectedTimeSignal(t, reports)
		awaitSelectedTimeSignal(t, scheduler.registered)
		assertResidualRemoved(t, marker)
		assertResidualRemoved(t, hidden)
		assertMetricsCoverageTotals(t, t.TempDir(), peer.url, peer.id, 0, 0, 0)
	})
	return nextTick
}

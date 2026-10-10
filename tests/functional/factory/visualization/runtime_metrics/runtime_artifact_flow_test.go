package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// M4-S/F: the same process and metrics root serve overlapping sessions. Each
// invocation owns its listener, profile and provider gate. Lifecycle controls
// use their API-owned contract; metrics reads use the customer CLI.
func TestActiveMetricsSessionsPreservePeerOnCompletionOrCancellation(t *testing.T) {
	t.Parallel()
	process, routes := activeMetricsProcess(t)
	for _, cancelFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel first=%t", cancelFirst), func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), "metrics")
			first := startActiveMetricsSession(t, process, routes, root)
			firstPaths := functionalMetricArtifactPaths(t, root)
			if len(firstPaths) != 1 {
				t.Fatalf("first active artifacts = %v, want one", firstPaths)
			}
			peer := startActiveMetricsSession(t, process, routes, root)
			assertRetainedMetricsTokens(t, t.TempDir(), first.url, first.id, 0)
			assertRetainedMetricsTokens(t, t.TempDir(), peer.url, peer.id, 0)
			if _, err := os.Stat(firstPaths[0]); err != nil {
				t.Fatalf("peer startup removed active artifact: %v", err)
			}
			if cancelFirst {
				control := selectedCancellationControl(t, first.url, first.id)
				if string(control.Outcome) != "ACCEPTED" {
					t.Fatalf("cancel outcome = %s", control.Outcome)
				}
				awaitSelectedTimeSignal(t, first.runner.cancelled)
				support.WaitForSessionStopped(t, first.url, first.id, 30*time.Second)
				assertSelectedCancellationState(t, first.url, first.id)
			} else {
				close(first.runner.release)
				support.WaitForSessionTerminalStatus(t, first.url, first.id, 30*time.Second)
				assertCompletedSessionMetrics(t, first.url, first.id)
			}
			// The peer is still gated: the first session's terminal provider facts
			// must not appear in the peer's selected report.
			assertRetainedMetricsTokens(t, t.TempDir(), peer.url, peer.id, 0)
			close(peer.runner.release)
			support.WaitForSessionTerminalStatus(t, peer.url, peer.id, 30*time.Second)
			assertCompletedSessionMetrics(t, peer.url, peer.id)
			assertSelectedTimeWork(t, selectedTimeFixture{url: peer.url, session: peer.id})
			first.command.Stop(t)
			peer.command.Stop(t)
			for _, path := range functionalMetricArtifactPaths(t, root) {
				if path != firstPaths[0] || !cancelFirst {
					assertFunctionalRuntimeMetricsRecords(t, path)
				}
			}
		})
	}
}

type activeMetricsProviderRoutes struct{ runners sync.Map }

func TestMetricsCoverageAgeRetentionKeepsFreshFactsAndUnsafeContent(t *testing.T) {
	t.Parallel()
	process, routes := activeMetricsProcess(t)
	root := filepath.Join(t.TempDir(), "metrics")
	expired := writeFunctionalMetricsFixture(t, root, "2020/01/01", "000000.000000000-runtime-metrics-expired.log", "expired")
	unsafe := writeFunctionalMetricsFixture(t, root, "2020/02/30", "000000.000000000-runtime-metrics-unsafe.log", "unsafe")
	fresh := filepath.Join(root, filepath.FromSlash(time.Now().UTC().Format("2006/01/02")), "235959.000000000-runtime-metrics-fresh.log")
	historyID := uuid.NewString()
	writeRuntimeMetricsArtifact(t, fresh, false, []map[string]any{
		{"metric_name": runtimeProviderInputTokens, "value": 7, "session_id": historyID, "dispatch_id": "retained", "provider": "codex"},
		{"metric_name": runtimeProviderOutputTokens, "value": 3, "session_id": historyID, "dispatch_id": "retained", "provider": "codex"},
		{"metric_name": "dispatch.completed", "value": 1, "session_id": historyID},
	})
	first := startActiveMetricsSession(t, process, routes, root)
	if _, err := os.Stat(expired); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expired startup artifact stat = %v, want removed", err)
	}
	assertFunctionalFileContents(t, unsafe, "{\"metric_name\":\"unsafe\",\"value\":1}\n")
	assertMetricsCoverageTotals(t, t.TempDir(), first.url, "", 7, 3, 1)
	assertMetricsCoverageTotals(t, t.TempDir(), first.url, first.id, 0, 0, 0)
	close(first.runner.release)
	support.WaitForSessionTerminalStatus(t, first.url, first.id, 30*time.Second)
	assertSelectedTimeWork(t, selectedTimeFixture{url: first.url, session: first.id})
	assertMetricsCoverageTotals(t, t.TempDir(), first.url, first.id, 1, 1, 1)
	assertMetricsCoverageTotals(t, t.TempDir(), first.url, "", 8, 4, 2)
	first.command.Stop(t)
}

// A storage failure occurs before provider dispatch. Repair and peer commands
// enter the same reusable process, with separate session and profile ownership.
func TestMetricsCoverageStorageFailureRepairAndPeer(t *testing.T) {
	t.Parallel()
	process, routes := activeMetricsProcess(t)
	for _, obstruction := range []string{"parent", "claim directory"} {
		t.Run(obstruction, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := filepath.Join(home, "metrics")
			blocked := home + string(os.PathSeparator) + "blocked"
			if obstruction == "parent" {
				root = filepath.Join(blocked, "metrics")
			} else {
				blocked = filepath.Join(root, ".runtime-metrics-retention-claims")
			}
			writeFunctionalFile(t, blocked, "customer obstruction")
			inputs, _ := runtimeMetricsRunInputs(t, home, root)
			runner := &activeMetricsProvider{started: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{})}
			dir := filepath.Clean(inputs.Args[3])
			routes.runners.Store(dir, runner)
			t.Cleanup(func() { routes.runners.Delete(dir) })
			err := process.Execute(inputs.Input)
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) || !strings.Contains(err.Error(), "runtime metrics") {
				t.Fatalf("storage startup error = %v, want metrics filesystem failure", err)
			}
			if inputs.Stdout() != "" {
				t.Fatalf("failed startup stdout = %q", inputs.Stdout())
			}
			select {
			case <-runner.started:
				t.Fatal("blocked metrics storage dispatched a provider")
			default:
			}
			assertFunctionalFileContents(t, blocked, "customer obstruction")
			peer := startActiveMetricsSession(t, process, routes, filepath.Join(t.TempDir(), "metrics"))
			finishPeriodicRetentionSessions(t, peer)
			if err := os.Remove(blocked); err != nil {
				t.Fatal(err)
			}
			first := startActiveMetricsSession(t, process, routes, root)
			close(first.runner.release)
			support.WaitForSessionTerminalStatus(t, first.url, first.id, 30*time.Second)
			assertCompletedSessionMetrics(t, first.url, first.id)
			assertMetricsCoverageTotals(t, home, first.url, first.id, 1, 1, 1)
			first.command.Stop(t)
			paths := functionalMetricArtifactPaths(t, root)
			var firstPath string
			for _, path := range paths {
				if strings.Contains(filepath.Base(path), first.id) {
					firstPath = path
				}
			}
			retained, err := os.ReadFile(firstPath)
			if err != nil {
				t.Fatal(err)
			}
			second := startActiveMetricsSession(t, process, routes, root)
			// Session selection is host-scoped: a stopped host's ID is not a
			// live ID on its successor. Its customer artifact remains intact.
			assertRetainedMetricsTokens(t, home, second.url, second.id, 0)
			assertFunctionalFileContents(t, firstPath, string(retained))
			close(second.runner.release)
			support.WaitForSessionTerminalStatus(t, second.url, second.id, 30*time.Second)
			assertCompletedSessionMetrics(t, second.url, second.id)
			assertRetainedMetricsTokens(t, home, second.url, second.id, 1)
			assertMetricsCoverageTotals(t, home, second.url, second.id, 1, 1, 1)
			assertMetricsCoverageTotals(t, home, second.url, "", 2, 2, 2)
			assertFunctionalFileContents(t, firstPath, string(retained))
			second.command.Stop(t)
		})
	}
}

func (routes *activeMetricsProviderRoutes) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner, ok := routes.runners.Load(filepath.Clean(request.WorkDir))
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("no active metrics provider route for %s", request.WorkDir)
	}
	return runner.(*activeMetricsProvider).Run(ctx, request)
}

type activeMetricsProvider struct {
	started, release, cancelled chan struct{}
	once                        sync.Once
}

func (runner *activeMetricsProvider) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.once.Do(func() { close(runner.started) })
	result, err := support.NewGatedSuccessCommandRunner("active metrics COMPLETE", runner.release).Run(ctx, request)
	if errors.Is(err, context.Canceled) {
		close(runner.cancelled)
	}
	return result, err
}

type activeMetricsSession struct {
	id, url string
	command *support.ProcessCommand
	runner  *activeMetricsProvider
}

func startActiveMetricsSession(t *testing.T, process support.ApplicationProcess, routes *activeMetricsProviderRoutes, root string) activeMetricsSession {
	t.Helper()
	home := t.TempDir()
	inputs, id := runtimeMetricsRunInputs(t, home, root,
		"--continuously", "--with-server", "--server", "http://127.0.0.1:1",
		"--runtime-metrics-max-size-mb", "1", "--runtime-metrics-max-age-days", "1")
	dir := inputs.Args[3]
	inputs.WorkingDirectory = dir
	inputs.Env = []string{"HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + filepath.Join(home, "appdata"), "LOCALAPPDATA=" + filepath.Join(home, "localappdata"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	runner := &activeMetricsProvider{started: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{})}
	routes.runners.Store(filepath.Clean(dir), runner)
	t.Cleanup(func() { routes.runners.Delete(filepath.Clean(dir)) })
	server := support.NewProcessAPIServer()
	inputs.Context = context.WithValue(t.Context(), retainedMetricsServerKey{}, server)
	command := support.StartProcessCommand(t, process, inputs.Input)
	url := server.WaitForURL(t)
	awaitSelectedTimeSignal(t, runner.started)
	return activeMetricsSession{id: id, url: url, command: command, runner: runner}
}

const functionalRuntimeArtifactTimeLayout = "150405.000000000"

// TestRuntimeMetricsAndArtifactsThroughRootProcess proves that an ordinary
// provider-backed customer flow creates readable metrics, applies configured
// startup retention, and leaves unrelated content protected.
func TestRuntimeMetricsAndArtifactsThroughRootProcess(t *testing.T) {
	t.Parallel()
	factoryDir := support.ScaffoldSingleStepFactory(t, "runtime-artifact-functional")
	testutil.WriteSeedFile(t, factoryDir, "task", []byte(`{"title":"runtime artifact flow"}`))
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	metricsRoot := filepath.Join(t.TempDir(), "metrics")
	expiredPath := writeFunctionalMetricsFixture(t, metricsRoot, "2020/01/01", "000000.000000000-runtime-metrics-expired.log", "expired")
	outsidePath := filepath.Join(filepath.Dir(metricsRoot), "outside-metrics.txt")
	writeFunctionalFile(t, outsidePath, "outside")
	writeFunctionalFile(t, filepath.Join(metricsRoot, "keep.txt"), "unrelated")
	symlinkPath := createFunctionalMetricsSymlink(t, metricsRoot, outsidePath)
	sessionID := uuid.NewString()
	var viewsMu sync.Mutex
	var views []factoryvisualization.View
	var visualization factoryvisualization.Service
	activationErrors := make(chan error, 1)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Args: []string{
			"--session", sessionID,
			"--runtime-metrics-dir", metricsRoot,
			"--runtime-metrics-max-size-mb", "1",
			"--runtime-metrics-max-age-days", "1",
		},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner("runtime artifact COMPLETE"),
			FactoryVisualizationSink: factoryvisualization.SinkFunc(func(view factoryvisualization.View) {
				viewsMu.Lock()
				defer viewsMu.Unlock()
				views = append(views, view)
			}),
			FactoryVisualizationRootObserver: func(root factoryvisualization.Root) {
				visualization = root
				_, err := root.Activate(t.Context(), factoryvisualization.ActivateRequest{Mode: factoryvisualization.ActivateModeRetainedThenLive})
				activationErrors <- err
			},
		},
	})

	select {
	case err := <-activationErrors:
		if err != nil {
			t.Fatalf("activate selected visualization: %v", err)
		}
	case <-time.After(support.ScaledTimeout(15 * time.Second)):
		t.Fatal("selected visualization was not opened")
	}
	support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, 30*time.Second)
	assertCompletedSessionMetrics(t, server.URL(), sessionID)
	livePaths := functionalMetricArtifactPaths(t, metricsRoot)
	if len(livePaths) != 1 {
		t.Fatalf("live metrics artifacts = %#v, want exactly one regular active artifact", livePaths)
	}
	assertFunctionalMetricPath(t, metricsRoot, livePaths[0])

	assertVisualizationDrained(t, visualization, &viewsMu, &views)
	server.Stop(t)
	assertVisualizationDrained(t, visualization, &viewsMu, &views)
	if _, err := os.Stat(expiredPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expired artifact stat error = %v, want startup retention to remove %q", err, expiredPath)
	}
	assertFunctionalFileContents(t, filepath.Join(metricsRoot, "keep.txt"), "unrelated")
	assertFunctionalFileContents(t, outsidePath, "outside")
	if symlinkPath != "" {
		if _, err := os.Lstat(symlinkPath); err != nil {
			t.Fatalf("protected metrics symlink %q: %v", symlinkPath, err)
		}
	}
	assertFunctionalRuntimeMetricsRecords(t, livePaths[0])
}

func assertVisualizationDrained(t *testing.T, visualization factoryvisualization.Service, mu *sync.Mutex, views *[]factoryvisualization.View) {
	t.Helper()
	ctx := t.Context()
	if drained, err := visualization.StopDrain(ctx, factoryvisualization.StopDrainRequest{}); err != nil || drained.State != factoryvisualization.LifecycleStateStopped {
		t.Fatalf("drain = %+v, %v", drained, err)
	}
	joined, err := visualization.Join(ctx, factoryvisualization.JoinRequest{})
	if err != nil || joined.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("visualization Join = %+v, %v, want drained lifecycle", joined, err)
	}
	mu.Lock()
	count := len(*views)
	var projected bool
	for _, view := range *views {
		projected = projected || view.Runtime.TickCount > 0
	}
	mu.Unlock()
	if count == 0 || !projected {
		t.Fatalf("selected visualization emitted %d views without runtime progress", count)
	}
	// Repeated drain after host completion must remain idempotent and emit no view.
	if drained, err := visualization.StopDrain(ctx, factoryvisualization.StopDrainRequest{}); err != nil || drained.State != factoryvisualization.LifecycleStateStopped {
		t.Fatalf("repeat drain = %+v, %v", drained, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*views) != count {
		t.Fatalf("visualization emitted after completed drain: %d -> %d", count, len(*views))
	}
}

func assertFunctionalRuntimeMetricsRecords(t *testing.T, path string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime metrics artifact %q: %v", path, err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode runtime metrics record %q: %v", line, err)
		}
		name, _ := record["metric_name"].(string)
		if name != "provider.input_tokens" && name != "provider.output_tokens" {
			continue
		}
		value, ok := record["value"].(float64)
		if !ok || value <= 0 {
			t.Fatalf("runtime metric %q value = %#v, want positive JSON number: %#v", name, record["value"], record)
		}
		for _, field := range []string{"session_id", "runtime_instance_id", "dispatch_id", "worker_session_id", "ts"} {
			if value, _ := record[field].(string); strings.TrimSpace(value) == "" {
				t.Fatalf("runtime metric %q field %q is empty: %#v", name, field, record)
			}
		}
		seen[name] = true
	}
	if !seen["provider.input_tokens"] || !seen["provider.output_tokens"] {
		t.Fatalf("runtime metrics artifact %q lacks provider input/output samples", path)
	}
}

func writeFunctionalMetricsFixture(t *testing.T, root, datedDirectory, name, metricName string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(datedDirectory), name)
	writeFunctionalFile(t, path, fmt.Sprintf("{\"metric_name\":%q,\"value\":1}\n", metricName))
	return path
}

func writeFunctionalFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func createFunctionalMetricsSymlink(t *testing.T, root, target string) string {
	t.Helper()
	path := filepath.Join(root, "2020", "01", "01", "010000.000000000-runtime-metrics-link.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatalf("Symlink(%q, %q): %v", target, path, err)
		}
		t.Logf("symlink preservation unavailable on Windows: %v", err)
		return ""
	}
	return path
}

func functionalMetricArtifactPaths(t *testing.T, root string) []string {
	t.Helper()
	paths := make([]string, 0, 1)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry != nil && entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".log") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%q): %v", root, err)
	}
	sort.Strings(paths)
	return paths
}

func assertFunctionalMetricPath(t *testing.T, root, path string) {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("Rel(%q, %q): %v", root, path, err)
	}
	parts := strings.Split(relative, string(os.PathSeparator))
	if len(parts) != 4 || !strings.Contains(parts[3], "-runtime-metrics-") {
		t.Fatalf("metrics artifact path = %q, want YYYY/MM/DD/<time>-runtime-metrics-*.log", path)
	}
	if _, err := time.Parse("2006/01/02", strings.Join(parts[:3], "/")); err != nil {
		t.Fatalf("metrics artifact date path = %q: %v", relative, err)
	}
	if _, err := time.Parse(functionalRuntimeArtifactTimeLayout, parts[3][:len(functionalRuntimeArtifactTimeLayout)]); err != nil {
		t.Fatalf("metrics artifact time in %q: %v", relative, err)
	}
}

func assertFunctionalFileContents(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(contents) != want {
		t.Fatalf("%q contents = %q, want %q", path, contents, want)
	}
}

// M1-F: the customer's root path is a file. Startup must retain the filesystem
// cause and leave that file and its peer unchanged rather than run without metrics.
func TestRuntimeMetricsStartupRejectsFileRootThroughSharedProcess(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := filepath.Join(home, "metrics-file")
	peer := filepath.Join(home, "peer.txt")
	writeFunctionalFile(t, root, "root contents")
	writeFunctionalFile(t, peer, "peer contents")
	inputs, _ := runtimeMetricsRunInputs(t, home, root)
	err := runtimeMetricsProcess(t).Execute(inputs.Input)
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || !strings.HasPrefix(filepath.Clean(pathErr.Path), root) ||
		!strings.Contains(err.Error(), "runtime metrics") {
		t.Fatalf("startup error = %v, want metrics filesystem path failure under %q", err, root)
	}
	if strings.Contains(inputs.Stdout(), "runtime metrics COMPLETE") {
		t.Fatalf("failed metrics startup published successful Work: %s", inputs.Stdout())
	}
	assertFunctionalFileContents(t, root, "root contents")
	assertFunctionalFileContents(t, peer, "peer contents")
}

// M2-S/F: size retention prunes the oldest eligible file and protects a
// recognized name that is already a directory when startup inventories it.
func TestMetricsCoverageStartupSizeRetentionThroughSharedProcess(t *testing.T) {
	t.Parallel()
	for _, protectDirectory := range []bool{false, true} {
		t.Run(fmt.Sprintf("protected directory=%t", protectDirectory), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := platformmetrics.RuntimeMetricsRoot(home)
			oldest := filepath.Join(root, "2026", "08", "20", "000000.000000000-runtime-metrics-oldest.log")
			newer := filepath.Join(root, "2026", "08", "20", "010000.000000000-runtime-metrics-newer.log")
			newerContents := strings.Repeat(" ", 600*1024) + "{\"metric_name\":\"newer\",\"value\":1}\n"
			writeFunctionalFile(t, newer, newerContents)
			unknown := filepath.Join(root, "2026", "08", "20", "keep.txt")
			writeFunctionalFile(t, unknown, "unknown file")
			if protectDirectory {
				writeFunctionalFile(t, filepath.Join(oldest, "child.txt"), "protected contents")
				oldest = filepath.Join(root, "2026", "08", "20", "003000.000000000-runtime-metrics-eligible.log")
			}
			writeFunctionalFile(t, oldest, strings.Repeat(" ", 600*1024)+"{\"metric_name\":\"oldest\",\"value\":1}\n")
			inputs, _ := runtimeMetricsRunInputs(t, home, root,
				"--runtime-metrics-max-size-mb", "1", "--runtime-metrics-max-age-days", "0")
			if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
				t.Fatalf("size retention Work: %v; stderr=%s", err, inputs.Stderr())
			}
			if _, err := os.Stat(oldest); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("oldest eligible artifact stat = %v, want pruned", err)
			}
			assertFunctionalFileContents(t, unknown, "unknown file")
			assertFunctionalFileContents(t, newer, newerContents)
			paths := functionalMetricArtifactPaths(t, root)
			if len(paths) != 2 {
				t.Fatalf("retained artifacts = %v, want newer and completed active artifact", paths)
			}
			for _, path := range paths {
				if path != newer {
					assertFunctionalRuntimeMetricsRecords(t, path)
				}
			}
			if protectDirectory {
				assertFunctionalFileContents(t, filepath.Join(root, "2026", "08", "20",
					"000000.000000000-runtime-metrics-oldest.log", "child.txt"), "protected contents")
			}
			server := startRetainedMetricsHost(t, home, uuid.NewString(), "--runtime-metrics-max-age-days", "0")
			assertMetricsCoverageTotals(t, home, server, "", 1, 1, 1)
		})
	}
}

// Retention cannot order an artifact whose calendar path or clock is invalid.
// Even above the size budget it must preserve those bytes, prune a valid peer,
// and remain able to prune the artifact after the customer repairs its path.
func TestMetricsCoverageStartupProtectsUnorderableHistoryAndRecovers(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name, date, clock string
	}{
		{name: "invalid calendar date", date: "2020/02/30", clock: "010000.000000000"},
		{name: "invalid clock", date: "2020/01/01", clock: "250000.000000000"},
		{name: "extra directory level", date: "2020/01/01/archive", clock: "010000.000000000"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			assertUnorderableHistoryRecovery(t, scenario.date, scenario.clock)
		})
	}
}

func assertUnorderableHistoryRecovery(t *testing.T, date, clock string) {
	t.Helper()
	home := t.TempDir()
	root := platformmetrics.RuntimeMetricsRoot(home)
	name := clock + "-runtime-metrics-unorderable.log"
	selected := filepath.Join(root, filepath.FromSlash(date), name)
	contents := strings.Repeat("x", 1024*1024+1)
	writeFunctionalFile(t, selected, contents)
	peer := writeFunctionalMetricsFixture(t, root, "2020/01/02", "010000.000000000-runtime-metrics-peer.log", "peer")
	unknown := filepath.Join(filepath.Dir(selected), "customer-note.txt")
	writeFunctionalFile(t, unknown, "preserve customer content")
	runHistoryRetentionWork(t, home, root)
	assertFunctionalFileContents(t, selected, contents)
	assertFunctionalFileContents(t, unknown, "preserve customer content")
	if _, err := os.Stat(peer); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("valid expired peer stat = %v, want pruned", err)
	}
	paths := functionalMetricArtifactPaths(t, root)
	if len(paths) != 2 {
		t.Fatalf("protected history artifacts = %v, want selected and completed Work", paths)
	}
	var completedPath, completedContents string
	for _, path := range paths {
		if path != selected {
			assertFunctionalRuntimeMetricsRecords(t, path)
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			completedPath, completedContents = path, string(contents)
		}
	}
	repaired := filepath.Join(root, "2020", "01", "03", "010000.000000000-runtime-metrics-repaired.log")
	if err := os.MkdirAll(filepath.Dir(repaired), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(selected, repaired); err != nil {
		t.Fatal(err)
	}
	runHistoryRetentionWork(t, home, root)
	if _, err := os.Stat(repaired); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("repaired expired history stat = %v, want pruned", err)
	}
	assertFunctionalFileContents(t, unknown, "preserve customer content")
	assertFunctionalFileContents(t, completedPath, completedContents)
	paths = functionalMetricArtifactPaths(t, root)
	if len(paths) != 2 {
		t.Fatalf("recovered history artifacts = %v, want both completed Works", paths)
	}
	for _, path := range paths {
		assertFunctionalRuntimeMetricsRecords(t, path)
	}
}

func runHistoryRetentionWork(t *testing.T, home, root string) {
	t.Helper()
	inputs, _ := runtimeMetricsRunInputs(t, home, root,
		"--runtime-metrics-max-size-mb", "1", "--runtime-metrics-max-age-days", "1")
	inputs.Input.Env = []string{"HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + filepath.Join(home, "appdata"), "LOCALAPPDATA=" + filepath.Join(home, "localappdata"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
		t.Fatalf("history retention Work: %v; stderr=%s", err, inputs.Stderr())
	}
}

func runtimeMetricsRunInputs(t *testing.T, home, root string, flags ...string) (*support.CapturedInputs, string) {
	t.Helper()
	factory := support.ScaffoldSingleStepFactory(t, "retention-work")
	testutil.WriteSeedFile(t, factory, "task", []byte(`{"title":"retention Work"}`))
	support.WriteAgentConfig(t, factory, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	session := uuid.NewString()
	args := append([]string{"you", "run", "--dir", factory, "--session", session,
		"--quiet", "--no-record", "--runtime-metrics-dir", root}, flags...)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = []string{"HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + filepath.Join(home, "appdata"), "LOCALAPPDATA=" + filepath.Join(home, "localappdata"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	inputs.Input.WorkingDirectory = home
	return inputs, session
}

// M1-S uses the owning host while its completed session still has a retained
// metrics scope. A different host cannot resolve that session under the public
// contract, even when it points at the same artifact directory.
func assertCompletedSessionMetrics(t *testing.T, server, session string) {
	t.Helper()
	query := retainedMetricsInputs(t, t.TempDir(), server, "--session", session)
	if err := runtimeMetricsProcess(t).Execute(query.Input); err != nil {
		t.Fatalf("completed session metrics: %v", err)
	}
	var report struct {
		Scope struct {
			Session string `json:"factory_session_id"`
		} `json:"scope"`
		Totals struct {
			Input  float64 `json:"input_tokens"`
			Output float64 `json:"output_tokens"`
		} `json:"totals"`
	}
	if err := json.Unmarshal([]byte(query.Stdout()), &report); err != nil {
		t.Fatal(err)
	}
	if report.Scope.Session != session || report.Totals.Input <= 0 || report.Totals.Output <= 0 || query.Stderr() != "" {
		t.Fatalf("selected session report = %+v, stderr=%q; want %q and positive provider tokens", report, query.Stderr(), session)
	}
}

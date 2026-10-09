package runtime_metrics_test

import (
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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
func TestRuntimeMetricsStartupSizeRetentionThroughSharedProcess(t *testing.T) {
	t.Parallel()
	for _, protectDirectory := range []bool{false, true} {
		t.Run(fmt.Sprintf("protected directory=%t", protectDirectory), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := platformmetrics.RuntimeMetricsRoot(home)
			oldest := filepath.Join(root, "2026", "08", "20", "000000.000000000-runtime-metrics-oldest.log")
			newer := writeFunctionalMetricsFixture(t, root, "2026/08/20", "010000.000000000-runtime-metrics-newer.log", "newer")
			unknown := filepath.Join(root, "2026", "08", "20", "keep.txt")
			writeFunctionalFile(t, unknown, "unknown file")
			if protectDirectory {
				writeFunctionalFile(t, filepath.Join(oldest, "child.txt"), "protected contents")
				oldest = filepath.Join(root, "2026", "08", "20", "003000.000000000-runtime-metrics-eligible.log")
			}
			writeFunctionalFile(t, oldest, strings.Repeat("x", 1024*1024+1))
			inputs, _ := runtimeMetricsRunInputs(t, home, root,
				"--runtime-metrics-max-size-mb", "1", "--runtime-metrics-max-age-days", "0")
			if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
				t.Fatalf("size retention Work: %v; stderr=%s", err, inputs.Stderr())
			}
			if _, err := os.Stat(oldest); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("oldest eligible artifact stat = %v, want pruned", err)
			}
			assertFunctionalFileContents(t, unknown, "unknown file")
			assertFunctionalFileContents(t, newer, "{\"metric_name\":\"newer\",\"value\":1}\n")
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
		})
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
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
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

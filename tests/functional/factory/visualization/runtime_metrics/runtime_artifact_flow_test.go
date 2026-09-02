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
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Args: []string{
			"--runtime-metrics-dir", metricsRoot,
			"--runtime-metrics-max-size-mb", "1",
			"--runtime-metrics-max-age-days", "1",
		},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner("runtime artifact COMPLETE"),
		},
	})

	support.WaitForTerminalStatus(t, server.URL(), 30*time.Second)
	livePaths := functionalMetricArtifactPaths(t, metricsRoot)
	if len(livePaths) != 1 {
		t.Fatalf("live metrics artifacts = %#v, want exactly one regular active artifact", livePaths)
	}
	assertFunctionalMetricPath(t, metricsRoot, livePaths[0])

	server.Stop(t)
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

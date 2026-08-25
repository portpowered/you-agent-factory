package runtime_metrics_test

import (
	"context"
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
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const functionalRuntimeArtifactTimeLayout = "150405.000000000"

// TestRuntimeMetricsAndArtifactsThroughRootProcess proves that an ordinary
// provider-backed customer flow creates readable metrics, starts retention
// before the live artifact is claimed, and leaves live or unrelated content
// protected. The direct Platform observations below cover retention reports
// and named reservation because no customer command exposes those mechanics.
func TestRuntimeMetricsAndArtifactsThroughRootProcess(t *testing.T) {
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

	retentionReport := sweepFunctionalMetricsWhileLive(t, metricsRoot, livePaths[0])
	if retentionReport.Protected.Files == 0 {
		t.Fatalf("live retention report = %#v, want the claimed live artifact protected", retentionReport)
	}
	if retentionReport.Failed.Files == 0 || !functionalReportContainsPath(retentionReport, functionalMetricsFailurePath(metricsRoot)) {
		t.Fatalf("live retention report = %#v, want a candidate-level failure", retentionReport)
	}

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

	assertFunctionalRuntimeMetricsRecords(t, metricsRoot)
	assertFunctionalNamedReservation(t)
}

func sweepFunctionalMetricsWhileLive(t *testing.T, root, livePath string) platformmetrics.RuntimeMetricsRetentionReport {
	t.Helper()
	failurePath := functionalMetricsFailurePath(root)
	writeLargeFunctionalMetricsFixture(t, failurePath)

	coordination, err := platformmetrics.NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	retention, err := platformmetrics.NewRuntimeMetricsRetention(
		functionalRetentionFileSystem{Local: platformfilesystem.Local{}, failRemove: failurePath},
		time.Now,
		coordination,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	report, err := retention.Sweep(t.Context(), platformmetrics.RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        platformmetrics.RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1, MaxBackups: 2},
	})
	if err != nil {
		t.Fatalf("Sweep(%q, live=%q): %v", root, livePath, err)
	}
	return report
}

func functionalMetricsFailurePath(root string) string {
	return filepath.Join(root, "2020", "01", "02", "010000.000000000-runtime-metrics-failure.log")
}

func assertFunctionalRuntimeMetricsRecords(t *testing.T, root string) {
	t.Helper()
	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader(): %v", err)
	}
	records, err := reader.Read(context.Background(), root)
	if err != nil {
		t.Fatalf("Read(%q): %v", root, err)
	}
	seen := map[string]bool{}
	for _, record := range records {
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
		t.Fatalf("runtime metrics records = %#v, want provider input and output samples", records)
	}
}

func assertFunctionalNamedReservation(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 23, 45, 3, 0, time.FixedZone("PDT", -7*60*60))
	reserver, err := platformruntimeartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver(): %v", err)
	}
	first, err := reserver.ReserveNamed(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamed(first): %v", err)
	}
	if err := os.WriteFile(first, []byte("preserve caller artifact"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", first, err)
	}
	if _, err := reserver.ReserveNamed(root, at, "session", ".jsonl"); !errors.Is(err, platformruntimeartifact.ErrNamedReservationExists) {
		t.Fatalf("ReserveNamed(occupied): %v, want ErrNamedReservationExists", err)
	}
	second, err := reserver.ReserveNamedWithCollision(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision(): %v", err)
	}
	wantFirst := filepath.Join(root, "2026", "05", "30", "session.jsonl")
	if first != wantFirst {
		t.Fatalf("UTC reservation = %q, want %q", first, wantFirst)
	}
	wantSecond := filepath.Join(root, "2026", "05", "30", "session-2.jsonl")
	if second != wantSecond {
		t.Fatalf("collision reservation = %q, want %q", second, wantSecond)
	}
	assertFunctionalFileContents(t, first, "preserve caller artifact")
	info, err := os.Stat(first)
	if err != nil {
		t.Fatalf("Stat(%q): %v", first, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("reserved artifact mode = %#o, want %#o", info.Mode().Perm(), 0o600)
	}
}

func writeFunctionalMetricsFixture(t *testing.T, root, datedDirectory, name, metricName string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(datedDirectory), name)
	writeFunctionalFile(t, path, fmt.Sprintf("{\"metric_name\":%q,\"value\":1}\n", metricName))
	return path
}

func writeLargeFunctionalMetricsFixture(t *testing.T, path string) {
	t.Helper()
	contents := fmt.Sprintf(
		"{\"metric_name\":%q,\"value\":1,\"padding\":%q}\n",
		"failed-removal",
		strings.Repeat("x", 1024*1024+1),
	)
	writeFunctionalFile(t, path, contents)
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

func functionalReportContainsPath(report platformmetrics.RuntimeMetricsRetentionReport, want string) bool {
	for _, failure := range report.Failures {
		if filepath.Clean(failure.Path) == filepath.Clean(want) {
			return true
		}
	}
	return false
}

type functionalRetentionFileSystem struct {
	platformfilesystem.Local
	failRemove string
}

func (filesystem functionalRetentionFileSystem) Remove(path string) error {
	if filepath.Clean(path) == filepath.Clean(filesystem.failRemove) {
		return errors.New("functional retention removal failure")
	}
	return filesystem.Local.Remove(path)
}

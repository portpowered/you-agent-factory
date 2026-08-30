package runtime_metrics_test

import (
	"context"
	"errors"
	"fmt"
	"io"
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

	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, server.URL())
	support.WaitForSessionWorkTerminalFromFactoryEvents(t, server.URL(), "~default", 30*time.Second)
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
	terminalObservation.Wait(30 * time.Second)
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

func TestRuntimeMetricsPublicReaderSelectionAndFailures(t *testing.T) {
	if _, err := platformmetrics.NewRuntimeMetricsReader(nil); err == nil {
		t.Fatal("NewRuntimeMetricsReader(nil) = nil error, want required filesystem error")
	}
	var nilReader *platformmetrics.RuntimeMetricsReader
	if err := nilReader.Stream(context.Background(), t.TempDir(), func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
		t.Fatal("nil reader Stream() = nil error, want typed configuration error")
	}

	var nilReadError *platformmetrics.RuntimeMetricsReadError
	if nilReadError.Error() != "" || nilReadError.Unwrap() != nil {
		t.Fatalf("nil RuntimeMetricsReadError = (%q, %v), want zero values", nilReadError.Error(), nilReadError.Unwrap())
	}
	for _, readError := range []*platformmetrics.RuntimeMetricsReadError{
		{Operation: "read", Cause: errors.New("cause")},
		{Operation: "read", Path: "metrics.log", Cause: errors.New("cause")},
	} {
		if readError.Error() == "" || !errors.Is(readError, readError.Cause) {
			t.Fatalf("RuntimeMetricsReadError = (%q, %v), want operation/path and unwrap", readError.Error(), readError.Cause)
		}
	}

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader(): %v", err)
	}
	root := t.TempDir()
	selectedPath := filepath.Join(root, "2026", "05", "29", "120000.000000000-runtime-metrics-selected.log")
	otherPath := filepath.Join(root, "2026", "05", "29", "120001.000000000-runtime-metrics-other.log")
	skipFilePath := filepath.Join(root, "2026", "05", "29", "120002.000000000-runtime-metrics-skip-file.log")
	skipDirectoryPath := filepath.Join(root, "skip-directory", "2026", "05", "29", "120003.000000000-runtime-metrics-skipped.log")
	writeFunctionalFile(t, selectedPath, "{\"metric_name\":\"selected\",\"session_id\":\"session-1\",\"value\":1}\n")
	writeFunctionalFile(t, otherPath, "{\"metric_name\":\"other\",\"session_id\":\"session-2\",\"value\":2}\n")
	writeFunctionalFile(t, skipFilePath, "{\"metric_name\":\"skip-file\",\"value\":3}\n")
	writeFunctionalFile(t, skipDirectoryPath, "{\"metric_name\":\"skip-directory\",\"value\":4}\n")

	stats := &platformmetrics.RuntimeMetricsReadStats{}
	var records []platformmetrics.RuntimeMetricRecord
	err = reader.StreamSelected(nil, root, platformmetrics.StreamSelection{
		Path: func(path string, isDirectory bool) bool {
			return !strings.Contains(path, "skip")
		},
		EnvelopeFields: []string{"metric_name", "session_id"},
		IncludeEnvelope: func(envelope platformmetrics.RuntimeMetricRecordEnvelope) bool {
			return envelope.Fields["metric_name"] == "selected"
		},
		Stats: stats,
	}, func(record platformmetrics.RuntimeMetricRecord) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamSelected(): %v", err)
	}
	if len(records) != 1 || records[0]["metric_name"] != "selected" || stats.ArtifactsVisited != 2 || stats.ArtifactsOpened != 2 || stats.RecordsDecoded != 1 || stats.BytesRead == 0 {
		t.Fatalf("selected records/stats = (%#v, %#v), want one selected record from two opened artifacts", records, stats)
	}

	incompleteRoot := t.TempDir()
	incompletePath := filepath.Join(incompleteRoot, "2026", "05", "29", "120004.000000000-runtime-metrics-incomplete.log")
	writeFunctionalFile(t, incompletePath, "{\"metric_name\":\"complete\",\"value\":1}\n{\"metric_name\":\"partial\"")
	completeRecords, err := reader.Read(context.Background(), incompleteRoot)
	if err != nil || len(completeRecords) != 1 {
		t.Fatalf("Read(incomplete artifact) = (%#v, %v), want one complete record and no error", completeRecords, err)
	}

	malformedRoot := t.TempDir()
	malformedPath := filepath.Join(malformedRoot, "2026", "05", "29", "120005.000000000-runtime-metrics-malformed.log")
	writeFunctionalFile(t, malformedPath, "not-json\n")
	if _, err := reader.Read(context.Background(), malformedRoot); err == nil {
		t.Fatal("Read(malformed artifact) = nil error, want typed decode error")
	}
	visitorErr := errors.New("visitor stopped")
	if err := reader.Stream(context.Background(), root, func(platformmetrics.RuntimeMetricRecord) error { return visitorErr }); !errors.Is(err, visitorErr) {
		t.Fatalf("Stream(visitor failure) = %v, want %v", err, visitorErr)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Read(canceled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read(canceled) = %v, want context canceled", err)
	}
	if _, err := reader.Read(context.Background(), ""); err == nil {
		t.Fatal("Read(empty root) = nil error, want root validation error")
	}
	nonDirectory := filepath.Join(t.TempDir(), "metrics.log")
	writeFunctionalFile(t, nonDirectory, "not a root")
	if _, err := reader.Read(context.Background(), nonDirectory); err == nil {
		t.Fatal("Read(file root) = nil error, want not-a-directory error")
	}
}

func TestRuntimeMetricsPublicCoordinationAndOpenerValidation(t *testing.T) {
	coordination, err := platformmetrics.NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	root := filepath.Join(t.TempDir(), "metrics")
	if _, err := coordination.LockRoot(nil, ""); err == nil {
		t.Fatal("LockRoot(empty path) = nil error, want path validation error")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordination.LockRoot(canceled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("LockRoot(canceled) = %v, want context canceled", err)
	}
	rootLock, err := coordination.LockRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("LockRoot(): %v", err)
	}
	if _, err := coordination.TryLockRoot(root); !errors.Is(err, platformmetrics.ErrRuntimeMetricsRootBusy) {
		t.Fatalf("TryLockRoot(held) = %v, want root busy", err)
	}
	if err := rootLock.Close(); err != nil {
		t.Fatalf("root lock Close(): %v", err)
	}
	claimPath := filepath.Join(root, "2026", "05", "29", "120006.000000000-runtime-metrics-claimed.log")
	claim, err := coordination.Claim(claimPath)
	if err != nil {
		t.Fatalf("Claim(): %v", err)
	}
	if _, err := coordination.TryClaim(claimPath); !errors.Is(err, platformmetrics.ErrRuntimeMetricsArtifactBusy) {
		t.Fatalf("TryClaim(held) = %v, want artifact busy", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("claim Close(): %v", err)
	}
}

func TestRuntimeMetricsPublicOpenerValidatesRequestsAndCloses(t *testing.T) {
	paths, err := platformruntimeartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver(): %v", err)
	}
	coordination, err := platformmetrics.NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	lifecycle := functionalMetricsRetentionLifecycle{}
	if _, err := platformmetrics.NewRuntimeMetricsOpener(nil, lifecycle); err == nil {
		t.Fatal("NewRuntimeMetricsOpener(nil paths) = nil error, want required reserver error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsOpener(paths, nil); err == nil {
		t.Fatal("NewRuntimeMetricsOpener(nil lifecycle) = nil error, want required lifecycle error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsOpener(paths, lifecycle, coordination, coordination); err == nil {
		t.Fatal("NewRuntimeMetricsOpener(two coordinators) = nil error, want selection error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsOpener(paths, lifecycle, nil); err == nil {
		t.Fatal("NewRuntimeMetricsOpener(nil coordinator) = nil error, want required coordination error")
	}

	root := filepath.Join(t.TempDir(), "metrics")
	opener, err := platformmetrics.NewRuntimeMetricsOpener(paths, lifecycle, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsOpener(): %v", err)
	}
	valid := platformmetrics.RuntimeMetricsOpeningRequest{
		SessionID:         "functional-session",
		RuntimeInstanceID: "functional-runtime",
		FolderPath:        root,
		FactoryDirectory:  root,
		RootDirectory:     root,
		StartTimeUTC:      time.Date(2026, time.May, 29, 23, 45, 3, 0, time.FixedZone("PDT", -7*60*60)),
		CollisionID:       "functional-collision",
		Config:            platformmetrics.RuntimeMetricsConfig{MaxSize: -1, MaxBackups: -1, MaxAge: -1},
	}
	for name, edit := range map[string]func(*platformmetrics.RuntimeMetricsOpeningRequest){
		"runtime": func(request *platformmetrics.RuntimeMetricsOpeningRequest) { request.RuntimeInstanceID = "" },
		"root":    func(request *platformmetrics.RuntimeMetricsOpeningRequest) { request.RootDirectory = "" },
		"clock":   func(request *platformmetrics.RuntimeMetricsOpeningRequest) { request.StartTimeUTC = time.Time{} },
		"collision": func(request *platformmetrics.RuntimeMetricsOpeningRequest) {
			request.CollisionID = ""
		},
	} {
		request := valid
		edit(&request)
		if _, err := opener.Open(request); err == nil {
			t.Fatalf("Open(%s) = nil error, want validation error", name)
		}
	}

	sink, err := opener.Open(valid)
	if err != nil {
		t.Fatalf("Open(valid): %v", err)
	}
	if sink.Config() != platformmetrics.DefaultRuntimeMetricsConfig() || sink.StartTimeUTC().Location() != time.UTC {
		t.Fatalf("sink configuration = (%#v, %v), want normalized defaults and UTC", sink.Config(), sink.StartTimeUTC().Location())
	}
	if err := sink.WriteMetric(context.Background(), map[string]any{"metric_name": "functional.validation", "value": 1}); err != nil {
		t.Fatalf("WriteMetric(): %v", err)
	}
	canceledWrite, cancelWrite := context.WithCancel(context.Background())
	cancelWrite()
	if err := sink.WriteMetric(canceledWrite, map[string]any{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteMetric(canceled) = %v, want context canceled", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("sink Close(): %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("sink Close(second): %v", err)
	}
	if err := sink.WriteMetric(context.Background(), map[string]any{}); err == nil {
		t.Fatal("WriteMetric(after close) = nil error, want closed sink error")
	}
	if err := opener.Close(context.Background()); err != nil {
		t.Fatalf("opener Close(): %v", err)
	}
	var nilOpener *platformmetrics.RuntimeMetricsOpener
	if err := nilOpener.Close(context.Background()); err != nil {
		t.Fatalf("nil opener Close() = %v, want nil", err)
	}
}

type functionalMetricsRetentionLifecycle struct{}

func (functionalMetricsRetentionLifecycle) Start(context.Context, platformmetrics.RuntimeMetricsRetentionRequest) (io.Closer, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (functionalMetricsRetentionLifecycle) Close(context.Context) error { return nil }

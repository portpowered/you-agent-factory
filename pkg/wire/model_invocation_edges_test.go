package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
)

func TestModelsManagedProcessStopAfterNaturalExitIsClean(t *testing.T) {
	if os.Getenv("GO_WANT_MODELS_HOST_EXIT_HELPER") == "1" {
		return
	}

	managed, err := (modelsProcessLauncher{}).Start(context.Background(), serviceedges.HostProcessStartSpec{
		Command:        os.Args[0],
		Args:           []string{"-test.run=^TestModelsManagedProcessStopAfterNaturalExitIsClean$"},
		Env:            append(os.Environ(), "GO_WANT_MODELS_HOST_EXIT_HELPER=1"),
		HealthEndpoint: "grpc://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("start exited host helper: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- managed.Wait()
	}()
	if err := <-done; err != nil {
		t.Fatalf("natural host exit = %v, want nil", err)
	}
	if err := managed.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() after natural host exit = %v, want nil", err)
	}
	if err := managed.Stop(context.Background()); err != nil {
		t.Fatalf("repeated Stop() after natural host exit = %v, want nil", err)
	}
}

func TestModelsProcessLauncherObservesManagedWindowsChildEnvironment(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("managed child environment proof is Windows-specific")
	}
	t.Parallel()

	root := t.TempDir()
	cmdPath, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatalf("locate Windows command interpreter: %v", err)
	}
	managedExecutable := filepath.Join(root, "vibevoice-cpp.exe")
	commandBody, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("read Windows command interpreter: %v", err)
	}
	if err := os.WriteFile(managedExecutable, commandBody, 0o700); err != nil {
		t.Fatalf("prepare prebuilt managed command: %v", err)
	}
	managedLibrary := filepath.Join(root, "libgovibevoicecpp.dll")
	if err := os.WriteFile(managedLibrary, []byte("controlled DLL marker"), 0o600); err != nil {
		t.Fatalf("prepare managed library marker: %v", err)
	}
	environmentDump := filepath.Join(root, "child-environment.txt")
	staleLibrary := filepath.Join(root, "stale-library.dll")
	environment := appendManagedBackendEnvironment(append([]string(nil), os.Environ()...), []string{
		"TEMP=" + root,
		"TMP=" + root,
		"vIbEvOiCeCpP_LiBrArY=" + staleLibrary,
	})
	evidencePath := filepath.Join(root, "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: evidencePath}
	managed, err := (modelsProcessLauncher{recorder: recorder}).Start(
		context.Background(),
		serviceedges.HostProcessStartSpec{
			Backend:      "localai-vibevoice",
			BackendFiles: []string{managedExecutable},
			WorkDir:      root,
			Env:          environment,
			Args: []string{
				"/c",
				"set > child-environment.txt & exit /b 0",
			},
		},
	)
	if err != nil {
		t.Fatalf("start controlled managed child: %v", err)
	}
	defer managed.Stop(context.Background())

	waitErr := waitForManagedProcess(t, managed)
	if waitErr != nil {
		t.Fatalf("controlled managed child exit = %v, want success", waitErr)
	}
	dump, err := os.ReadFile(environmentDump)
	if err != nil {
		t.Fatalf("read child environment dump: %v", err)
	}
	childEnvironment := parseEnvironmentDump(string(dump))
	wantEnvironment := map[string]string{
		"PATH":                 requiredEnvironmentValue(t, environment, "PATH"),
		"TEMP":                 root,
		"TMP":                  root,
		"VIBEVOICECPP_LIBRARY": managedLibrary,
	}
	for name, want := range wantEnvironment {
		if got := childEnvironment[strings.ToUpper(name)]; got != want {
			t.Fatalf("child %s = %q, want managed value %q", name, got, want)
		}
	}

	records := readManagedChildEvidence(t, evidencePath)
	if len(records) != 2 {
		t.Fatalf("managed child evidence records = %d, want start and exit: %#v", len(records), records)
	}
	started, exited := records[0], records[1]
	if started.Kind != managedChildEvidenceKind || started.Phase != managedChildPhaseStarted ||
		started.ProcessID <= 0 || exited.Kind != managedChildEvidenceKind ||
		exited.Phase != managedChildPhaseExited || exited.ProcessID != started.ProcessID ||
		exited.ExitClass != managedChildExitClassExited {
		t.Fatalf("managed child lifecycle evidence = %#v, want one PID with start/exit phases", records)
	}
	wantDigests := map[string]string{
		"PATH":                 environmentValueSHA256(wantEnvironment["PATH"]),
		"TEMP":                 environmentValueSHA256(root),
		"TMP":                  environmentValueSHA256(root),
		"VIBEVOICECPP_LIBRARY": environmentValueSHA256(managedLibrary),
	}
	if len(started.Environment) != len(wantDigests) {
		t.Fatalf("started environment facts = %#v, want four allowlisted facts", started.Environment)
	}
	for _, fact := range started.Environment {
		if !fact.Present || fact.ValueSHA256 != wantDigests[fact.Name] {
			t.Fatalf("started environment fact = %#v, want bounded digest", fact)
		}
	}
	body, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("read managed child evidence: %v", err)
	}
	for _, marker := range []string{root, staleLibrary, managedLibrary, environmentDump, requiredEnvironmentValue(t, environment, "PATH")} {
		if strings.Contains(string(body), marker) {
			t.Fatalf("managed child evidence leaked raw value %q: %s", marker, body)
		}
	}
	if !bytes.Contains(body, []byte(`"sequence":1`)) || !bytes.Contains(body, []byte(`"sequence":2`)) {
		t.Fatalf("managed child evidence sequence = %s, want ordered records", body)
	}
}

func TestModelsProcessLauncherRecordsNonzeroManagedChildExit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("managed child exit proof is Windows-specific")
	}
	t.Parallel()

	cmdPath, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatalf("locate Windows command interpreter: %v", err)
	}
	evidencePath := filepath.Join(t.TempDir(), "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: evidencePath}
	managed, err := (modelsProcessLauncher{recorder: recorder}).Start(
		context.Background(),
		serviceedges.HostProcessStartSpec{
			Command:        cmdPath,
			Args:           []string{"/c", "exit /b 7"},
			Backend:        "localai-vibevoice",
			HealthEndpoint: "grpc://127.0.0.1:1",
		},
	)
	if err != nil {
		t.Fatalf("start nonzero managed child: %v", err)
	}
	defer managed.Stop(context.Background())
	if waitErr := waitForManagedProcess(t, managed); waitErr == nil {
		t.Fatal("nonzero managed child exit = nil, want process exit error")
	}
	records := readManagedChildEvidence(t, evidencePath)
	if len(records) != 2 || records[0].ProcessID <= 0 || records[1].ProcessID != records[0].ProcessID ||
		records[0].Phase != managedChildPhaseStarted || records[1].Phase != managedChildPhaseExited ||
		records[1].ExitClass != managedChildExitClassNonzero || len(records[1].Environment) != 0 {
		t.Fatalf("nonzero managed child evidence = %#v, want distinct bounded exit", records)
	}
}

func waitForManagedProcess(t *testing.T, process interface{ Wait() error }) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatal("timed out waiting for controlled managed child")
		return nil
	}
}

func readManagedChildEvidence(t *testing.T, path string) []managedChildEnvironmentEvidence {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read managed child evidence %q: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	var records []managedChildEnvironmentEvidence
	for {
		var record managedChildEnvironmentEvidence
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatalf("decode managed child evidence: %v", err)
		}
		records = append(records, record)
	}
}

func parseEnvironmentDump(dump string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(dump, "\r\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.ToUpper(key)] = value
		}
	}
	return values
}

func requiredEnvironmentValue(t *testing.T, environment []string, name string) string {
	t.Helper()
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, name) {
			return value
		}
	}
	t.Fatalf("required environment value %s is missing", name)
	return ""
}

var (
	_ modelswire.AssetHTTPDoer                = serviceedges.Edges{}.ModelAssetHTTPClient
	_ modelswire.HostHTTPDoer                 = serviceedges.Edges{}.ModelHostHTTPClient
	_ modelswire.RuntimeHTTPDoer              = serviceedges.Edges{}.ModelRuntimeHTTPClient
	_ modelswire.InvocationArtifactFileSystem = serviceedges.Edges{}.ModelInvocationArtifactFileSystem
	_ modelswire.HostProcessLauncher          = modelHostProcessLauncherAdapter{}
	_ modelswire.HostClock                    = modelHostClockAdapter{}
	_ modelswire.RuntimeCreateTempFile        = adaptModelRuntimeTempFile(nil)
	_ modelswire.PullMetricsRecorder          = modelsPullMetricsAdapter{}
)

func TestModelsServiceIsConstructedOnceAndOpensRuntimeScopeOnSameRoot(t *testing.T) {
	t.Parallel()

	root, err := provideModelsService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideModelsService: %v", err)
	}
	if _, err := root.ListCatalog(context.Background(), models.ListModelsRequest{}); err == nil {
		t.Fatal("unbound Models service unexpectedly accepted a catalog operation")
	}
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: t.TempDir(),
			Runtime:        models.RuntimeConfig{},
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	if opened.Scope.IsZero() {
		t.Fatal("OpenRuntimeScope returned a zero scope")
	}
	if _, err := root.ListCatalog(context.Background(), models.ListModelsRequest{
		Scope: opened.Scope,
	}); err != nil {
		t.Fatalf("same process-scoped Models root rejected its opened scope: %v", err)
	}
	closed, err := root.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	if !closed.Closed || closed.Scope != opened.Scope {
		t.Fatalf("CloseRuntimeScope result = %#v, want issued scope closed", closed)
	}
}

type workingDirectoryOverride struct{}

func (*workingDirectoryOverride) Getwd() (string, error) { return "override", nil }

type artifactWriteCloser struct{ bytes.Buffer }

func (*artifactWriteCloser) Close() error { return nil }

type invocationArtifactFileSystemOverride struct {
	opened  string
	created string
	output  *artifactWriteCloser
}

type portableFileSystemOverride struct {
	platformfilesystem.Local
	walked bool
}

func (f *portableFileSystemOverride) WalkDir(string, fs.WalkDirFunc) error {
	f.walked = true
	return nil
}

func TestFactoryDefinitionPortableFileSystemPreservesOverrideAndSelectsDefault(t *testing.T) {
	t.Parallel()

	selectedDefault := provideFactoryDefinitionPortableFileSystem(serviceedges.Edges{})
	if _, ok := selectedDefault.(platformfilesystem.Local); !ok {
		t.Fatalf("default portable filesystem = %T, want platform local adapter", selectedDefault)
	}
	if err := selectedDefault.WalkDir(t.TempDir(), func(string, fs.DirEntry, error) error { return nil }); err != nil {
		t.Fatalf("default portable directory walker: %v", err)
	}

	override := &portableFileSystemOverride{}
	selected := provideFactoryDefinitionPortableFileSystem(serviceedges.Edges{
		FactoryDefinitionPortableFileSystem: override,
	})
	if selected != override {
		t.Fatal("portable filesystem override was not selected")
	}
	if err := selected.WalkDir("unused", nil); err != nil {
		t.Fatalf("portable directory walker override: %v", err)
	}
	if !override.walked {
		t.Fatal("portable directory walker override was not selected")
	}
}

func (s *invocationArtifactFileSystemOverride) Open(path string) (io.ReadCloser, error) {
	s.opened = path
	return io.NopCloser(bytes.NewBufferString("audio")), nil
}

func (s *invocationArtifactFileSystemOverride) Create(path string) (io.WriteCloser, error) {
	s.created = path
	s.output = &artifactWriteCloser{}
	return s.output, nil
}

func TestModelInvocationEdgesPreserveOverridesAndSelectPlatformDefaults(t *testing.T) {
	t.Parallel()

	if _, ok := provideFactorySessionsWorkingDirectory(serviceedges.Edges{}).(platformfilesystem.Local); !ok {
		t.Fatalf("default working-directory edge = %T, want platform filesystem adapter", provideFactorySessionsWorkingDirectory(serviceedges.Edges{}))
	}
	workingOverride := &workingDirectoryOverride{}
	if got := provideFactorySessionsWorkingDirectory(serviceedges.Edges{FactorySessionsWorkingDirectory: workingOverride}); got != workingOverride {
		t.Fatalf("working-directory override = %#v, want original override", got)
	}

	filesystemOverride := &invocationArtifactFileSystemOverride{}
	exporter, err := provideModelInvocationArtifactExporter(serviceedges.Edges{
		ModelInvocationArtifactFileSystem: filesystemOverride,
	})
	if err != nil {
		t.Fatalf("provideModelInvocationArtifactExporter: %v", err)
	}
	if err := exporter.ExportInvocationArtifact("runtime.wav", "customer.wav"); err != nil {
		t.Fatalf("ExportInvocationArtifact: %v", err)
	}
	if filesystemOverride.opened != "runtime.wav" || filesystemOverride.created != "customer.wav" || filesystemOverride.output.String() != "audio" {
		t.Fatalf("artifact override observed (%q, %q, %q)", filesystemOverride.opened, filesystemOverride.created, filesystemOverride.output.String())
	}
	if got := provideModelInvocationTimeout(); got != factorysessions.DefaultModelInvocationTimeout {
		t.Fatalf("model invocation timeout = %v, want %v", got, factorysessions.DefaultModelInvocationTimeout)
	}
}

func TestModelAssetHostPlatformPreservesOverrideAndSelectsProcessDefault(t *testing.T) {
	t.Parallel()

	if got := provideModelAssetHostPlatform(serviceedges.Edges{}); got != (models.AssetHostPlatform{
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
	}) {
		t.Fatalf("default model asset host platform = %#v, want current process platform", got)
	}

	override := models.AssetHostPlatform{OperatingSystem: "customer-os", Architecture: "customer-arch"}
	if got := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: override}); got != override {
		t.Fatalf("model asset host platform override = %#v, want %#v", got, override)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestModelsCompositionAdaptsEdgePortsAtTheWireBoundary(t *testing.T) {
	t.Parallel()

	process := &modelEdgeManagedProcess{healthEndpoint: "http://model-host/health"}
	var gotSpec serviceedges.HostProcessStartSpec
	launcher := adaptModelHostProcessLauncher(&modelEdgeProcessLauncher{
		process: process,
		gotSpec: &gotSpec,
	})
	gotProcess, err := launcher.Start(context.Background(), modelswire.HostProcessStartSpec{
		Command: "model-host", Args: []string{"serve"}, Env: []string{"MODEL=seal"},
		WorkDir: "runtime", HealthEndpoint: process.healthEndpoint,
		Backend: "localai-llamacpp", ModelPath: "runtime/model.gguf",
		BackendFiles: []string{"runtime/backend.zip"},
	})
	if err != nil {
		t.Fatalf("adapted process launcher: %v", err)
	}
	if gotSpec.Command != "model-host" || len(gotSpec.Args) != 1 || gotSpec.Args[0] != "serve" ||
		len(gotSpec.Env) != 1 || gotSpec.Env[0] != "MODEL=seal" || gotSpec.WorkDir != "runtime" ||
		gotSpec.HealthEndpoint != process.healthEndpoint || gotSpec.Backend != "localai-llamacpp" ||
		gotSpec.ModelPath != "runtime/model.gguf" || len(gotSpec.BackendFiles) != 1 ||
		gotSpec.BackendFiles[0] != "runtime/backend.zip" {
		t.Fatalf("adapted process spec = %#v, want exact edge projection", gotSpec)
	}
	if gotProcess.HealthEndpoint() != process.healthEndpoint {
		t.Fatalf("adapted process health endpoint = %q, want %q", gotProcess.HealthEndpoint(), process.healthEndpoint)
	}
	if err := gotProcess.Stop(context.Background()); err != nil {
		t.Fatalf("adapted managed process Stop: %v", err)
	}
	if !process.stopped {
		t.Fatal("adapted managed process did not preserve the edge process")
	}

	timer := &modelEdgeTimer{}
	clock := adaptModelHostClock(modelEdgeClock{timer: timer})
	if got := clock.Now(); !got.Equal(modelEdgeClockTime) {
		t.Fatalf("adapted host clock Now = %v, want %v", got, modelEdgeClockTime)
	}
	if got := clock.NewTimer(time.Second); got != timer {
		t.Fatal("adapted host clock did not preserve the edge timer")
	}

	tempFile := &modelEdgeTempFile{name: "runtime.tmp"}
	createTempFile := adaptModelRuntimeTempFile(func(string, string) (interface {
		Close() error
		Name() string
	}, error) {
		return tempFile, nil
	})
	gotTempFile, err := createTempFile("runtime", "model-*")
	if err != nil {
		t.Fatalf("adapted runtime temp file: %v", err)
	}
	if gotTempFile.Name() != tempFile.name {
		t.Fatalf("adapted temp file name = %q, want %q", gotTempFile.Name(), tempFile.name)
	}

	labels := map[string]string{"model": "seal"}
	recorder := &modelEdgePullMetricsRecorder{}
	adaptedRecorder := adaptModelsPullMetricsRecorder(recorder)
	adaptedRecorder.RecordModelPullMetric(modelswire.PullMetric{Name: "model.pull", Labels: labels})
	labels["model"] = "mutated-after-record"
	if recorder.metric.Name != "model.pull" || recorder.metric.Labels["model"] != "seal" {
		t.Fatalf("adapted pull metric = %#v, want copied edge metric", recorder.metric)
	}
}

func TestModelsCompositionRejectsTypedNilHostEdges(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		edges serviceedges.Edges
		want  string
	}{
		{
			name: "process launcher",
			edges: serviceedges.Edges{
				ModelHostProcessLauncher: (*modelEdgeProcessLauncher)(nil),
			},
			want: "model host process launcher is required",
		},
		{
			name: "clock",
			edges: serviceedges.Edges{
				ModelHostClock: (*modelEdgeClock)(nil),
			},
			want: "model host clock is required",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := provideModelsService(testCase.edges)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("provideModelsService() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestModelsCompositionRejectsMissingAssetStagingCoordination(t *testing.T) {
	t.Parallel()

	_, err := provideModelsService(serviceedges.Edges{
		ModelAssetStagingCoordinationFactory: func() (serviceedges.AssetStagingCoordination, error) {
			return nil, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Models Assets staging coordination is required") {
		t.Fatalf("provideModelsService() error = %v, want missing staging coordination diagnostic", err)
	}
}

func TestModelsCompositionAdaptsProtocolAndCompatibilityPorts(t *testing.T) {
	t.Parallel()

	request := modelEdgeProtocolRequest()
	assertAdaptedProtocolNegotiation(t, request)
	assertAdaptedCompatibility(t, request)
	assertAdaptedGRPCConnection(t, request)
	assertAdaptedOptionalPorts(t)
}

func modelEdgeProtocolRequest() modelswire.HostProtocolNegotiationRequest {
	return modelswire.HostProtocolNegotiationRequest{
		ProtocolVersion: "model-host.v1", Backend: "localai-vibevoice", ModelName: "tts",
		Revision:  "revision-1",
		Platform:  models.AssetHostPlatform{OperatingSystem: "test-os", Architecture: "test-arch"},
		ModelPath: "runtime/model.gguf", ModelFiles: []string{"runtime/model.gguf", "runtime/tokenizer.gguf"},
	}
}

func assertAdaptedProtocolNegotiation(t *testing.T, request modelswire.HostProtocolNegotiationRequest) {
	t.Helper()
	protocol := &modelEdgeProtocolNegotiator{}
	adaptedProtocol := adaptModelHostProtocolNegotiator(protocol)
	result, err := adaptedProtocol.Negotiate(context.Background(), "grpc://model-host", request)
	if err != nil {
		t.Fatalf("protocol negotiation: %v", err)
	}
	if protocol.endpoint != "grpc://model-host" || protocol.request.ProtocolVersion != request.ProtocolVersion ||
		protocol.request.Backend != request.Backend || protocol.request.ModelName != request.ModelName ||
		protocol.request.Revision != request.Revision || protocol.request.Platform != request.Platform ||
		protocol.request.ModelPath != request.ModelPath || !equalStringSlices(protocol.request.ModelFiles, request.ModelFiles) {
		t.Fatalf("edge protocol request = %#v at %q, want exact projection", protocol.request, protocol.endpoint)
	}
	if result != (modelswire.HostProtocolNegotiationResult{
		ProtocolVersion: "model-host.v1", Backend: request.Backend, Ready: true,
	}) {
		t.Fatalf("protocol result = %#v, want ready pinned result", result)
	}
}

func assertAdaptedCompatibility(t *testing.T, request modelswire.HostProtocolNegotiationRequest) {
	t.Helper()
	compatibility := &modelEdgeCompatibilityChecker{}
	if err := adaptModelHostCompatibilityChecker(compatibility).Check(context.Background(), modelswire.HostCompatibilityRequest{
		Backend: request.Backend, ModelName: request.ModelName, Revision: request.Revision, Platform: request.Platform,
	}); err != nil {
		t.Fatalf("compatibility check: %v", err)
	}
	if compatibility.request.Backend != request.Backend || compatibility.request.ModelName != request.ModelName ||
		compatibility.request.Revision != request.Revision || compatibility.request.Platform != request.Platform {
		t.Fatalf("edge compatibility request = %#v, want exact projection", compatibility.request)
	}
}

func assertAdaptedGRPCConnection(t *testing.T, request modelswire.HostProtocolNegotiationRequest) {
	t.Helper()
	connection := &modelEdgeGRPCConnection{}
	dialer := modelHostGRPCDialerAdapter{next: &modelEdgeGRPCDialer{connection: connection}}
	adaptedConnection, err := dialer.Dial(context.Background(), "grpc://model-host")
	if err != nil {
		t.Fatalf("dial model host: %v", err)
	}
	if _, err := adaptedConnection.Negotiate(context.Background(), request); err != nil {
		t.Fatalf("dialed protocol negotiation: %v", err)
	}
	if err := adaptedConnection.Close(); err != nil {
		t.Fatalf("close model host connection: %v", err)
	}
	if connection.request.Backend != request.Backend || connection.request.ModelPath != request.ModelPath ||
		!equalStringSlices(connection.request.ModelFiles, request.ModelFiles) || !connection.closed {
		t.Fatalf("dialed connection state = %#v, want request and close", connection)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertAdaptedOptionalPorts(t *testing.T) {
	t.Helper()
	if adaptModelHostProtocolNegotiator(nil) != nil {
		t.Fatal("nil protocol negotiator should stay nil")
	}
	if adaptModelHostCompatibilityChecker(nil) != nil {
		t.Fatal("nil compatibility checker should stay nil")
	}
	if got := (modelsClock{source: modelEdgeClock{}}).Now(); !got.Equal(modelEdgeClockTime) {
		t.Fatalf("injected models clock = %v, want %v", got, modelEdgeClockTime)
	}
	if got := (modelsClock{}).Now(); !got.IsZero() {
		t.Fatalf("empty models clock = %v, want zero time", got)
	}
}

var modelEdgeClockTime = time.Unix(1_725_000_000, 0)

type modelEdgeManagedProcess struct {
	healthEndpoint string
	stopped        bool
}

func (process *modelEdgeManagedProcess) HealthEndpoint() string { return process.healthEndpoint }
func (*modelEdgeManagedProcess) Wait() error                    { return nil }
func (process *modelEdgeManagedProcess) Stop(context.Context) error {
	process.stopped = true
	return nil
}

type modelEdgeProcessLauncher struct {
	process *modelEdgeManagedProcess
	gotSpec *serviceedges.HostProcessStartSpec
}

func (launcher *modelEdgeProcessLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	*launcher.gotSpec = spec
	return launcher.process, nil
}

type modelEdgeTimer struct{}

func (*modelEdgeTimer) C() <-chan time.Time { return nil }
func (*modelEdgeTimer) Stop() bool          { return true }

type modelEdgeClock struct {
	timer interface {
		C() <-chan time.Time
		Stop() bool
	}
}

func (modelEdgeClock) Now() time.Time { return modelEdgeClockTime }
func (clock modelEdgeClock) NewTimer(time.Duration) interface {
	C() <-chan time.Time
	Stop() bool
} {
	return clock.timer
}

type modelEdgeTempFile struct {
	name string
}

func (file *modelEdgeTempFile) Close() error { return nil }
func (file *modelEdgeTempFile) Name() string { return file.name }

type modelEdgePullMetricsRecorder struct {
	metric serviceedges.PullMetric
}

func (recorder *modelEdgePullMetricsRecorder) RecordModelPullMetric(metric serviceedges.PullMetric) {
	recorder.metric = metric
}

type modelEdgeProtocolNegotiator struct {
	endpoint string
	request  serviceedges.ModelHostProtocolNegotiationRequest
}

func (negotiator *modelEdgeProtocolNegotiator) Negotiate(
	_ context.Context,
	endpoint string,
	request serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	negotiator.endpoint = endpoint
	negotiator.request = request
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: request.ProtocolVersion,
		Backend:         request.Backend,
		Ready:           true,
	}, nil
}

type modelEdgeCompatibilityChecker struct {
	request serviceedges.ModelHostCompatibilityRequest
}

func (checker *modelEdgeCompatibilityChecker) Check(
	_ context.Context,
	request serviceedges.ModelHostCompatibilityRequest,
) error {
	checker.request = request
	return nil
}

type modelEdgeGRPCDialer struct {
	connection *modelEdgeGRPCConnection
}

func (dialer *modelEdgeGRPCDialer) Dial(context.Context, string) (interface {
	Negotiate(
		context.Context,
		serviceedges.ModelHostProtocolNegotiationRequest,
	) (serviceedges.ModelHostProtocolNegotiationResult, error)
	Close() error
}, error) {
	return dialer.connection, nil
}

type modelEdgeGRPCConnection struct {
	request serviceedges.ModelHostProtocolNegotiationRequest
	closed  bool
}

func (connection *modelEdgeGRPCConnection) Negotiate(
	_ context.Context,
	request serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	connection.request = request
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: request.ProtocolVersion,
		Backend:         request.Backend,
		Ready:           true,
	}, nil
}

func (connection *modelEdgeGRPCConnection) Close() error {
	connection.closed = true
	return nil
}

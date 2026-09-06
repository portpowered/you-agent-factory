package wire

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
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
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
		Revision: "revision-1",
		Platform: models.AssetHostPlatform{OperatingSystem: "test-os", Architecture: "test-arch"},
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
		protocol.request.Revision != request.Revision || protocol.request.Platform != request.Platform {
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
	if connection.request.Backend != request.Backend || !connection.closed {
		t.Fatalf("dialed connection state = %#v, want request and close", connection)
	}
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

func TestWorkerRecordingRootUsesCanonicalHomeAndScenarioHome(t *testing.T) {
	t.Parallel()

	defaultRoot, err := workerRecordingRoot(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("workerRecordingRoot(default) error = %v", err)
	}
	defaultHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	if want := filepath.Join(defaultHome, workerRecordingHomeDirectory, workerRecordingStoreDirectory); defaultRoot != want {
		t.Fatalf("workerRecordingRoot(default) = %q, want %q", defaultRoot, want)
	}

	home := t.TempDir()
	resolvedRoot, err := workerRecordingRoot(serviceedges.Edges{
		WorkerSessionResolveHomeDirectory: func() (string, error) { return home, nil },
	})
	if err != nil {
		t.Fatalf("workerRecordingRoot(scenario) error = %v", err)
	}
	want := filepath.Join(home, workerRecordingHomeDirectory, workerRecordingStoreDirectory)
	if resolvedRoot != want {
		t.Fatalf("workerRecordingRoot(scenario) = %q, want %q", resolvedRoot, want)
	}
	if filepath.Dir(resolvedRoot) == filepath.Dir(defaultRoot) {
		t.Fatalf("scenario recording root %q unexpectedly shares the temporary parent %q", resolvedRoot, defaultRoot)
	}
}

func TestProvideWorkerRecordingWriterPersistsUnderResolvedHome(t *testing.T) {
	t.Parallel()

	for _, home := range []string{t.TempDir(), t.TempDir()} {
		writer, err := provideWorkerRecordingWriter(serviceedges.Edges{
			WorkerSessionResolveHomeDirectory: func() (string, error) { return home, nil },
		})
		if err != nil {
			t.Fatalf("provideWorkerRecordingWriter() error = %v", err)
		}
		failureWriter, ok := writer.(recordings.WorkerRecordingFailureWriter)
		if !ok || failureWriter == nil {
			t.Fatalf("worker recording writer type %T does not expose failure persistence", writer)
		}
		if err := failureWriter.PersistWorkerRecordingFailure(context.Background(), recordings.WorkerRecordingFailure{
			RecordingID:     "recording-home-seam",
			WorkerSessionID: "worker-home-seam",
			Topic:           "worker-session/worker-home-seam/events",
			Code:            "PERSISTENCE_FAILED",
		}); err != nil {
			t.Fatalf("PersistWorkerRecordingFailure() error = %v", err)
		}

		root := filepath.Join(home, workerRecordingHomeDirectory, workerRecordingStoreDirectory)
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read resolved Worker recording root %q: %v", root, err)
		}
		if len(entries) != 1 || entries[0].IsDir() {
			t.Fatalf("resolved Worker recording entries = %#v, want one artifact", entries)
		}
		reader, ok := writer.(recordings.WorkerRecordingReader)
		if !ok || reader == nil {
			t.Fatalf("worker recording type %T does not expose reading", writer)
		}
		snapshot, err := reader.LoadWorkerRecording(context.Background(), "recording-home-seam")
		if err != nil || snapshot.RecordingID != "recording-home-seam" {
			t.Fatalf("LoadWorkerRecording() = (%#v, %v), want persisted scenario identity", snapshot, err)
		}
	}
}

func TestWorkerRecordingRootReportsResolverFailures(t *testing.T) {
	t.Parallel()

	resolverErr := errors.New("home unavailable")
	if root, err := workerRecordingRoot(serviceedges.Edges{
		WorkerSessionResolveHomeDirectory: func() (string, error) { return "", resolverErr },
	}); root != "" || !errors.Is(err, resolverErr) {
		t.Fatalf("workerRecordingRoot(failing resolver) = (%q, %v), want wrapped resolver error", root, err)
	}
	if root, err := workerRecordingRoot(serviceedges.Edges{
		WorkerSessionResolveHomeDirectory: func() (string, error) { return "  ", nil },
	}); root != "" || err == nil {
		t.Fatalf("workerRecordingRoot(empty resolver) = (%q, %v), want empty-path error", root, err)
	}
}

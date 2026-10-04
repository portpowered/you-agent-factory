package wire

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	"go.uber.org/zap"
)

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

// Existing component fixtures supply the same separately selected roles as Wire.
func newModelsServiceFixture(edges serviceedges.Edges) (models.Service, error) {
	edges = selectedTestTimeEdges(edges)
	scopes, err := provideModelRuntimeScopes()
	if err != nil {
		return nil, err
	}
	assets, err := newModelAssetsFixture(edges, scopes)
	if err != nil {
		return nil, err
	}
	catalog, err := modelswire.NewCatalog(scopes, modelswire.NewCatalogReadinessQuery(assets))
	if err != nil {
		return nil, err
	}
	source, err := provideModelRuntimeEvidenceSource()
	if err != nil {
		return nil, err
	}
	evidence := provideModelOrderedRuntimeEvidence(source)
	host, err := newModelHostFixture(edges, scopes, assets, source, evidence)
	if err != nil {
		return nil, err
	}
	runner, err := provideModelRuntimeRunner(edges)
	if err != nil {
		return nil, err
	}
	temp, create, inspect := provideModelRuntimeTempDirectory(edges), provideModelRuntimeTempFile(edges), provideModelRuntimeInspectFile(edges)
	runtime, err := provideModelInvocationRuntime(edges, runner, temp, create, provideModelAssetWriteFile(edges), inspect, provideModelAssetReadFile(edges), provideModelAssetRemovePath(edges))
	if err != nil {
		return nil, err
	}
	now := provideModelNow(edges)
	inference, err := provideModelInference(scopes, assets, catalog, host, runtime, now)
	if err != nil {
		return nil, err
	}
	resolver, err := provideModelBackendArtifactResolver(edges, runner, provideModelAssetHTTP(edges))
	if err != nil {
		return nil, err
	}
	return provideModelsService(edges, scopes, assets, catalog, host, inference, provideModelHostLauncher(edges, source),
		provideModelHostHTTP(edges), provideModelHostClock(edges), runner, provideModelRuntimeHTTP(edges), inspect, temp, create,
		now, provideModelHostLogger(), provideModelHostMetrics(edges), evidence, resolver, provideModelAssetHostPlatform(edges))
}

func newModelAssetsFixture(edges serviceedges.Edges, scopes modelswire.RuntimeScopes) (modelswire.Assets, error) {
	coordination, err := provideModelAssetCoordination(edges)
	if err != nil {
		return nil, err
	}
	return provideModelAssets(scopes, provideModelAssetHostPlatform(edges), provideModelAssetHTTP(edges), provideModelAssetEndpoints(edges),
		provideModelAssetMakeDirectories(edges), provideModelAssetInspectPath(edges), provideModelAssetResolveHomeDirectory(edges),
		provideModelAssetWriteFile(edges), provideModelAssetRenamePath(edges), provideModelAssetRemovePath(edges),
		provideModelAssetReadFile(edges), provideModelAssetReadDirectory(edges), provideModelAssetCreateFile(edges), provideModelAssetOpenFile(edges),
		provideModelAssetResolveEnvironment(edges), provideModelAssetRevision(edges), coordination)
}

func newModelHostFixture(edges serviceedges.Edges, scopes modelswire.RuntimeScopes, assets modelswire.Assets,
	source modelRuntimeEvidenceSource, evidence modelswire.RuntimeEvidenceRecorder) (modelswire.RuntimeHost, error) {
	state := modelswire.NewSlotState()
	clock, logger, metrics := provideModelHostClock(edges), provideModelHostLogger(), provideModelHostMetrics(edges)
	coordinator := provideModelSlotCoordinator(state, scopes, clock, logger, metrics)
	leases, err := modelswire.NewHostLeases(clock, provideModelSlotFacts(state, scopes, assets), coordinator)
	if err != nil {
		return nil, err
	}
	compatibility, err := provideModelHostCompatibility(edges)
	if err != nil {
		return nil, err
	}
	symlinks := provideModelHostSymlinks()
	return provideModelRuntimeHost(scopes, assets, leases, state, provideModelHostLauncher(edges, source), provideModelHostHTTP(edges), clock,
		logger, metrics, provideModelAssetHostPlatform(edges), provideModelHostProtocol(edges, symlinks), compatibility, symlinks, evidence)
}

func TestModelsServiceIsConstructedOnceAndOpensRuntimeScopeOnSameRoot(t *testing.T) {
	t.Parallel()

	root, err := newModelsServiceFixture(serviceedges.Edges{})
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

	// Managed backend selection now accepts published CUDA archives on both
	// desktop platforms, so a detected GPU selects the CUDA accelerator
	// everywhere a CUDA-capable archive can be published.
	selectsCUDA := runtime.GOARCH == "amd64" &&
		(runtime.GOOS == "linux" || runtime.GOOS == "windows")
	if got := provideModelAssetHostPlatform(serviceedges.Edges{}); got.OperatingSystem != runtime.GOOS ||
		got.Architecture != runtime.GOARCH ||
		(got.Accelerator != "" && !(selectsCUDA && got.Accelerator == "cuda")) ||
		(got.Accelerator == "cuda" && !got.CUDAAvailable) {
		t.Fatalf("default model asset host platform = %#v, want current process platform and optional detected CUDA", got)
	}

	override := models.AssetHostPlatform{OperatingSystem: "customer-os", Architecture: "customer-arch", Accelerator: "cuda", CUDAAvailable: true}
	if got := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: override}); got != override {
		t.Fatalf("model asset host platform override = %#v, want %#v", got, override)
	}
	if got := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: models.AssetHostPlatform{Accelerator: "cuda"}}); got != (models.AssetHostPlatform{
		OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH, Accelerator: "cuda", CUDAAvailable: true,
	}) {
		t.Fatalf("model asset host platform with accelerator override = %#v, want process OS and architecture with CUDA", got)
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
		Configuration: modelswire.ResolvedHostConfiguration{
			Backend:      "localai-llamacpp",
			ModelPath:    "runtime/model.gguf",
			MMProjPath:   "runtime/mmproj.gguf",
			BackendFiles: []string{"runtime/backend.zip"},
		},
	})
	if err != nil {
		t.Fatalf("adapted process launcher: %v", err)
	}
	if gotSpec.Command != "model-host" || len(gotSpec.Args) != 1 || gotSpec.Args[0] != "serve" ||
		len(gotSpec.Env) != 1 || gotSpec.Env[0] != "MODEL=seal" || gotSpec.WorkDir != "runtime" ||
		gotSpec.HealthEndpoint != process.healthEndpoint || gotSpec.Backend != "localai-llamacpp" ||
		gotSpec.ModelPath != "runtime/model.gguf" || gotSpec.MMProjPath != "runtime/mmproj.gguf" || len(gotSpec.BackendFiles) != 1 ||
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
			_, err := newModelsServiceFixture(testCase.edges)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("provideModelsService() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestModelsCompositionSanitizesOptionalHostProcessDiagnosticAtWireBoundary(t *testing.T) {
	t.Parallel()

	private := "token=private endpoint=https://private.example.test/model prompt=secret"
	var gotSpec serviceedges.HostProcessStartSpec
	process := &modelEdgeManagedProcess{
		healthEndpoint: "http://model-host/health",
		diagnostic: serviceedges.HostProcessDiagnosticSnapshot{
			ExitClass: "NONZERO_EXIT", ExitCode: 17, ExitCodeKnown: true,
			Stdout: serviceedges.HostProcessStreamDiagnostic{
				Bytes: 99, SHA256: strings.Repeat("A", 64), Truncated: true,
			},
			CauseCode: "RPC_REJECTED", CauseMessage: private,
		},
		diagnosticReady: true,
	}
	launcher := adaptModelHostProcessLauncher(&modelEdgeProcessLauncher{process: process, gotSpec: &gotSpec})
	gotProcess, err := launcher.Start(context.Background(), modelswire.HostProcessStartSpec{
		Command:       "model-host",
		Configuration: modelswire.ResolvedHostConfiguration{Backend: "localai-llamacpp"},
	})
	if err != nil {
		t.Fatalf("adapted process launcher: %v", err)
	}
	source := requireWireDiagnosticSource(t, gotProcess)
	assertWireDiagnosticSnapshot(t, source, private)
	for _, code := range []string{"UNKNOWN", "PROJECTOR_LOAD_FAILED", "native-secret"} {
		assertWireDiagnosticCauseRejected(t, source, process, code, private)
	}
}

func requireWireDiagnosticSource(
	t *testing.T,
	process modelswire.HostManagedProcess,
) modelswire.HostManagedProcessDiagnosticSource {
	t.Helper()
	source, ok := process.(modelswire.HostManagedProcessDiagnosticSource)
	if !ok {
		t.Fatal("adapted process did not preserve optional diagnostic capability")
	}
	return source
}

func assertWireDiagnosticSnapshot(
	t *testing.T,
	source modelswire.HostManagedProcessDiagnosticSource,
	private string,
) {
	t.Helper()
	snapshot, ready := source.DiagnosticSnapshot()
	if !ready {
		t.Fatal("adapted process did not return ready diagnostic snapshot")
	}
	if snapshot.ExitClass != "NONZERO_EXIT" || snapshot.ExitCode != 17 ||
		!snapshot.ExitCodeKnown || snapshot.Stdout.SHA256 != strings.Repeat("a", 64) ||
		snapshot.Stdout.Bytes != 99 || !snapshot.Stdout.Truncated ||
		snapshot.CauseCode != "RPC_REJECTED" ||
		snapshot.CauseMessage != "backend RPC request rejected" ||
		snapshot.CauseMessageRedacted {
		t.Fatalf("wire diagnostic snapshot = %#v, want bounded allow-listed facts", snapshot)
	}
	if strings.Contains(snapshot.CauseMessage, private) {
		t.Fatalf("wire diagnostic snapshot leaked private cause: %#v", snapshot)
	}
}

func assertWireDiagnosticCauseRejected(
	t *testing.T,
	source modelswire.HostManagedProcessDiagnosticSource,
	process *modelEdgeManagedProcess,
	code, private string,
) {
	t.Helper()
	process.diagnostic.CauseCode = code
	process.diagnostic.CauseMessage = private
	got, ready := source.DiagnosticSnapshot()
	if !ready || got.CauseCode != "" || got.CauseMessage != "" {
		t.Fatalf("unsupported wire cause %q crossed boundary: %#v, ready=%t", code, got, ready)
	}
}

func TestManagedProcessCauseReducerUsesOnlySafeCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		marker  string
		code    string
		message string
		redact  bool
	}{
		{name: "model", marker: "model load failed", code: "MODEL_LOAD_FAILED", message: "model load failed"},
		{name: "projector", marker: "mmproj projector load error", code: "MODEL_LOAD_FAILED", message: "model load failed"},
		{name: "protocol", marker: "protocol incompatible", code: "PROTOCOL_INCOMPATIBLE", message: "backend protocol incompatible"},
		{name: "endpoint", marker: "endpoint bind: address already in use", code: "ENDPOINT_BIND_FAILED", message: "backend endpoint bind failed"},
		{name: "rpc", marker: "rpc rejected invalid request", code: "RPC_REJECTED", message: "backend RPC request rejected"},
		{name: "timeout", marker: "request timed out", code: "TIMEOUT", message: "backend operation timed out"},
		{name: "cancel", marker: "operation cancelled", code: "CANCELLED", message: "backend operation cancelled"},
		{name: "unknown", marker: "native token=https://private.example.test/model", code: "PROCESS_EXITED", message: "managed backend process exited", redact: true},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			code, message, redacted := reduceManagedProcessCause(
				errors.New("controlled exit"), []byte(testCase.marker), nil,
			)
			if code != testCase.code || message != testCase.message || redacted != testCase.redact {
				t.Fatalf("reduced cause = (%q, %q, %t), want (%q, %q, %t)", code, message, redacted, testCase.code, testCase.message, testCase.redact)
			}
			if strings.Contains(message, "private") || strings.Contains(message, "token") || strings.Contains(message, "https://") {
				t.Fatalf("reduced cause leaked native output: %q", message)
			}
		})
	}
}

func TestModelsCompositionRejectsMissingAssetStagingCoordination(t *testing.T) {
	t.Parallel()

	_, err := newModelsServiceFixture(serviceedges.Edges{
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
		Configuration: modelswire.ResolvedHostConfiguration{
			ProtocolVersion: "model-host.v1", Backend: "localai-vibevoice", ModelName: "tts",
			Revision:  "revision-1",
			Platform:  models.AssetHostPlatform{OperatingSystem: "test-os", Architecture: "test-arch"},
			ModelPath: "runtime/model.gguf", MMProjPath: "runtime/mmproj.gguf",
			ModelFiles: []string{"runtime/model.gguf", "runtime/tokenizer.gguf"},
		},
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
	if protocol.endpoint != "grpc://model-host" || protocol.request.ProtocolVersion != request.Configuration.ProtocolVersion ||
		protocol.request.Backend != request.Configuration.Backend || protocol.request.ModelName != request.Configuration.ModelName ||
		protocol.request.Revision != request.Configuration.Revision || protocol.request.Platform != request.Configuration.Platform ||
		protocol.request.ModelPath != request.Configuration.ModelPath || protocol.request.MMProjPath != request.Configuration.MMProjPath ||
		!equalStringSlices(protocol.request.ModelFiles, request.Configuration.ModelFiles) {
		t.Fatalf("edge protocol request = %#v at %q, want exact projection", protocol.request, protocol.endpoint)
	}
	if result != (modelswire.HostProtocolNegotiationResult{
		ProtocolVersion: "model-host.v1", Backend: request.Configuration.Backend, Ready: true,
	}) {
		t.Fatalf("protocol result = %#v, want ready pinned result", result)
	}
}

func assertAdaptedCompatibility(t *testing.T, request modelswire.HostProtocolNegotiationRequest) {
	t.Helper()
	compatibility := &modelEdgeCompatibilityChecker{}
	if err := adaptModelHostCompatibilityChecker(compatibility).Check(context.Background(), modelswire.HostCompatibilityRequest{
		Configuration: request.Configuration,
	}); err != nil {
		t.Fatalf("compatibility check: %v", err)
	}
	if compatibility.request.Backend != request.Configuration.Backend || compatibility.request.ModelName != request.Configuration.ModelName ||
		compatibility.request.Revision != request.Configuration.Revision || compatibility.request.Platform != request.Configuration.Platform {
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
	if connection.request.Backend != request.Configuration.Backend || connection.request.ModelPath != request.Configuration.ModelPath ||
		connection.request.MMProjPath != request.Configuration.MMProjPath ||
		!equalStringSlices(connection.request.ModelFiles, request.Configuration.ModelFiles) || !connection.closed {
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
	healthEndpoint  string
	stopped         bool
	diagnostic      serviceedges.HostProcessDiagnosticSnapshot
	diagnosticReady bool
}

func (process *modelEdgeManagedProcess) HealthEndpoint() string { return process.healthEndpoint }
func (*modelEdgeManagedProcess) Wait() error                    { return nil }
func (process *modelEdgeManagedProcess) Stop(context.Context) error {
	process.stopped = true
	return nil
}

func (process *modelEdgeManagedProcess) DiagnosticSnapshot() (serviceedges.HostProcessDiagnosticSnapshot, bool) {
	return process.diagnostic, process.diagnosticReady
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

type modelsCLICompositionRootStub struct {
	modelservice.Service
	openRuntime  func(context.Context, modelservice.OpenRuntimeScopeRequest) (modelservice.OpenRuntimeScopeResult, error)
	closeRuntime func(context.Context, modelservice.CloseRuntimeScopeRequest) (modelservice.CloseRuntimeScopeResult, error)
}

func (stub modelsCLICompositionRootStub) OpenRuntimeScope(
	ctx context.Context,
	request modelservice.OpenRuntimeScopeRequest,
) (modelservice.OpenRuntimeScopeResult, error) {
	if stub.openRuntime == nil {
		return modelservice.OpenRuntimeScopeResult{}, errors.New("unexpected standalone Models scope open")
	}
	return stub.openRuntime(ctx, request)
}

func (stub modelsCLICompositionRootStub) CloseRuntimeScope(
	ctx context.Context,
	request modelservice.CloseRuntimeScopeRequest,
) (modelservice.CloseRuntimeScopeResult, error) {
	if stub.closeRuntime == nil {
		return modelservice.CloseRuntimeScopeResult{}, errors.New("unexpected standalone Models scope close")
	}
	return stub.closeRuntime(ctx, request)
}

type modelsCLICompositionScopeSourceStub struct {
	factorysessionwire.InvocationOperation
	request modelservice.PresentationScopeRequest
	scope   modelservice.PresentationScope
	err     error
	calls   int
}

func (stub *modelsCLICompositionScopeSourceStub) OpenModelsCatalogScope(
	_ context.Context,
) (modelservice.PresentationScope, error) {
	return stub.scope, nil
}

func (stub *modelsCLICompositionScopeSourceStub) OpenModelsPresentationScope(
	_ context.Context,
	request modelservice.PresentationScopeRequest,
) (modelservice.PresentationScope, error) {
	stub.calls++
	stub.request = request
	return stub.scope, stub.err
}

func TestModelsInvokeCompositionMapsCacheSelectionToPresentationScope(t *testing.T) {
	t.Parallel()

	scope, err := (modelservice.RuntimeScopeRef{}).Parse("wire:models:invoke")
	if err != nil {
		t.Fatalf("parse Models runtime scope: %v", err)
	}
	logger := zap.NewNop()
	source := &modelsCLICompositionScopeSourceStub{
		scope: modelservice.PresentationScope{Scope: scope},
	}
	composition, err := provideModelsCLIComposition(modelsCLICompositionRootStub{}, source, nil)
	if err != nil {
		t.Fatalf("provideModelsCLIComposition() error = %v", err)
	}

	config := modelscli.InvokeConfig{
		FactoryDir:       "factory",
		WorkingDirectory: "working",
		HomeDir:          "home",
		OperatorDefaults: operatorsettings.ResolvedDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-test",
		},
		Logger:  logger,
		Verbose: true,
	}
	cacheAware, ok := composition.(modelscli.CompositionInvokeScopeWithModelCacheOpener)
	if !ok {
		t.Fatal("Models CLI composition does not expose optional cache-aware scope opener")
	}
	opened, err := cacheAware.CompositionOpenInvokeScopeWithModelCache(context.Background(), modelscli.InvokeScopeRequest{
		Config:        config,
		ModelCacheDir: "selected-model-cache",
	})
	if err != nil {
		t.Fatalf("CompositionOpenInvokeScope() error = %v", err)
	}
	if opened.Scope != scope {
		t.Fatalf("opened scope = %q, want %q", opened.Scope, scope)
	}
	want := modelservice.PresentationScopeRequest{
		FactoryDir:       config.FactoryDir,
		WorkingDirectory: config.WorkingDirectory,
		HomeDir:          config.HomeDir,
		OperatorDefaults: modelservice.PresentationOperatorDefaults{
			WorkerModelProvider: config.OperatorDefaults.WorkerModelProvider,
			WorkerModel:         config.OperatorDefaults.WorkerModel,
		},
		Logger:        config.Logger,
		Verbose:       config.Verbose,
		ModelCacheDir: "selected-model-cache",
	}
	if !reflect.DeepEqual(source.request, want) {
		t.Fatalf("presentation scope request = %#v, want %#v", source.request, want)
	}
}

func TestModelsInvokeStandaloneScopeProjectsOperatorModelOverlay(t *testing.T) {
	t.Parallel()

	scope, err := (modelservice.RuntimeScopeRef{}).Parse("wire:models:standalone-overlay")
	if err != nil {
		t.Fatalf("parse standalone overlay scope: %v", err)
	}
	home := t.TempDir()
	fixtureSource := "hf://fixture/models/llm.gguf@0000000000000000000000000000000000000000"
	var openRequest modelservice.OpenRuntimeScopeRequest
	root := modelsCLICompositionRootStub{
		openRuntime: func(_ context.Context, request modelservice.OpenRuntimeScopeRequest) (modelservice.OpenRuntimeScopeResult, error) {
			openRequest = request
			return modelservice.OpenRuntimeScopeResult{Scope: scope}, nil
		},
		closeRuntime: func(_ context.Context, request modelservice.CloseRuntimeScopeRequest) (modelservice.CloseRuntimeScopeResult, error) {
			return modelservice.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
		},
	}
	loader := func(path string) (operatorsettings.Config, error) {
		if path != operatorsettings.DefaultConfigPath(home) {
			t.Fatalf("operator config path = %q, want %q", path, operatorsettings.DefaultConfigPath(home))
		}
		return operatorsettings.Config{Models: map[string]operatorsettings.ModelConfig{
			modelservice.BuiltInModelNameLLM: {Source: &fixtureSource},
		}}, nil
	}
	composition, err := provideModelsCLIComposition(
		root,
		&modelsCLICompositionScopeSourceStub{err: factorydefinitions.ErrFactoryLayoutNotFound},
		loader,
	)
	if err != nil {
		t.Fatalf("provideModelsCLIComposition() error = %v", err)
	}
	opener, ok := composition.(modelscli.CompositionInvokeScopeWithModelCacheOpener)
	if !ok {
		t.Fatal("Models CLI composition does not expose cache-aware invoke scope opener")
	}
	opened, err := opener.CompositionOpenInvokeScopeWithModelCache(context.Background(), modelscli.InvokeScopeRequest{
		Config: modelscli.InvokeConfig{HomeDir: home},
	})
	if err != nil {
		t.Fatalf("CompositionOpenInvokeScopeWithModelCache() error = %v", err)
	}
	if overlay := openRequest.Config.OperatorModels[modelservice.BuiltInModelNameLLM]; overlay.Source == nil || *overlay.Source != fixtureSource {
		t.Fatalf("standalone Models operator overlay = %#v, want fixture source %q", overlay, fixtureSource)
	}
	if err := opened.Close(context.Background()); err != nil {
		t.Fatalf("close standalone overlay scope: %v", err)
	}
}

func TestModelsCatalogWithoutCacheIncludesCustomOperatorModel(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	const source = "file:///models/index-tts2_5-orig.gguf"
	configJSON := `{"models":{"index-tts2.5":{"source":"` + source + `","backend":"localai-audio-cpp","loadPolicy":"ON_DEMAND","operations":["TTS"]}}}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	scope, err := (modelservice.RuntimeScopeRef{}).Parse("wire:models:custom-catalog")
	if err != nil {
		t.Fatal(err)
	}
	var openRequest modelservice.OpenRuntimeScopeRequest
	root := modelsCLICompositionRootStub{
		openRuntime: func(_ context.Context, request modelservice.OpenRuntimeScopeRequest) (modelservice.OpenRuntimeScopeResult, error) {
			openRequest = request
			return modelservice.OpenRuntimeScopeResult{Scope: scope}, nil
		},
		closeRuntime: func(_ context.Context, request modelservice.CloseRuntimeScopeRequest) (modelservice.CloseRuntimeScopeResult, error) {
			return modelservice.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
		},
	}
	sourceScope := &modelsCLICompositionScopeSourceStub{err: factorydefinitions.ErrFactoryLayoutNotFound}
	loader := func(path string) (operatorsettings.Config, error) {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return operatorsettings.Config{}, readErr
		}
		return globalconfig.Decode(data)
	}
	composition, err := provideModelsCLIComposition(root, sourceScope, loader)
	if err != nil {
		t.Fatal(err)
	}
	ctx := startupcli.WithHomeDirectory(context.Background(), home)
	ctx = startupcli.WithWorkingDirectory(ctx, t.TempDir())
	opened, err := composition.CompositionOpenCatalogScope(ctx)
	if err != nil {
		t.Fatalf("open catalog scope: %v", err)
	}
	if sourceScope.calls != 1 {
		t.Fatalf("presentation scope probes = %d, want one", sourceScope.calls)
	}
	assertCustomCatalogOverlay(t, openRequest.Config.OperatorModels, source)
	if err := opened.Close(context.Background()); err != nil {
		t.Fatalf("close catalog scope: %v", err)
	}
}

func assertCustomCatalogOverlay(t *testing.T, overlays map[string]modelservice.ModelOverlay, source string) {
	t.Helper()
	overlay, ok := overlays["index-tts2.5"]
	if !ok {
		t.Fatal("custom catalog overlay is missing")
	}
	if overlay.Source == nil || *overlay.Source != source {
		t.Fatalf("custom catalog source = %v, want %q", overlay.Source, source)
	}
	if overlay.Backend == nil || *overlay.Backend != "localai-audio-cpp" {
		t.Fatalf("custom catalog backend = %v, want localai-audio-cpp", overlay.Backend)
	}
	if overlay.LoadPolicy == nil || *overlay.LoadPolicy != modelservice.LoadPolicyOnDemand {
		t.Fatalf("custom catalog load policy = %v, want ON_DEMAND", overlay.LoadPolicy)
	}
	if !reflect.DeepEqual(overlay.Operations, []string{"TTS"}) {
		t.Fatalf("custom catalog operations = %v, want [TTS]", overlay.Operations)
	}
}

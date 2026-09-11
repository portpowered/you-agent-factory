package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestLocalAIDirectCLICharacterizesResolvedConfigurationFacts proves the
// direct Models CLI path carries one operator-selected LocalAI model source
// through immutable revision resolution, a populated offline cache, pinned
// host startup, protocol negotiation, and semantic output. The second lane
// reuses the same composition and proves a controlled backend protocol error
// remains typed and cannot publish partial output.
func TestLocalAIDirectCLICharacterizesResolvedConfigurationFacts(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		protocolError error
		wantOutput    string
		wantFailure   bool
	}{
		{
			name:       "semantic output",
			wantOutput: "direct LocalAI characterization response",
		},
		{
			name:          "typed backend protocol failure",
			protocolError: errors.New("controlled LocalAI backend protocol failure"),
			wantFailure:   true,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			scenario := newLocalAIDirectConfigurationFactsScenario(t, test.protocolError, test.wantOutput)
			var stdout, stderr bytes.Buffer
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "--json", "models", "invoke", models.BuiltInModelNameLLM,
				"--offline", "--operation", models.OperationOMNI,
				"--input", "prompt=direct LocalAI characterization prompt",
			})
			inputs.Input.Env = scenario.environment
			inputs.Input.WorkingDirectory = scenario.factoryDir
			inputs.Input.Stdout = &stdout
			inputs.Input.Stderr = &stderr

			err := scenario.process.Execute(inputs.Input)
			if test.wantFailure {
				assertLocalAIDirectTypedFailure(t, err, stdout.String())
			} else {
				assertLocalAIDirectSemanticOutput(t, err, stdout.Bytes(), stderr.String(), test.wantOutput)
			}

			assertLocalAIDirectConfigurationFacts(t, scenario)
			if err := scenario.process.Close(context.Background()); err != nil {
				t.Fatalf("close direct LocalAI characterization process: %v", err)
			}
			assertLocalAIDirectHostReleased(t, scenario.launcher)

			t.Logf(
				"direct LocalAI configuration facts: source=%q revision=%q cache=%q modelPath=%q protocol=%q backend=%q platform=%s/%s offline=true networkCalls=%d hostStarts=%d hostStops=%d hostWaits=%d",
				scenario.source, scenario.revision, scenario.modelCacheRoot,
				scenario.launcher.ModelPath(), scenario.negotiator.ProtocolVersion(),
				scenario.backend, scenario.platform.OperatingSystem, scenario.platform.Architecture,
				scenario.assetNetwork.Calls(), scenario.launcher.Starts(), scenario.launcher.Stops(), scenario.launcher.Waits(),
			)
		})
	}
}

type localAIDirectConfigurationFactsScenario struct {
	process        support.ApplicationProcess
	factoryDir     string
	environment    []string
	home           string
	modelCacheRoot string
	source         string
	revision       string
	backend        string
	platform       models.AssetHostPlatform
	assetNetwork   *rejectingModelAssetHTTP
	resolver       *localAIRevisionRecorder
	backendSelect  *localAIBackendSelectionRecorder
	launcher       *localAIHostLauncher
	negotiator     *localAIHostProtocolRecorder
	compatibility  *localAIHostCompatibilityRecorder
	invocation     *localAIInvocationProtocolRecorder
}

func newLocalAIDirectConfigurationFactsScenario(
	t *testing.T,
	protocolError error,
	response string,
) localAIDirectConfigurationFactsScenario {
	t.Helper()

	const (
		operatorSource = "hf://characterization/localai-llm.gguf"
		resolvedCommit = "0123456789abcdef0123456789abcdef01234567"
		backend        = "localai-llamacpp"
	)
	platform := models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}
	home := functionalTempDir(t)
	writeLocalAIModelOverlay(t, home, operatorSource)
	writeGenericBuiltinModelCache(t, home, operatorSource+"@"+resolvedCommit)
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, backend, selection, []byte("localai-llamacpp/linux-amd64"))

	assetNetwork := &rejectingModelAssetHTTP{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	resolver := &localAIRevisionRecorder{revision: resolvedCommit}
	backendSelect := &localAIBackendSelectionRecorder{selection: selection}
	launcher := &localAIHostLauncher{endpoint: "http://localai-characterization.invalid"}
	negotiator := &localAIHostProtocolRecorder{}
	compatibility := &localAIHostCompatibilityRecorder{}
	invocation := &localAIInvocationProtocolRecorder{response: response, failure: protocolError}
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:            assetNetwork,
		ModelAssetMakeDirectories:       assetFiles.MkdirAll,
		ModelAssetInspectPath:           assetFiles.Stat,
		ModelAssetResolveHomeDirectory:  assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:    func(string) string { return "" },
		ModelAssetWriteFile:             assetFiles.WriteFile,
		ModelAssetRenamePath:            assetFiles.Rename,
		ModelAssetRemovePath:            assetFiles.Remove,
		ModelAssetReadFile:              assetFiles.ReadFile,
		ModelAssetReadDirectory:         assetFiles.ReadDir,
		ModelAssetCreateFile:            assetFiles.Create,
		ModelAssetOpenFile:              assetFiles.Open,
		ModelHostProcessLauncher:        launcher,
		ModelHostProtocolNegotiator:     negotiator,
		ModelHostCompatibilityChecker:   compatibility,
		ModelAssetHostPlatform:          platform,
		ModelResolveBackendArtifact:     backendSelect.Resolve,
		ModelResolveHuggingFaceRevision: resolver.Resolve,
		ModelHostHTTPClient:             assetNetwork,
		ModelRuntimeHTTPClient:          assetNetwork,
		ModelInvocationProtocolClient:   invocation,
	})

	return localAIDirectConfigurationFactsScenario{
		process: process, factoryDir: factoryDir,
		environment: cleanModelsEnvironment(home), home: home,
		modelCacheRoot: filepath.Join(home, ".agent-factory", "models"),
		source:         operatorSource, revision: resolvedCommit,
		backend: backend, platform: platform,
		assetNetwork: assetNetwork, resolver: resolver,
		backendSelect: backendSelect, launcher: launcher,
		negotiator: negotiator, compatibility: compatibility,
		invocation: invocation,
	}
}

func writeLocalAIModelOverlay(t *testing.T, home, source string) {
	t.Helper()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create LocalAI operator config directory: %v", err)
	}
	config, err := json.Marshal(map[string]any{
		"models": map[string]any{
			models.BuiltInModelNameLLM: map[string]any{"source": source},
		},
	})
	if err != nil {
		t.Fatalf("marshal LocalAI operator config: %v", err)
	}
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("write LocalAI operator config: %v", err)
	}
}

func assertLocalAIDirectSemanticOutput(
	t *testing.T,
	err error,
	stdout []byte,
	stderr, want string,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("direct LocalAI semantic invocation: %v\nstderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("direct LocalAI semantic invocation stderr = %q, want empty", stderr)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(stdout, &response); err != nil {
		t.Fatalf("decode direct LocalAI semantic response: %v\n%s", err, stdout)
	}
	if len(response.Outputs) != 1 || response.Outputs[0].Name != "text" ||
		response.Outputs[0].Content == nil || *response.Outputs[0].Content != want {
		t.Fatalf("direct LocalAI semantic response = %#v, want one text output %q", response, want)
	}
}

func assertLocalAIDirectTypedFailure(t *testing.T, err error, stdout string) {
	t.Helper()
	if err == nil {
		t.Fatal("direct LocalAI protocol failure returned nil error")
	}
	if stdout != "" {
		t.Fatalf("direct LocalAI protocol failure stdout = %q, want no partial success", stdout)
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure == nil {
		t.Fatalf("direct LocalAI protocol error = %v (%T), want *models.InvocationFailure", err, err)
	}
	if failure.Class != models.InvocationFailureClassBackendProtocol ||
		failure.Operation != models.OperationOMNI {
		t.Fatalf("direct LocalAI typed failure = %#v, want BACKEND_PROTOCOL/OMNI", failure)
	}
}

func assertLocalAIDirectConfigurationFacts(
	t *testing.T,
	scenario localAIDirectConfigurationFactsScenario,
) {
	t.Helper()

	sources := scenario.resolver.Sources()
	if len(sources) == 0 {
		t.Fatal("LocalAI source resolver calls = 0, want operator-selected source observation")
	}
	for _, source := range sources {
		if source != scenario.source {
			t.Fatalf("LocalAI resolved source = %q, want operator-selected %q", source, scenario.source)
		}
	}
	if scenario.assetNetwork.Calls() != 0 {
		t.Fatalf("LocalAI offline asset network calls = %d, want zero", scenario.assetNetwork.Calls())
	}

	backendRequests := scenario.backendSelect.Requests()
	if len(backendRequests) == 0 {
		t.Fatal("LocalAI backend selection calls = 0, want selected backend observation")
	}
	for _, request := range backendRequests {
		if request.Backend != scenario.backend || request.Platform != scenario.platform ||
			request.ProtocolVersion != "localai-backend-v1" {
			t.Fatalf("LocalAI backend selection request = %#v, want backend/protocol/platform facts", request)
		}
	}

	spec, ok := scenario.launcher.LastSpec()
	if !ok {
		t.Fatal("LocalAI host start spec was not observed")
	}
	if spec.Backend != scenario.backend || spec.ModelPath == "" || len(spec.ModelFiles) != 1 ||
		spec.ModelFiles[0] != spec.ModelPath || len(spec.BackendFiles) != 1 {
		t.Fatalf("LocalAI host start spec = %#v, want backend/model/backend file facts", spec)
	}
	if !pathWithinLocalAIRoot(scenario.modelCacheRoot, spec.ModelPath) ||
		!strings.HasSuffix(filepath.ToSlash(spec.ModelPath), "/localai-llm.gguf") {
		t.Fatalf("LocalAI model path = %q, want selected managed cache artifact", spec.ModelPath)
	}
	if !pathWithinLocalAIRoot(filepath.Join(scenario.modelCacheRoot, "backend-artifacts", ".you-content-addressed", "backend"), spec.BackendFiles[0]) {
		t.Fatalf("LocalAI backend file = %q, want selected backend cache artifact", spec.BackendFiles[0])
	}

	compatibilityRequests := scenario.compatibility.Requests()
	if len(compatibilityRequests) == 0 {
		t.Fatal("LocalAI compatibility checks = 0, want resolved host fact observation")
	}
	for _, request := range compatibilityRequests {
		if request.Backend != scenario.backend || request.ModelName != models.BuiltInModelNameLLM ||
			request.Revision != scenario.revision || request.Platform != scenario.platform {
			t.Fatalf("LocalAI compatibility request = %#v, want backend/model/revision/platform facts", request)
		}
	}

	negotiationRequests := scenario.negotiator.Requests()
	if len(negotiationRequests) == 0 {
		t.Fatal("LocalAI protocol negotiation calls = 0")
	}
	for _, request := range negotiationRequests {
		if request.ProtocolVersion != "localai-backend-v1" || request.Backend != scenario.backend ||
			request.ModelName != models.BuiltInModelNameLLM || request.Revision != scenario.revision ||
			request.Platform != scenario.platform || request.ModelPath != spec.ModelPath ||
			len(request.ModelFiles) != 1 || request.ModelFiles[0] != spec.ModelPath {
			t.Fatalf("LocalAI protocol negotiation request = %#v, want resolved protocol facts", request)
		}
	}
	if scenario.invocation.Calls() != 1 {
		t.Fatalf("LocalAI invocation protocol calls = %d, want one direct attempt", scenario.invocation.Calls())
	}
}

func assertLocalAIDirectHostReleased(t *testing.T, launcher *localAIHostLauncher) {
	t.Helper()
	waitContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := launcher.awaitWait(waitContext); err != nil {
		t.Fatalf("wait for LocalAI host Wait completion: %v", err)
	}
	if launcher.Active() || launcher.Starts() != launcher.Stops() || launcher.Stops() != launcher.Waits() {
		t.Fatalf("LocalAI host lifecycle = starts:%d stops:%d waits:%d active:%t, want fully released", launcher.Starts(), launcher.Stops(), launcher.Waits(), launcher.Active())
	}
}

func pathWithinLocalAIRoot(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

type localAIRevisionRecorder struct {
	mu       sync.Mutex
	revision string
	sources  []string
}

func (recorder *localAIRevisionRecorder) Resolve(_ context.Context, source string) (string, error) {
	recorder.mu.Lock()
	recorder.sources = append(recorder.sources, source)
	revision := recorder.revision
	recorder.mu.Unlock()
	return revision, nil
}

func (recorder *localAIRevisionRecorder) Sources() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]string(nil), recorder.sources...)
}

type localAIBackendSelectionRecorder struct {
	mu        sync.Mutex
	selection serviceedges.ModelBackendArtifactSelection
	requests  []serviceedges.ModelBackendArtifactSelectionRequest
}

func (recorder *localAIBackendSelectionRecorder) Resolve(
	_ context.Context,
	request serviceedges.ModelBackendArtifactSelectionRequest,
) (serviceedges.ModelBackendArtifactSelection, error) {
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, request)
	selection := recorder.selection
	recorder.mu.Unlock()
	return selection, nil
}

func (recorder *localAIBackendSelectionRecorder) Requests() []serviceedges.ModelBackendArtifactSelectionRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]serviceedges.ModelBackendArtifactSelectionRequest(nil), recorder.requests...)
}

type localAIHostLauncher struct {
	mu       sync.Mutex
	endpoint string
	specs    []serviceedges.HostProcessStartSpec
	active   int
	starts   int
	stops    int
	waits    int
	waitDone <-chan struct{}
}

func (launcher *localAIHostLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	waitDone := make(chan struct{})
	launcher.mu.Lock()
	launcher.specs = append(launcher.specs, cloneLocalAIHostSpec(spec))
	launcher.starts++
	launcher.active++
	launcher.waitDone = waitDone
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &localAIHostProcess{
		endpoint: endpoint,
		stopped:  make(chan struct{}),
		waitDone: waitDone,
		launcher: launcher,
	}, nil
}

func (launcher *localAIHostLauncher) recordStop() {
	launcher.mu.Lock()
	launcher.stops++
	launcher.active--
	launcher.mu.Unlock()
}

func (launcher *localAIHostLauncher) recordWait() {
	launcher.mu.Lock()
	launcher.waits++
	launcher.mu.Unlock()
}

func (launcher *localAIHostLauncher) awaitWait(ctx context.Context) error {
	launcher.mu.Lock()
	waitDone := launcher.waitDone
	launcher.mu.Unlock()
	if waitDone == nil {
		return errors.New("LocalAI host was not started")
	}
	select {
	case <-waitDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (launcher *localAIHostLauncher) LastSpec() (serviceedges.HostProcessStartSpec, bool) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if len(launcher.specs) == 0 {
		return serviceedges.HostProcessStartSpec{}, false
	}
	return cloneLocalAIHostSpec(launcher.specs[len(launcher.specs)-1]), true
}

func (launcher *localAIHostLauncher) Active() bool {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.active != 0
}

func (launcher *localAIHostLauncher) Starts() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}

func (launcher *localAIHostLauncher) Stops() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.stops
}

func (launcher *localAIHostLauncher) Waits() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.waits
}

func (launcher *localAIHostLauncher) ModelPath() string {
	spec, _ := launcher.LastSpec()
	return spec.ModelPath
}

type localAIHostProcess struct {
	endpoint string
	stopped  chan struct{}
	waitDone chan struct{}
	launcher *localAIHostLauncher
	once     sync.Once
	waitOnce sync.Once
}

func (process *localAIHostProcess) HealthEndpoint() string { return process.endpoint }

func (process *localAIHostProcess) Wait() error {
	<-process.stopped
	process.waitOnce.Do(func() {
		process.launcher.recordWait()
		close(process.waitDone)
	})
	return nil
}

func (process *localAIHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		close(process.stopped)
		process.launcher.recordStop()
	})
	return nil
}

func cloneLocalAIHostSpec(spec serviceedges.HostProcessStartSpec) serviceedges.HostProcessStartSpec {
	spec.Args = append([]string(nil), spec.Args...)
	spec.Env = append([]string(nil), spec.Env...)
	spec.ModelFiles = append([]string(nil), spec.ModelFiles...)
	spec.BackendFiles = append([]string(nil), spec.BackendFiles...)
	return spec
}

type localAIHostProtocolRecorder struct {
	mu       sync.Mutex
	requests []serviceedges.ModelHostProtocolNegotiationRequest
}

func (recorder *localAIHostProtocolRecorder) Negotiate(
	_ context.Context,
	_ string,
	request serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, cloneLocalAIHostProtocolRequest(request))
	recorder.mu.Unlock()
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: request.ProtocolVersion,
		Backend:         request.Backend,
		Ready:           true,
	}, nil
}

func (recorder *localAIHostProtocolRecorder) Requests() []serviceedges.ModelHostProtocolNegotiationRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	requests := make([]serviceedges.ModelHostProtocolNegotiationRequest, len(recorder.requests))
	for index, request := range recorder.requests {
		requests[index] = cloneLocalAIHostProtocolRequest(request)
	}
	return requests
}

func (recorder *localAIHostProtocolRecorder) ProtocolVersion() string {
	requests := recorder.Requests()
	if len(requests) == 0 {
		return ""
	}
	return requests[len(requests)-1].ProtocolVersion
}

func cloneLocalAIHostProtocolRequest(request serviceedges.ModelHostProtocolNegotiationRequest) serviceedges.ModelHostProtocolNegotiationRequest {
	request.ModelFiles = append([]string(nil), request.ModelFiles...)
	return request
}

type localAIHostCompatibilityRecorder struct {
	mu       sync.Mutex
	requests []serviceedges.ModelHostCompatibilityRequest
}

func (recorder *localAIHostCompatibilityRecorder) Check(
	_ context.Context,
	request serviceedges.ModelHostCompatibilityRequest,
) error {
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, request)
	recorder.mu.Unlock()
	return nil
}

func (recorder *localAIHostCompatibilityRecorder) Requests() []serviceedges.ModelHostCompatibilityRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]serviceedges.ModelHostCompatibilityRequest(nil), recorder.requests...)
}

type localAIInvocationProtocolRecorder struct {
	mu       sync.Mutex
	response string
	failure  error
	request  models.InvocationProtocolRequest
	calls    int
}

func (recorder *localAIInvocationProtocolRecorder) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	recorder.mu.Lock()
	recorder.request = cloneLocalAIInvocationProtocolRequest(request)
	recorder.calls++
	response, failure := recorder.response, recorder.failure
	recorder.mu.Unlock()
	if failure != nil {
		return models.InvocationProtocolResponse{}, failure
	}
	return models.InvocationProtocolResponse{Text: response}, nil
}

func (recorder *localAIInvocationProtocolRecorder) Calls() int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.calls
}

func cloneLocalAIInvocationProtocolRequest(request models.InvocationProtocolRequest) models.InvocationProtocolRequest {
	request.Inputs = append([]models.InvocationProtocolInput(nil), request.Inputs...)
	if request.Parameters != nil {
		request.Parameters = make([]models.OperationParameter, len(request.Parameters))
		for index, parameter := range request.Parameters {
			request.Parameters[index] = parameter.Clone()
		}
	}
	return request
}

package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type genericCLIHostLauncher interface {
	Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
		HealthEndpoint() string
		Wait() error
		Stop(context.Context) error
	}, error)
}

type genericCLIProtocolNegotiator interface {
	Negotiate(context.Context, string, serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error)
}

type genericCLICompatibilityChecker interface {
	Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error
}

type genericCLIOutputFailureEffects struct {
	failedTarget string
	failed       atomic.Bool
}

func (effects *genericCLIOutputFailureEffects) CreateTemp(dir, pattern string) (interface {
	io.Writer
	io.Closer
	Name() string
}, error) {
	return os.CreateTemp(dir, pattern)
}

func (*genericCLIOutputFailureEffects) Inspect(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (*genericCLIOutputFailureEffects) Remove(path string) error {
	return os.Remove(path)
}

func (effects *genericCLIOutputFailureEffects) Rename(oldPath, newPath string) error {
	if newPath == effects.failedTarget && effects.failed.CompareAndSwap(false, true) {
		return fmt.Errorf("injected mapped publication failure for %s", newPath)
	}
	return os.Rename(oldPath, newPath)
}

// TestModelsGenericCLIProcessPublishesSingleOutputToStdoutOnly proves generic CLI inference emits exactly one stdout result.
func TestModelsGenericCLIProcessPublishesSingleOutputToStdoutOnly(t *testing.T) {
	t.Parallel()

	process, directory, environment := buildGenericCLIProcess(t, singleOutputModelFactoryConfig, nil, nil, nil, nil)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "stdout payload",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = directory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(single generic output) error = %v", err)
	}
	if inputs.Stdout() != "stdout payload" {
		t.Fatalf("single generic stdout = %q, want canonical payload without framing", inputs.Stdout())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("single generic stderr = %q, want diagnostics kept off stdout and empty for quiet success", inputs.Stderr())
	}
	closeRootProcess(t, process, "close single-output root process")
}

// TestModelsGenericCLIProcessRollsBackMappedOutputsThroughEdges proves failed generic inference rolls back mapped outputs through injected edges.
func TestModelsGenericCLIProcessRollsBackMappedOutputsThroughEdges(t *testing.T) {
	t.Parallel()

	outputDirectory := t.TempDir()
	textPath := filepath.Join(outputDirectory, "text.out")
	usagePath := filepath.Join(outputDirectory, "usage.out")
	if err := os.WriteFile(textPath, []byte("old text"), 0o644); err != nil {
		t.Fatalf("seed text output: %v", err)
	}
	if err := os.WriteFile(usagePath, []byte("old usage"), 0o644); err != nil {
		t.Fatalf("seed usage output: %v", err)
	}
	effects := &genericCLIOutputFailureEffects{failedTarget: usagePath}
	process, directory, environment := buildGenericCLIProcess(t, multiOutputModelFactoryConfig, effects, nil, nil, nil)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "new outputs",
		"--output-map", "text=" + textPath, "--output-map", "usage=" + usagePath,
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = directory
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "injected mapped publication failure") {
		t.Fatalf("Process.Execute(mapped publication) error = %v, want injected second-publication failure", err)
	}
	if !effects.failed.Load() {
		t.Fatal("mapped output failure edge was not exercised")
	}
	assertFunctionalFile(t, textPath, "old text")
	assertFunctionalFile(t, usagePath, "old usage")
	entries, err := os.ReadDir(outputDirectory)
	if err != nil {
		t.Fatalf("read mapped output directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".you-model-output-") {
			t.Fatalf("partial mapped output artifact %q remains after rollback", entry.Name())
		}
	}
	closeRootProcess(t, process, "close rollback root process")
}

// TestModelsGenericCLIProcessCancellationStopsReadinessAndPublishesNothing proves cancellation stops readiness without publishing output.
func TestModelsGenericCLIProcessCancellationStopsReadinessAndPublishesNothing(t *testing.T) {
	t.Parallel()

	protocol := &blockingGenericCLIProtocol{}
	launcher := &stoppableGenericCLIHostLauncher{}
	protocol.init()
	process, directory, environment := buildGenericCLIProcess(t, singleOutputModelFactoryConfig, nil, launcher, protocol, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "cancel me",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = directory
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	waitForGenericCLIEventOrResult(t, protocol.started, done, "readiness negotiation start")
	cancel()
	err := waitForGenericCLIResult(t, done, "cancellation")
	if err == nil || (!errors.Is(err, models.ErrInferenceCancelled) && !errors.Is(err, context.Canceled)) {
		t.Fatalf("cancelled Process.Execute error = %v, want cancellation-class error", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("cancelled stdout = %q, want no successful publication", inputs.Stdout())
	}
	if launcher.StopCalls() == 0 {
		t.Fatal("cancelled readiness did not stop the supervised process")
	}
	closeRootProcess(t, process, "close cancelled root process")
}

// TestModelsGenericCLIProcessTimeoutStopsReadinessAndPublishesNothing proves timeout stops readiness without publishing output.
func TestModelsGenericCLIProcessTimeoutStopsReadinessAndPublishesNothing(t *testing.T) {
	t.Parallel()

	protocol := &blockingGenericCLIProtocol{}
	launcher := &stoppableGenericCLIHostLauncher{}
	protocol.init()
	process, directory, environment := buildGenericCLIProcess(t, singleOutputModelFactoryConfig, nil, launcher, protocol, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "timeout me",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = directory
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	waitForGenericCLIEventOrResult(t, protocol.started, done, "readiness negotiation start")
	// The protocol returns only after the context deadline; the bounded select
	// below joins the customer-boundary operation without polling or sleeping.
	err := waitForGenericCLIResult(t, done, "timeout")
	if err == nil || !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("timed-out Process.Execute error = %v, want cancellation-class timeout", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("timed-out stdout = %q, want no successful publication", inputs.Stdout())
	}
	if launcher.StopCalls() == 0 {
		t.Fatal("timed-out readiness did not stop the supervised process")
	}
	closeRootProcess(t, process, "close timed-out root process")
}

// TestModelsGenericCLIProcessRedactsCrashedHostDetails proves crashed-host diagnostics redact sensitive process details.
func TestModelsGenericCLIProcessRedactsCrashedHostDetails(t *testing.T) {
	t.Parallel()

	const secret = "HF_TOKEN=fixture-secret endpoint=https://private.invalid:7437/cache"
	launcher := &crashedGenericCLIHostLauncher{waitErr: errors.New(secret)}
	process, directory, environment := buildGenericCLIProcess(t, singleOutputModelFactoryConfig, nil, launcher, nil, nil)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "crash me",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = directory
	err := process.Execute(inputs.Input)
	if launcher.StartCalls() == 0 {
		t.Fatalf("crashed host launcher was not started; Process.Execute error = %v", err)
	}
	if err == nil || (!errors.Is(err, models.ErrHostProcessCrash) && !errors.Is(err, models.ErrHostRuntimeNotReady)) {
		t.Fatalf("crashed Process.Execute error = %v, want provider-neutral host failure", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "7437") || strings.Contains(err.Error(), "HF_TOKEN") {
		t.Fatalf("crashed host error leaked private details: %v", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("crashed stdout = %q, want no successful publication", inputs.Stdout())
	}
	closeRootProcess(t, process, "close crashed root process")
}

func buildGenericCLIProcess(
	t *testing.T,
	configFactory func(string) map[string]any,
	outputEffects *genericCLIOutputFailureEffects,
	launcher genericCLIHostLauncher,
	protocol genericCLIProtocolNegotiator,
	compatibility genericCLICompatibilityChecker,
) (support.Process, string, []string) {
	t.Helper()
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)
	home := t.TempDir()
	selection := genericLlamaBackendSelection()
	writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))
	if launcher == nil {
		launcher = &recordingModelHostLauncher{endpoint: modelServer.URL}
	}
	if protocol == nil {
		protocol = &joinedProtocolNegotiator{}
	}
	if compatibility == nil {
		compatibility = &joinedCompatibilityChecker{}
	}
	assetFiles := functionalModelAssetFileSystem{home: home}
	config := configFactory(modelServer.URL)
	directory := support.ScaffoldFactory(t, config)
	edges := serviceedges.Edges{
		ModelAssetHTTPClient: &rejectingModelAssetHTTP{},
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       launcher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
		ModelInvocationProtocolClient:  genericCLIProtocolClient{},
	}
	if outputEffects != nil {
		edges.ModelCLIOutputCreateTempFile = outputEffects.CreateTemp
		edges.ModelCLIOutputInspectPath = outputEffects.Inspect
		edges.ModelCLIOutputRemovePath = outputEffects.Remove
		edges.ModelCLIOutputRenamePath = outputEffects.Rename
	}
	return support.BuildProcess(t, edges), directory, functionalHomeEnvironment(home)
}

type genericCLIProtocolClient struct{}

func (genericCLIProtocolClient) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	value := request.Prompt
	if value == "" && len(request.Inputs) > 0 {
		value = request.Inputs[0].Content
	}
	return models.InvocationProtocolResponse{Text: value, Usage: value}, nil
}

func assertFunctionalFile(t testing.TB, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("file %s = %q, %v; want %q", path, data, err, want)
	}
}

func singleOutputModelFactoryConfig(endpoint string) map[string]any {
	config := multiOutputModelFactoryConfig(endpoint)
	workers := config["workers"].([]map[string]any)
	workers[0]["operations"] = []map[string]any{{
		"name": "OMNI",
		"inputs": []map[string]any{{
			"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true,
		}},
		"outputs": []map[string]any{{
			"name": "text", "contentTypes": []string{"TEXT"}, "required": true,
		}},
	}}
	return config
}

func genericLlamaBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Bytes:    28,
		SHA256:   "9285e7ffc76aaadf4dfcc6b2de5e23c6b01d4e7068e8f2dd65673626cc5de4ed",
	}
}

func waitForGenericCLIEventOrResult(t testing.TB, event <-chan struct{}, done <-chan error, name string) {
	t.Helper()
	select {
	case <-event:
	case err := <-done:
		t.Fatalf("Process.Execute returned before %s: %v", name, err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitForGenericCLIResult(t testing.TB, done <-chan error, name string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s Process.Execute result", name)
		return nil
	}
}

type blockingGenericCLIProtocol struct {
	started chan struct{}
}

func (protocol *blockingGenericCLIProtocol) init() {
	if protocol.started == nil {
		protocol.started = make(chan struct{})
	}
}

func (protocol *blockingGenericCLIProtocol) Negotiate(
	ctx context.Context,
	_ string,
	_ serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	protocol.init()
	select {
	case <-protocol.started:
	default:
		close(protocol.started)
	}
	<-ctx.Done()
	return serviceedges.ModelHostProtocolNegotiationResult{}, ctx.Err()
}

type stoppableGenericCLIHostLauncher struct {
	mu       sync.Mutex
	endpoint string
	stopCall atomic.Int32
}

func (launcher *stoppableGenericCLIHostLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	if launcher.endpoint == "" {
		launcher.endpoint = spec.HealthEndpoint
	}
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &stoppableGenericCLIHostProcess{endpoint: endpoint, launcher: launcher, stopped: make(chan struct{})}, nil
}

func (launcher *stoppableGenericCLIHostLauncher) StopCalls() int {
	return int(launcher.stopCall.Load())
}

type stoppableGenericCLIHostProcess struct {
	endpoint string
	launcher *stoppableGenericCLIHostLauncher
	stopped  chan struct{}
	once     sync.Once
}

func (process *stoppableGenericCLIHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *stoppableGenericCLIHostProcess) Wait() error {
	<-process.stopped
	return nil
}
func (process *stoppableGenericCLIHostProcess) Stop(context.Context) error {
	process.launcher.stopCall.Add(1)
	process.once.Do(func() { close(process.stopped) })
	return nil
}

type crashedGenericCLIHostLauncher struct {
	waitErr   error
	startCall atomic.Int32
}

func (launcher *crashedGenericCLIHostLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.startCall.Add(1)
	return nil, launcher.waitErr
}

func (launcher *crashedGenericCLIHostLauncher) StartCalls() int {
	return int(launcher.startCall.Load())
}

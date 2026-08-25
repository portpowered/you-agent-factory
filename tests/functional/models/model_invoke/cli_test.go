package model_invoke_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestProcessModelsInvokeUsesCanonicalGraphAndExactExternalEdges proves that
// the public Models CLI resolves the current built-in TTS model through a
// root.BuildProcess and uses only the published external-effect edges.
func TestProcessModelsInvokeUsesCanonicalGraphAndExactExternalEdges(t *testing.T) {
	t.Parallel()

	fixture := localai.Start(t)
	modelServer := processModelHealthServer(t)
	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	factoryDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	var backendRequests []models.InvokeModelRequest
	boundaries := newProcessTTSBoundaries(home, modelServer, func(
		ctx context.Context,
		request models.InvokeModelRequest,
	) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		backendRequests = append(backendRequests, request)
		return fixture.InvocationBackend(ctx, request)
	})
	process, err := root.BuildProcess(context.Background(), boundaries.edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := process.Close(context.Background()); closeErr != nil {
			t.Errorf("close reusable process: %v", closeErr)
		}
	})

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	if err := process.Execute(root.Input{
		Args: []string{
			"you", "--json", "models", "invoke", "tts",
			"--operation", "TTS", "--text", "hello from the process",
		},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	}); err != nil {
		t.Fatalf("Process.Execute(models invoke) error = %v", err)
	}

	var response struct {
		ModelName         string `json:"modelName"`
		Operation         string `json:"operation"`
		Mode              string `json:"mode"`
		ValidationOnly    bool   `json:"validationOnly"`
		InferenceExecuted bool   `json:"inferenceExecuted"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "tts" || response.Operation != "TTS" ||
		response.Mode != "VALIDATION_ONLY" || !response.ValidationOnly || response.InferenceExecuted {
		t.Fatalf("response = %#v, want validation-only tts/TTS metadata", response)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("models invoke JSON stderr = %q, want empty", diagnostics.String())
	}

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	output.Reset()
	diagnostics.Reset()
	if err := process.Execute(root.Input{
		Args: []string{
			"you", "models", "invoke", "tts",
			"--operation", "TTS", "--text", "write audio from the process",
			"--output", audioPath,
		},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	}); err != nil {
		t.Fatalf("Process.Execute(models invoke --output) error = %v", err)
	}
	if got, want := output.String(), "Wrote audio: "+audioPath+"\n"; got != want {
		t.Fatalf("models invoke stdout = %q, want %q", got, want)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("models invoke stderr = %q, want empty", diagnostics.String())
	}
	if len(backendRequests) != 1 {
		t.Fatalf("backend invocation count = %d, want only the file invocation", len(backendRequests))
	}
	request := backendRequests[0]
	if request.ModelName != "tts" || request.Operation != models.OperationTTS ||
		len(request.Inputs) != 1 || request.Inputs[0].Content != "write audio from the process" {
		t.Fatalf("backend TTS request = %#v, want canonical tts/TTS text request", request)
	}

	wantAudio := localai.AudioBytes()
	written, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read models invoke audio: %v", err)
	}
	if !bytes.Equal(written, wantAudio) {
		t.Fatalf("models invoke audio = %d bytes, want fixture audio %d bytes", len(written), len(wantAudio))
	}
	if boundaries.assetNetwork.Calls() != 0 {
		t.Fatalf("asset network calls = %d, want 0 from content-addressed cache", boundaries.assetNetwork.Calls())
	}
}

func TestProcessModelsInvokeFailureKeepsStreamsSafeAndReleasesCapacity(t *testing.T) {
	t.Parallel()

	fixture := localai.Start(t)
	modelServer := processModelHealthServer(t)
	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	factoryDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	var backendMu sync.Mutex
	failBackend := false
	backendInvocations := 0
	boundaries := newProcessTTSBoundaries(home, modelServer, func(
		ctx context.Context,
		request models.InvokeModelRequest,
	) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		backendMu.Lock()
		backendInvocations++
		shouldFail := failBackend
		backendMu.Unlock()
		if shouldFail {
			return nil, nil, errors.New("deterministic TTS backend failure")
		}
		return fixture.InvocationBackend(ctx, request)
	})
	process, err := root.BuildProcess(context.Background(), boundaries.edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := process.Close(context.Background()); closeErr != nil {
			t.Errorf("close reusable process: %v", closeErr)
		}
	})

	firstAudioPath := filepath.Join(t.TempDir(), "first.wav")
	stdout, stderr, err := executeProcessTTS(t, process, home, factoryDir, firstAudioPath, "successful invocation")
	if err != nil {
		t.Fatalf("first Process.Execute(models invoke) error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	assertSuccessfulProcessTTS(t, stdout, stderr, firstAudioPath, localai.AudioBytes())

	secondAudioPath := filepath.Join(t.TempDir(), "second.wav")
	stdout, stderr, err = executeProcessTTS(t, process, home, factoryDir, secondAudioPath, "follow-up after success")
	if err != nil {
		t.Fatalf("follow-up after successful Process.Execute(models invoke) error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	assertSuccessfulProcessTTS(t, stdout, stderr, secondAudioPath, localai.AudioBytes())

	backendMu.Lock()
	failBackend = true
	backendMu.Unlock()
	failedAudioPath := filepath.Join(t.TempDir(), "failed.wav")
	stdout, stderr, err = executeProcessTTS(t, process, home, factoryDir, failedAudioPath, "failed invocation")
	if err == nil {
		t.Fatal("failed Process.Execute(models invoke) error = nil, want backend failure")
	}
	if stdout != "" {
		t.Fatalf("failed models invoke stdout = %q, want empty", stdout)
	}
	support.RequireSafeCLIDiagnostic(t, stderr)
	if _, statErr := os.Stat(failedAudioPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed models invoke artifact stat error = %v, want no customer audio artifact", statErr)
	}

	backendMu.Lock()
	failBackend = false
	backendMu.Unlock()
	finalAudioPath := filepath.Join(t.TempDir(), "after-failure.wav")
	stdout, stderr, err = executeProcessTTS(t, process, home, factoryDir, finalAudioPath, "follow-up after failure")
	if err != nil {
		t.Fatalf("follow-up after failed Process.Execute(models invoke) error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	assertSuccessfulProcessTTS(t, stdout, stderr, finalAudioPath, localai.AudioBytes())

	backendMu.Lock()
	gotBackendInvocations := backendInvocations
	backendMu.Unlock()
	if gotBackendInvocations != 4 {
		t.Fatalf("backend invocation count = %d, want four success/success/failure/success attempts", gotBackendInvocations)
	}
}

func executeProcessTTS(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	home string,
	factoryDir string,
	outputPath string,
	text string,
) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := process.Execute(root.Input{
		Args: []string{
			"you", "models", "invoke", "tts",
			"--operation", "TTS", "--text", text, "--output", outputPath,
		},
		Env:              homeEnvironment(home),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	})
	return stdout.String(), stderr.String(), err
}

func assertSuccessfulProcessTTS(
	t *testing.T,
	stdout string,
	stderr string,
	outputPath string,
	wantAudio []byte,
) {
	t.Helper()
	if got, want := stdout, "Wrote audio: "+outputPath+"\n"; got != want {
		t.Fatalf("successful models invoke stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Fatalf("successful models invoke stderr = %q, want empty", stderr)
	}
	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read successful models invoke audio: %v", err)
	}
	if !bytes.Equal(written, wantAudio) {
		t.Fatalf("successful models invoke audio = %d bytes, want fixture audio %d bytes", len(written), len(wantAudio))
	}
}

type processTTSBoundaries struct {
	edges         serviceedges.Edges
	assetNetwork  *processRejectingAssetHTTP
	launcher      *processModelLauncher
	protocol      *processProtocolNegotiator
	compatibility *processCompatibilityChecker
	hostHTTP      *processRecordingHTTPClient
}

func newProcessTTSBoundaries(
	home string,
	modelServer *httptest.Server,
	backend serviceedges.ModelInvocationBackend,
) processTTSBoundaries {
	assetFiles := processModelAssetFileSystem{home: home}
	assetNetwork := &processRejectingAssetHTTP{}
	launcher := &processModelLauncher{endpoint: modelServer.URL}
	protocol := &processProtocolNegotiator{}
	compatibility := &processCompatibilityChecker{}
	hostHTTP := &processRecordingHTTPClient{delegate: modelServer.Client()}
	return processTTSBoundaries{
		assetNetwork: assetNetwork, launcher: launcher, protocol: protocol,
		compatibility: compatibility, hostHTTP: hostHTTP,
		edges: serviceedges.Edges{
			ModelAssetHTTPClient:           assetNetwork,
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
			ModelResolveBackendArtifact: func(
				context.Context,
				serviceedges.ModelBackendArtifactSelectionRequest,
			) (serviceedges.ModelBackendArtifactSelection, error) {
				return processPinnedTTSBackendSelection(), nil
			},
			ModelInvocationBackend: backend,
			ModelHostHTTPClient:    hostHTTP,
			ModelRuntimeHTTPClient: hostHTTP,
		},
	}
}

func processModelHealthServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(server.Close)
	return server
}

func builtInOnlyModelFactoryConfig() map[string]any {
	return map[string]any{
		"name": "models-built-in-only",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
	}
}

const processBuiltinTTSSource = "hf://vibevoice/VibeVoice-7B@505114ae6ad17be74df98e6939707434ec49c187"

func writeGenericBuiltinTTSCache(t *testing.T, home string) {
	t.Helper()
	body := []byte("joined built-in tts fixture")
	name := "weights.bin"
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", processBuiltinTTSSource, name, len(body), digest)
	snapshot := filepath.Join(home, ".agent-factory", "models", ".you-content-addressed", "model", fmt.Sprintf("%x", sha256.Sum256([]byte(identity))))
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create generic model snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o644); err != nil {
		t.Fatalf("write generic model snapshot: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "model", "identity": identity, "source": processBuiltinTTSSource, "sourceKey": processBuiltinTTSSource,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	})
	if err != nil {
		t.Fatalf("marshal generic model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write generic model metadata: %v", err)
	}
}

func processPinnedTTSBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Bytes:    22,
		SHA256:   "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
	}
}

func writeGenericBuiltinTTSBackendCache(t *testing.T, home string) {
	t.Helper()
	selection := processPinnedTTSBackendSelection()
	body := []byte("pinned-backend-fixture")
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://localai-vibevoice/release://" + urlHash
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, selection.SHA256)
	snapshot := filepath.Join(home, ".agent-factory", "models", "backend-artifacts", ".you-content-addressed", "backend", fmt.Sprintf("%x", sha256.Sum256([]byte(identity))))
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create generic backend snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), body, 0o644); err != nil {
		t.Fatalf("write generic backend snapshot: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256}},
	})
	if err != nil {
		t.Fatalf("marshal generic backend metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write generic backend metadata: %v", err)
	}
}

type processRejectingAssetHTTP struct {
	mu    sync.Mutex
	calls int
}

func (client *processRejectingAssetHTTP) Do(*http.Request) (*http.Response, error) {
	client.mu.Lock()
	client.calls++
	client.mu.Unlock()
	return nil, errors.New("unexpected model asset network request")
}

func (client *processRejectingAssetHTTP) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

type processRecordingHTTPClient struct {
	delegate *http.Client
	mu       sync.Mutex
	calls    int
}

func (client *processRecordingHTTPClient) Do(request *http.Request) (*http.Response, error) {
	client.mu.Lock()
	client.calls++
	client.mu.Unlock()
	return client.delegate.Do(request)
}

func (client *processRecordingHTTPClient) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

type processProtocolNegotiator struct {
	mu    sync.Mutex
	calls int
}

func (negotiator *processProtocolNegotiator) Negotiate(
	context.Context,
	string,
	serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	negotiator.mu.Lock()
	negotiator.calls++
	negotiator.mu.Unlock()
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: "localai-backend-v1", Backend: "localai-vibevoice", Ready: true,
	}, nil
}

func (negotiator *processProtocolNegotiator) Calls() int {
	negotiator.mu.Lock()
	defer negotiator.mu.Unlock()
	return negotiator.calls
}

type processCompatibilityChecker struct {
	mu    sync.Mutex
	calls int
}

func (checker *processCompatibilityChecker) Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error {
	checker.mu.Lock()
	checker.calls++
	checker.mu.Unlock()
	return nil
}

func (checker *processCompatibilityChecker) Calls() int {
	checker.mu.Lock()
	defer checker.mu.Unlock()
	return checker.calls
}

type processModelLauncher struct {
	mu       sync.Mutex
	calls    int
	endpoint string
}

func (launcher *processModelLauncher) Start(
	context.Context,
	serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	launcher.calls++
	launcher.mu.Unlock()
	return &processModelProcess{endpoint: launcher.endpoint, stopped: make(chan struct{})}, nil
}

func (launcher *processModelLauncher) Calls() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.calls
}

type processModelProcess struct {
	endpoint string
	stopped  chan struct{}
	once     sync.Once
}

func (process *processModelProcess) HealthEndpoint() string { return process.endpoint }
func (process *processModelProcess) Stop(context.Context) error {
	process.once.Do(func() { close(process.stopped) })
	return nil
}
func (process *processModelProcess) Wait() error {
	<-process.stopped
	return nil
}

type processModelAssetFileSystem struct{ home string }

func (fs processModelAssetFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (fs processModelAssetFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
func (fs processModelAssetFileSystem) UserHomeDir() (string, error) { return fs.home, nil }
func (fs processModelAssetFileSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}
func (fs processModelAssetFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
func (fs processModelAssetFileSystem) Remove(path string) error { return os.Remove(path) }
func (fs processModelAssetFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (fs processModelAssetFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}
func (fs processModelAssetFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}
func (fs processModelAssetFileSystem) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func homeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

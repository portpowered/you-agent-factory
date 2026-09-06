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
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	boundaries := newProcessTTSBoundaries(home, modelServer, fixture, func(
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

	assertProcessModelsValidationOnly(t, &output)
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
	if len(backendRequests) != 0 {
		t.Fatalf("generic backend invocation count = %d, want zero private-route fallback calls", len(backendRequests))
	}
	ttsCalls := fixture.Calls()
	if len(ttsCalls) != 1 || ttsCalls[0].Method != "TTS" || ttsCalls[0].Model != "tts" || ttsCalls[0].Text != "write audio from the process" {
		t.Fatalf("private TTS calls = %#v, want one canonical tts/TTS text request", ttsCalls)
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

func assertProcessModelsValidationOnly(t testing.TB, output *bytes.Buffer) {
	t.Helper()
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
}

// TestProcessLegacyModelsInvokeMissingFactoryLayoutReportsFailure proves the
// legacy named-model output form resolves its Factory directory before trying
// to open an invocation runtime or publish an artifact.
func TestProcessLegacyModelsInvokeMissingFactoryLayoutReportsFailure(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	workingDirectory := t.TempDir()
	outputPath := filepath.Join(workingDirectory, "speech.wav")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "missing layout", "--output", outputPath,
	})
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(legacy models invoke) error = nil, want missing-layout failure")
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("legacy models invoke output stat error = %v, want no artifact", err)
	}
}

// TestProcessLegacyNamedModelInvokeUsesInvocationOperation proves a named
// Factory model with an audio output still reaches the legacy invocation
// operation and publishes the streamed artifact through Process.Execute.
func TestProcessLegacyNamedModelInvokeUsesInvocationOperation(t *testing.T) {
	t.Parallel()

	audio := localai.AudioBytes()
	modelServer := processLegacyModelServer(t, audio)
	home := t.TempDir()
	writeReadyOmniVoiceCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	factoryDir := support.ScaffoldFactory(t, legacyModelFactoryConfig(modelServer.URL))
	boundaries := newProcessTTSBoundaries(home, modelServer, nil, nil)
	process, err := root.BuildProcess(context.Background(), boundaries.edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := process.Close(context.Background()); closeErr != nil {
			t.Errorf("close reusable process: %v", closeErr)
		}
	})

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	stdout, stderr, err := executeLegacyModelTTS(t, process, home, factoryDir, outputPath)
	if err != nil {
		t.Fatalf("Process.Execute(legacy models invoke) error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	if stdout != "Wrote audio: "+outputPath+"\n" || stderr != "" {
		t.Fatalf("legacy models invoke streams = stdout %q stderr %q, want status-only stdout", stdout, stderr)
	}
	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read legacy models invoke audio: %v", err)
	}
	if !bytes.Equal(written, audio) {
		t.Fatalf("legacy models invoke audio = %q, want %q", written, audio)
	}
}

func executeLegacyModelTTS(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	home, factoryDir, outputPath string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := process.Execute(root.Input{
		Args: []string{
			"you", "models", "invoke", "OMNIVOICE_Q4_K_M",
			"--operation", "TTS", "--text", "legacy invocation", "--output", outputPath,
		},
		Env: homeEnvironment(home), Stdout: &stdout, Stderr: &stderr,
		Context: context.Background(), WorkingDirectory: factoryDir,
	})
	return stdout.String(), stderr.String(), err
}

func TestProcessModelsInvokeFailureKeepsStreamsSafeAndReleasesCapacity(t *testing.T) {
	t.Parallel()

	fixture := localai.Start(t, localai.Options{TTSFailureText: "failed invocation"})
	modelServer := processModelHealthServer(t)
	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	factoryDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	backendInvocations := 0
	boundaries := newProcessTTSBoundaries(home, modelServer, fixture, func(
		ctx context.Context,
		request models.InvokeModelRequest,
	) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		backendInvocations++
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

	failedAudioPath := filepath.Join(t.TempDir(), "failed.wav")
	stdout, stderr, err = executeProcessTTS(t, process, home, factoryDir, failedAudioPath, "failed invocation")
	if err == nil {
		t.Fatal("failed Process.Execute(models invoke) error = nil, want backend failure")
	}
	if stdout != "" {
		t.Fatalf("failed models invoke stdout = %q, want empty", stdout)
	}
	var diagnostic factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(stderr), &diagnostic); decodeErr != nil {
		t.Fatalf("decode failed models invoke diagnostic: %v\nstderr=%q", decodeErr, stderr)
	}
	if diagnostic.Code != factoryapi.ErrorResponseCode("MODEL_BACKEND_NOT_READY") || diagnostic.Message != "TTS backend is unavailable" {
		t.Fatalf("failed models invoke diagnostic = %#v, want typed backend-readiness response", diagnostic)
	}
	if _, statErr := os.Stat(failedAudioPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed models invoke artifact stat error = %v, want no customer audio artifact", statErr)
	}

	finalAudioPath := filepath.Join(t.TempDir(), "after-failure.wav")
	stdout, stderr, err = executeProcessTTS(t, process, home, factoryDir, finalAudioPath, "follow-up after failure")
	if err != nil {
		t.Fatalf("follow-up after failed Process.Execute(models invoke) error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	assertSuccessfulProcessTTS(t, stdout, stderr, finalAudioPath, localai.AudioBytes())

	if backendInvocations != 0 {
		t.Fatalf("generic backend invocation count = %d, want zero private-route fallback calls", backendInvocations)
	}
	ttsCalls := fixture.Calls()
	if len(ttsCalls) != 4 {
		t.Fatalf("private TTS invocation count = %d, want four success/success/failure/success attempts", len(ttsCalls))
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
	privateFixture *localai.Fixture,
	backend serviceedges.ModelInvocationBackend,
) processTTSBoundaries {
	assetFiles := processModelAssetFileSystem{home: home}
	assetNetwork := &processRejectingAssetHTTP{}
	launcherEndpoint := modelServer.URL
	if privateFixture != nil {
		launcherEndpoint = privateFixture.Endpoint()
	}
	launcher := &processModelLauncher{endpoint: launcherEndpoint}
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

func legacyModelFactoryConfig(endpoint string) map[string]any {
	return map[string]any{
		"name": "legacy-model-invoke",
		"resources": []map[string]any{{
			"name": "omnivoice-cache", "type": factorydefinitions.ResourceTypeModel,
			"capacity": 1, "model": "OMNIVOICE_Q4_K_M", "backend": "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name": "voice-local", "type": factorydefinitions.WorkerTypeModel,
			"modelProvider": "CODEX", "model": "OMNIVOICE_Q4_K_M",
			"modelLocality": factorydefinitions.ModelLocalityLocal,
			"command":       "omnivoice-llamacpp", "args": []string{"--health-endpoint", endpoint},
			"resources": []map[string]any{{"name": "omnivoice-cache", "capacity": 1}},
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name": "text", "contentTypes": []string{factorydefinitions.ModelOperationContentTypeText},
					"required": true,
				}},
				"outputs": []map[string]any{{
					"name": "audio", "contentTypes": []string{factorydefinitions.ModelOperationContentTypeAudio},
				}},
			}},
		}},
	}
}

func writeReadyOmniVoiceCache(t *testing.T, home string) {
	t.Helper()
	revisionDir := filepath.Join(home, ".agent-factory", "models", "OMNIVOICE_Q4_K_M", "rev-test")
	if err := os.MkdirAll(revisionDir, 0o755); err != nil {
		t.Fatalf("create model cache fixture: %v", err)
	}
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(revisionDir, name), []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write model cache fixture %s: %v", name, err)
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M", "revision": "rev-test",
		"files": []map[string]any{{"path": files[0]}, {"path": files[1]}},
	})
	if err != nil {
		t.Fatalf("marshal model cache metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(revisionDir), ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write model cache metadata: %v", err)
	}
}

func processLegacyModelServer(t *testing.T, audio []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			writer.WriteHeader(http.StatusOK)
		case "/invoke":
			var payload struct {
				OutputFile string `json:"outputFile"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			if err := os.WriteFile(payload.OutputFile, audio, 0o644); err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			response, err := json.Marshal(map[string]any{
				"content": []map[string]any{{
					"type": "AUDIO", "slot": "audio", "file": payload.OutputFile,
					"contentType": "audio/wav",
				}},
			})
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write(response)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
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

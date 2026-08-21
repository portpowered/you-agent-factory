package root_composition_test

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

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsInferenceInvokeActivatesThroughRootBuildProcess proves a process
// constructed only through root.BuildProcess executes public Models invoke and
// returns observable inference output while host, runtime, and asset external
// effects are replaced exclusively through published edges.Edges fields.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestModelsInferenceInvokeActivatesThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	audio := []byte("RIFF....WAVE")
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(modelServer.Close)

	cacheDirectory := t.TempDir()
	writeReadyOmniVoiceInvokeCache(t, cacheDirectory)

	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	hostHTTP := &recordingModelHTTPClient{delegate: modelServer.Client()}
	assetFiles := functionalModelAssetFileSystem{home: cacheDirectory}

	dir := support.ScaffoldFactory(t, localModelInferenceInvokeFactoryConfig(modelServer.URL))
	environment := functionalHomeEnvironment(cacheDirectory)
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostHTTPClient:            hostHTTP,
		ModelRuntimeHTTPClient:         modelServer.Client(),
	})

	var output bytes.Buffer
	jsonInvoke := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "hello from root composition invoke",
	})
	jsonInvoke.Input.Env = environment
	jsonInvoke.Input.WorkingDirectory = dir
	jsonInvoke.Input.Stdout = &output
	jsonInvoke.Input.Stderr = io.Discard
	if err := process.Execute(jsonInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke --json) error = %v", err)
	}

	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" || response.Operation != "TTS" {
		t.Fatalf("models invoke identity = %#v, want OMNIVOICE_Q4_K_M/TTS", response)
	}
	if response.Worker != "tts-worker" || len(response.Content) == 0 {
		t.Fatalf("models invoke response = %#v, want tts-worker content", response)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("asset network calls = %d during invoke, want 0 via edges", rejectingNetwork.Calls())
	}
	if hostLauncher.Calls() == 0 {
		t.Fatal("host process launcher calls = 0 after invoke, want host activation through edges")
	}
	if hostHTTP.Calls() == 0 {
		t.Fatal("host HTTP client calls = 0 after invoke, want inference activation through edges")
	}

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	output.Reset()
	audioInvoke := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "write audio from root composition invoke",
		"--output", audioPath,
	})
	audioInvoke.Input.Env = environment
	audioInvoke.Input.WorkingDirectory = dir
	audioInvoke.Input.Stdout = &output
	audioInvoke.Input.Stderr = io.Discard
	if err := process.Execute(audioInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(models invoke --output) error = %v", err)
	}
	written, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read models invoke audio: %v", err)
	}
	if !bytes.Equal(written, audio) {
		t.Fatalf("models invoke audio = %q, want %q", written, audio)
	}
}

// TestModelsJoinedInvokeConsumesGenericCacheThroughRootBuildProcess proves
// the generic joined path can use a prepared content-addressed model snapshot
// through the real root composition, without a legacy managed-cache fixture or
// a model-weight download.
func TestModelsJoinedInvokeConsumesGenericCacheThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	config := localModelReadinessAssetsHostFactoryConfig(modelServer.URL)
	resources := config["resources"].([]map[string]any)
	resources[0]["model"] = "tts"
	resources[0]["backend"] = "localai-vibevoice"
	workers := config["workers"].([]map[string]any)
	workers[0]["name"] = "tts-worker"
	workers[0]["model"] = "tts"
	workers[0]["args"] = []string{"--grpc-endpoint", modelServer.URL}

	dir := support.ScaffoldFactory(t, config)
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
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
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelResolveBackendArtifact:    resolvePinnedTTSBackend,
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
	})

	var output bytes.Buffer
	jsonInvoke := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "tts", "--operation", "TTS", "--text", "joined cache input",
	})
	jsonInvoke.Input.Env = functionalHomeEnvironment(home)
	jsonInvoke.Input.WorkingDirectory = dir
	jsonInvoke.Input.Stdout = &output
	jsonInvoke.Input.Stderr = io.Discard
	if err := process.Execute(jsonInvoke.Input); err != nil {
		t.Fatalf("Process.Execute(joined built-in invoke) error = %v", err)
	}

	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode joined models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "tts" || response.Operation != "TTS" || len(response.Content) == 0 {
		t.Fatalf("joined models invoke response = %#v, want tts/TTS content", response)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("joined asset network calls = %d, want 0 from content-addressed cache", rejectingNetwork.Calls())
	}
	if hostLauncher.Calls() != 1 {
		t.Fatalf("joined host starts = %d, want exactly 1", hostLauncher.Calls())
	}

	closer, ok := process.(interface{ Close(context.Context) error })
	if !ok {
		t.Fatal("root process does not expose lifecycle close")
	}
	if err := closer.Close(context.Background()); err != nil {
		t.Fatalf("close joined root process: %v", err)
	}
	if protocol.Calls() == 0 {
		t.Fatalf("joined protocol negotiations = %d, want at least 1", protocol.Calls())
	}
	if compatibility.Calls() == 0 {
		t.Fatalf("joined compatibility checks = %d, want at least 1", compatibility.Calls())
	}
}

// TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess proves the
// registered generic HTTP route uses the live Models scope and returns named
// output from the joined root rather than the legacy model-invocation envelope.
func TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	config := localModelReadinessAssetsHostFactoryConfig(modelServer.URL)
	resources := config["resources"].([]map[string]any)
	resources[0]["model"] = "tts"
	resources[0]["backend"] = "localai-vibevoice"
	workers := config["workers"].([]map[string]any)
	workers[0]["name"] = "tts-worker"
	workers[0]["model"] = "tts"
	workers[0]["args"] = []string{"--grpc-endpoint", modelServer.URL}
	dir := support.ScaffoldFactory(t, config)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       functionalHomeEnvironment(home),
		Edges:                     genericHTTPInvocationEdges(rejectingNetwork, assetFiles, hostLauncher, protocol, compatibility, modelServer),
	})

	inputs := []factoryapi.ModelInvocationInput{{
		Name: "text", Modality: factoryapi.ModelInvocationContentTypeText,
		Content: func() *string { value := "generic HTTP input"; return &value }(),
	}}
	response := postFunctionalJSON[factoryapi.GenericModelInvocationResponse](
		t,
		server.URL()+"/models/invocations",
		factoryapi.GenericModelInvocationRequest{
			Scope: "factory-session:caller-supplied", Holder: "functional-generic-http",
			Model: factoryapi.ModelReference{NameOrUri: "tts"}, Operation: "TTS", Inputs: &inputs,
		},
		"POST /models/invocations",
	)
	if len(response.Outputs) != 1 || response.Outputs[0].Name != "audio" || response.Outputs[0].Modality != factoryapi.ModelInvocationContentTypeAudio || response.Outputs[0].Content == nil || *response.Outputs[0].Content != "generic HTTP input" {
		t.Fatalf("generic HTTP response = %#v, want one named audio output", response)
	}
	if rejectingNetwork.Calls() != 0 || hostLauncher.Calls() != 1 || protocol.Calls() == 0 || compatibility.Calls() == 0 {
		t.Fatalf("generic HTTP effects = network %d, starts %d, protocol %d, compatibility %d; want cache hit and joined lifecycle", rejectingNetwork.Calls(), hostLauncher.Calls(), protocol.Calls(), compatibility.Calls())
	}
}

func genericHTTPInvocationEdges(
	rejectingNetwork *rejectingModelAssetHTTP,
	assetFiles functionalModelAssetFileSystem,
	hostLauncher *recordingModelHostLauncher,
	protocol *joinedProtocolNegotiator,
	compatibility *joinedCompatibilityChecker,
	modelServer *httptest.Server,
) serviceedges.Edges {
	return serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
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
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelResolveBackendArtifact:    resolvePinnedTTSBackend,
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
	}
}

func TestModelsJoinedInvokeRejectsPinnedBackendBeforeProcessStartThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{failure: errors.New("fixture backend is incompatible")}
	assetFiles := functionalModelAssetFileSystem{home: home}
	config := localModelReadinessAssetsHostFactoryConfig(modelServer.URL)
	resources := config["resources"].([]map[string]any)
	resources[0]["model"] = "tts"
	resources[0]["backend"] = "localai-vibevoice"
	workers := config["workers"].([]map[string]any)
	workers[0]["model"] = "tts"
	workers[0]["args"] = []string{"--grpc-endpoint", modelServer.URL}
	dir := support.ScaffoldFactory(t, config)
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
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
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelResolveBackendArtifact:    resolvePinnedTTSBackend,
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
	})

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "tts", "--operation", "TTS", "--text", "must fail preflight",
	})
	inputs.Input.Env = functionalHomeEnvironment(home)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Stdout = io.Discard
	inputs.Input.Stderr = io.Discard
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(incompatible joined invoke) error = nil, want preflight failure")
	}
	if compatibility.Calls() != 1 || protocol.Calls() != 0 || hostLauncher.Calls() != 0 {
		t.Fatalf(
			"pinned preflight effects = compatibility %d, protocol %d, starts %d; want 1/0/0",
			compatibility.Calls(), protocol.Calls(), hostLauncher.Calls(),
		)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("pinned preflight asset network calls = %d, want 0", rejectingNetwork.Calls())
	}
}

func localModelInferenceInvokeFactoryConfig(endpoint string) map[string]any {
	return localModelReadinessAssetsHostFactoryConfig(endpoint)
}

func writeReadyOmniVoiceInvokeCache(t *testing.T, home string) {
	t.Helper()
	modelRoot := filepath.Join(home, ".agent-factory", "models", "OMNIVOICE_Q4_K_M")
	revisionDir := filepath.Join(modelRoot, "rev-test")
	if err := os.MkdirAll(revisionDir, 0o755); err != nil {
		t.Fatalf("create model cache fixture: %v", err)
	}
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(revisionDir, name), []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write model cache fixture %s: %v", name, err)
		}
	}
	metadata := map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M", "revision": "rev-test",
		"files": []map[string]any{{"path": files[0]}, {"path": files[1]}},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal model cache metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelRoot, ".managed-cache.json"), data, 0o644); err != nil {
		t.Fatalf("write model cache metadata: %v", err)
	}
}

func writeGenericBuiltinTTSCache(t *testing.T, home string) {
	t.Helper()
	const source = "hf://vibevoice/VibeVoice-7B@505114ae6ad17be74df98e6939707434ec49c187"
	name := "weights.bin"
	body := []byte("joined built-in tts fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", source, name, len(body), digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", ".you-content-addressed", "model", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create generic model snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o644); err != nil {
		t.Fatalf("write generic model snapshot: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "model", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	})
	if err != nil {
		t.Fatalf("marshal generic model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write generic model metadata: %v", err)
	}
}

func resolvePinnedTTSBackend(
	_ context.Context,
	request serviceedges.ModelBackendArtifactSelectionRequest,
) (serviceedges.ModelBackendArtifactSelection, error) {
	if request.Backend != "localai-vibevoice" || request.ProtocolVersion != "localai-backend-v1" {
		return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("unexpected pinned backend selection request")
	}
	return pinnedTTSBackendSelection(), nil
}

func pinnedTTSBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-fixture/localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Bytes:    22,
		SHA256:   "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
	}
}

func writeGenericBuiltinTTSBackendCache(t *testing.T, home string) {
	t.Helper()
	selection := pinnedTTSBackendSelection()
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://localai-vibevoice/release://" + urlHash
	digest := selection.SHA256
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", "backend-artifacts", ".you-content-addressed", "backend", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create generic backend snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), []byte("pinned-backend-fixture"), 0o644); err != nil {
		t.Fatalf("write generic backend snapshot: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{
			"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256,
		}},
	})
	if err != nil {
		t.Fatalf("marshal generic backend metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write generic backend metadata: %v", err)
	}
}

type joinedProtocolNegotiator struct {
	mu    sync.Mutex
	calls int
}

func (negotiator *joinedProtocolNegotiator) Negotiate(
	context.Context,
	string,
	serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	negotiator.mu.Lock()
	negotiator.calls++
	negotiator.mu.Unlock()
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: "localai-backend-v1",
		Backend:         "localai-vibevoice",
		Ready:           true,
	}, nil
}

func (negotiator *joinedProtocolNegotiator) Calls() int {
	negotiator.mu.Lock()
	defer negotiator.mu.Unlock()
	return negotiator.calls
}

type joinedCompatibilityChecker struct {
	mu      sync.Mutex
	calls   int
	failure error
}

func (checker *joinedCompatibilityChecker) Check(
	context.Context,
	serviceedges.ModelHostCompatibilityRequest,
) error {
	checker.mu.Lock()
	checker.calls++
	failure := checker.failure
	checker.mu.Unlock()
	return failure
}

func (checker *joinedCompatibilityChecker) Calls() int {
	checker.mu.Lock()
	defer checker.mu.Unlock()
	return checker.calls
}

func functionalHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return append(os.Environ(), "USERPROFILE="+home)
	}
	if runtime.GOOS == "plan9" {
		return append(os.Environ(), "home="+home)
	}
	return append(os.Environ(), "HOME="+home)
}

type recordingModelHTTPClient struct {
	mu       sync.Mutex
	calls    int
	delegate *http.Client
}

func (client *recordingModelHTTPClient) Do(request *http.Request) (*http.Response, error) {
	client.mu.Lock()
	client.calls++
	delegate := client.delegate
	client.mu.Unlock()
	if delegate == nil {
		return nil, http.ErrUseLastResponse
	}
	return delegate.Do(request)
}

func (client *recordingModelHTTPClient) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

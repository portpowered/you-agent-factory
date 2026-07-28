package root_composition_test

import (
	"bytes"
	"encoding/json"
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

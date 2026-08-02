package model_invoke_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	root "github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestProcessModelsInvokeUsesCanonicalGraphAndExactExternalEdges(t *testing.T) {
	audio := []byte("RIFF....WAVE")
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/invoke":
			var payload struct {
				OutputFile string `json:"outputFile"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := os.WriteFile(payload.OutputFile, audio, 0o644); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeReadyOmniVoiceCache(t, home)
	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, processLocalModelFactory(modelServer.URL))
	assetFiles := processModelAssetFileSystem{home: home}

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{
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
		ModelHostProcessLauncher:       &processModelLauncher{endpoint: modelServer.URL},
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	var output bytes.Buffer
	if err := process.Execute(root.Input{
		Args: []string{
			"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
			"--operation", "TTS", "--text", "hello from the process",
		},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           io.Discard,
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	}); err != nil {
		t.Fatalf("Process.Execute(models invoke) error = %v", err)
	}

	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode models invoke output: %v\n%s", err, output.String())
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" || response.Operation != "TTS" {
		t.Fatalf("response identity = %#v, want OMNIVOICE_Q4_K_M/TTS", response)
	}
	if response.Worker != "voice-local" || len(response.Content) == 0 {
		t.Fatalf("response = %#v, want voice-local content", response)
	}

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	output.Reset()
	if err := process.Execute(root.Input{
		Args: []string{
			"you", "models", "invoke", "OMNIVOICE_Q4_K_M",
			"--operation", "TTS", "--text", "write audio from the process",
			"--output", audioPath,
		},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           io.Discard,
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	}); err != nil {
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

func processLocalModelFactory(endpoint string) map[string]any {
	return map[string]any{
		"name": "factory",
		"resources": []map[string]any{{
			"name": "omnivoice-cache", "type": factorydefinitions.ResourceTypeModel,
			"capacity": 1, "model": "OMNIVOICE_Q4_K_M", "backend": "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name": "voice-local", "type": factorydefinitions.WorkerTypeModel,
			"modelProvider": "CODEX", "model": "OMNIVOICE_Q4_K_M",
			"modelLocality": factorydefinitions.ModelLocalityLocal,
			"command":       "omnivoice-llamacpp",
			"args":          []string{"--health-endpoint", endpoint},
			"resources":     []map[string]any{{"name": "omnivoice-cache", "capacity": 1}},
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

type processModelLauncher struct {
	mu       sync.Mutex
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
	defer launcher.mu.Unlock()
	return &processModelProcess{endpoint: launcher.endpoint, stopped: make(chan struct{})}, nil
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

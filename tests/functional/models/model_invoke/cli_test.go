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
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestProcessModelsInvokeUsesCanonicalGraphAndExactExternalEdges(t *testing.T) {
	audio := []byte("RIFF....WAVE")
	var backendPayload struct {
		Operation  string `json:"operation"`
		ModelName  string `json:"modelName"`
		OutputFile string `json:"outputFile"`
		Text       string `json:"text"`
	}
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/invoke":
			if err := json.NewDecoder(request.Body).Decode(&backendPayload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := os.WriteFile(backendPayload.OutputFile, audio, 0o644); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			response, err := json.Marshal(map[string]any{
				"content": []map[string]any{{
					"type":        "AUDIO",
					"slot":        "audio",
					"file":        backendPayload.OutputFile,
					"contentType": "audio/wav",
				}},
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(response)
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
	var diagnostics bytes.Buffer
	if err := process.Execute(root.Input{
		Args: []string{
			"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
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
	if diagnostics.Len() != 0 {
		t.Fatalf("models invoke JSON stderr = %q, want empty", diagnostics.String())
	}

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	output.Reset()
	diagnostics.Reset()
	if err := process.Execute(root.Input{
		Args: []string{
			"you", "models", "invoke", "OMNIVOICE_Q4_K_M",
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
	if backendPayload.Operation != "TTS" || backendPayload.ModelName != "OMNIVOICE_Q4_K_M" ||
		backendPayload.Text != "write audio from the process" || backendPayload.OutputFile == "" {
		t.Fatalf("backend TTS payload = %#v, want model, operation, text, and output file", backendPayload)
	}
	if filepath.Ext(backendPayload.OutputFile) != ".wav" {
		t.Fatalf("backend output file = %q, want .wav audio artifact", backendPayload.OutputFile)
	}
	audioInfo, err := os.Stat(audioPath)
	if err != nil {
		t.Fatalf("stat models invoke output audio: %v", err)
	}
	if !audioInfo.Mode().IsRegular() || audioInfo.Size() != int64(len(audio)) {
		t.Fatalf("models invoke output audio info = %#v, want regular file with %d bytes", audioInfo, len(audio))
	}
	written, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read models invoke audio: %v", err)
	}
	if !bytes.Equal(written, audio) {
		t.Fatalf("models invoke audio = %q, want %q", written, audio)
	}
}

func TestProcessModelsInvokeFailureKeepsStreamsSafeAndReleasesCapacity(t *testing.T) {
	audio := []byte("RIFF....WAVE")
	var backendMu sync.Mutex
	failBackend := false
	backendInvocations := 0
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
			backendMu.Lock()
			backendInvocations++
			shouldFail := failBackend
			backendMu.Unlock()
			if shouldFail {
				http.Error(w, "deterministic TTS backend failure", http.StatusInternalServerError)
				return
			}
			if err := os.WriteFile(payload.OutputFile, audio, 0o644); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			response, err := json.Marshal(map[string]any{
				"content": []map[string]any{{
					"type":        "AUDIO",
					"slot":        "audio",
					"file":        payload.OutputFile,
					"contentType": "audio/wav",
				}},
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(response)
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
	assertSuccessfulProcessTTS(t, stdout, stderr, firstAudioPath, audio)

	secondAudioPath := filepath.Join(t.TempDir(), "second.wav")
	stdout, stderr, err = executeProcessTTS(t, process, home, factoryDir, secondAudioPath, "follow-up after success")
	if err != nil {
		t.Fatalf("follow-up after successful Process.Execute(models invoke) error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	assertSuccessfulProcessTTS(t, stdout, stderr, secondAudioPath, audio)

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
	assertSuccessfulProcessTTS(t, stdout, stderr, finalAudioPath, audio)

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
			"you", "models", "invoke", "OMNIVOICE_Q4_K_M",
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
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat successful models invoke audio: %v", err)
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(wantAudio)) {
		t.Fatalf("successful models invoke audio info = %#v, want regular file with %d bytes", info, len(wantAudio))
	}
	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read successful models invoke audio: %v", err)
	}
	if !bytes.Equal(written, wantAudio) {
		t.Fatalf("successful models invoke audio = %q, want %q", written, wantAudio)
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

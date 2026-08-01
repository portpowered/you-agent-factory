package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsReadinessAssetsHostEffectsRemainInertThroughRootBuildProcess proves
// readiness, assets, and host Model edge families stay at zero during
// root.BuildProcess construction before runtime lifecycle starts.
func TestModelsReadinessAssetsHostEffectsRemainInertThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	recorder := newModelEffectRecorder()
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.totalReadiness(); got != 0 {
		t.Fatalf("readiness effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalAssets(); got != 0 {
		t.Fatalf("assets effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalHost(); got != 0 {
		t.Fatalf("host effect calls = %d during BuildProcess, want 0", got)
	}
}

// TestModelsReadinessAssetsHostActivateThroughRootBuildProcessAfterLifecycle proves
// assets pull, readiness inspection, and host startup activate through public
// Models HTTP surfaces after runtime lifecycle on a process constructed only
// through root.BuildProcess with edges.Edges effect replacement.
func TestModelsReadinessAssetsHostActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
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
	writeCachedOmniVoiceAssets(t, cacheDirectory)

	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	home := t.TempDir()
	assetFiles := functionalModelAssetFileSystem{home: home}

	dir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig(modelServer.URL))
	environment := append(os.Environ(), runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Edges: serviceedges.Edges{
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
			ModelHostHTTPClient:            modelServer.Client(),
			ModelRuntimeHTTPClient:         modelServer.Client(),
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	pull := postFunctionalJSON[factoryapi.ModelPullResponse](
		t,
		server.URL()+"/models/OMNIVOICE_Q4_K_M/pull",
		nil,
		"POST /models pull",
	)
	if pull.Outcome != factoryapi.ModelPullOutcomeALREADYPRESENT ||
		pull.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("POST /models pull = %#v, want cached READY asset pull", pull)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("asset pull made %d upstream network requests, want 0 via edges", rejectingNetwork.Calls())
	}

	model := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/OMNIVOICE_Q4_K_M")
	if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("GET /models/{name} readiness = %s, want READY", model.ManagedRuntime.ReadinessState)
	}

	responseMode := factoryapi.METADATA
	invocation := postFunctionalJSON[factoryapi.ModelInvocationResponse](
		t,
		server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations",
		factoryapi.ModelInvocationRequest{
			Operation: "TTS",
			Bindings:  localModelReadinessAssetsHostBindings(),
			Content: &factoryapi.WorkContent{
				mustFunctionalTextPart(t, "activate host through public Models invoke"),
			},
			Options: &factoryapi.ModelInvocationOptions{ResponseMode: &responseMode},
		},
		"POST /models invocations",
	)
	if invocation.ModelName != "OMNIVOICE_Q4_K_M" || invocation.Operation != "TTS" {
		t.Fatalf("POST /models invocations identity = %#v, want OMNIVOICE_Q4_K_M/TTS", invocation)
	}
	if hostLauncher.Calls() == 0 {
		t.Fatal("host process launcher calls = 0 after invoke, want host activation through edges")
	}
}

func localModelReadinessAssetsHostFactoryConfig(endpoint string) map[string]any {
	return map[string]any{
		"name": "models-readiness-assets-host",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       interfaces.ResourceTypeModel,
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          interfaces.WorkerTypeModel,
			"model":         "OMNIVOICE_Q4_K_M",
			"modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityLocal,
			"command":       "omnivoice-llamacpp",
			"args":          []string{"--health-endpoint", endpoint},
			"resources": []map[string]any{{
				"name": "omnivoice-cache", "capacity": 1,
			}},
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"contentTypes": []string{interfaces.ModelOperationContentTypeText},
					"required":     true,
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
	}
}

func writeCachedOmniVoiceAssets(t *testing.T, cacheDirectory string) {
	t.Helper()

	revision := "cached-revision"
	modelDirectory := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", revision)
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create cached model directory: %v", err)
	}
	assetBody := []byte("cached-model-asset")
	checksum := fmt.Sprintf("%x", sha256.Sum256(assetBody))
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(modelDirectory, name), assetBody, 0o644); err != nil {
			t.Fatalf("write cached model asset %s: %v", name, err)
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M",
		"revision":  revision,
		"files": []map[string]any{
			{"path": files[0], "bytes": len(assetBody), "sha256": checksum},
			{"path": files[1], "bytes": len(assetBody), "sha256": checksum},
		},
	})
	if err != nil {
		t.Fatalf("marshal cached model metadata: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", ".managed-cache.json"),
		metadata,
		0o644,
	); err != nil {
		t.Fatalf("write cached model metadata: %v", err)
	}
}

func localModelReadinessAssetsHostBindings() *[]factoryapi.WorkstationOperationBinding {
	return &[]factoryapi.WorkstationOperationBinding{{
		Slot: "text",
		Selector: &factoryapi.WorkstationOperationBindingSelector{
			Type: func() *factoryapi.ModelOperationContentType {
				value := factoryapi.ModelOperationContentTypeText
				return &value
			}(),
		},
	}}
}

func mustFunctionalTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	}); err != nil {
		t.Fatalf("build functional text content part: %v", err)
	}
	return part
}

func postFunctionalJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	response, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return result
}

type rejectingModelAssetHTTP struct {
	mu    sync.Mutex
	calls int
}

func (client *rejectingModelAssetHTTP) Do(*http.Request) (*http.Response, error) {
	client.mu.Lock()
	client.calls++
	client.mu.Unlock()
	return nil, fmt.Errorf("unexpected model asset network request")
}

func (client *rejectingModelAssetHTTP) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

type recordingModelHostLauncher struct {
	mu       sync.Mutex
	calls    int
	endpoint string
}

func (launcher *recordingModelHostLauncher) Start(
	context.Context,
	serviceedges.HostProcessStartSpec,
) (serviceedges.HostManagedProcess, error) {
	launcher.mu.Lock()
	launcher.calls++
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &functionalModelHostProcess{endpoint: endpoint, stopped: make(chan struct{})}, nil
}

func (launcher *recordingModelHostLauncher) Calls() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.calls
}

type functionalModelHostProcess struct {
	endpoint string
	stopped  chan struct{}
	once     sync.Once
}

func (process *functionalModelHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *functionalModelHostProcess) Stop(context.Context) error {
	process.once.Do(func() { close(process.stopped) })
	return nil
}
func (process *functionalModelHostProcess) Wait() error {
	<-process.stopped
	return nil
}

type functionalModelAssetFileSystem struct{ home string }

func (filesystem functionalModelAssetFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (filesystem functionalModelAssetFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
func (filesystem functionalModelAssetFileSystem) UserHomeDir() (string, error) {
	return filesystem.home, nil
}
func (filesystem functionalModelAssetFileSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}
func (filesystem functionalModelAssetFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
func (filesystem functionalModelAssetFileSystem) Remove(path string) error { return os.Remove(path) }
func (filesystem functionalModelAssetFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (filesystem functionalModelAssetFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}
func (filesystem functionalModelAssetFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}
func (filesystem functionalModelAssetFileSystem) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

package local_inference_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
	mu        sync.Mutex
	calls     int
	stops     int
	endpoint  string
	exclusive bool
	active    bool
	commands  map[string]int
}

func (launcher *recordingModelHostLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	if launcher.exclusive && launcher.active {
		launcher.mu.Unlock()
		return nil, fmt.Errorf("model host fixture: previous process is still active")
	}
	launcher.calls++
	if launcher.commands == nil {
		launcher.commands = make(map[string]int)
	}
	launcher.commands[spec.Command]++
	launcher.active = true
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &functionalModelHostProcess{
		endpoint: endpoint,
		stopped:  make(chan struct{}),
		onStop: func() {
			launcher.mu.Lock()
			launcher.active = false
			launcher.stops++
			launcher.mu.Unlock()
		},
	}, nil
}

func (launcher *recordingModelHostLauncher) CallsForCommand(command string) int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.commands[command]
}

func (launcher *recordingModelHostLauncher) Calls() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.calls
}

func (launcher *recordingModelHostLauncher) StopCalls() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.stops
}

type functionalModelHostProcess struct {
	endpoint string
	stopped  chan struct{}
	onStop   func()
	once     sync.Once
}

func (process *functionalModelHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *functionalModelHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		close(process.stopped)
		if process.onStop != nil {
			process.onStop()
		}
	})
	return nil
}
func (process *functionalModelHostProcess) Wait() error {
	<-process.stopped
	return nil
}

type functionalModelAssetFileSystem struct {
	home  string
	trace *functionalModelAssetTrace
}

type functionalModelAssetTrace struct {
	mu    sync.Mutex
	paths []string
}

func (trace *functionalModelAssetTrace) record(path string) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.paths = append(trace.paths, path)
	trace.mu.Unlock()
}

func (trace *functionalModelAssetTrace) snapshot() []string {
	if trace == nil {
		return nil
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return append([]string(nil), trace.paths...)
}

func (filesystem functionalModelAssetFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (filesystem functionalModelAssetFileSystem) Stat(path string) (os.FileInfo, error) {
	filesystem.trace.record("stat:" + path)
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
	filesystem.trace.record("read:" + path)
	return os.ReadFile(path)
}
func (filesystem functionalModelAssetFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	filesystem.trace.record("readdir:" + path)
	return os.ReadDir(path)
}
func (filesystem functionalModelAssetFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}
func (filesystem functionalModelAssetFileSystem) Open(path string) (io.ReadCloser, error) {
	filesystem.trace.record("open:" + path)
	return os.Open(path)
}

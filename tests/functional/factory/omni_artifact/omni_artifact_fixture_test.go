package omni_artifact_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func omniFactoryConfig(endpoint string) map[string]any {
	return map[string]any{
		"name": "omni-artifact-functional",
		"invocationSignature": map[string]any{
			"parameters": []map[string]any{{
				"name": "prompt", "externalName": "prompt", "required": true,
				"bindings": []map[string]any{{"kind": "POSITIONAL", "position": 1}, {"kind": "STDIN"}, {"kind": "NAMED"}},
			}},
		},
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name": "llm-cache", "type": factorydefinitions.ResourceTypeModel, "capacity": 1,
			"model": "llm", "backend": "localai-llamacpp", "loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name": "llm-worker", "type": factorydefinitions.WorkerTypeInference, "model": "llm",
			"modelProvider": "CODEX", "modelLocality": factorydefinitions.ModelLocalityLocal,
			"command": "llama-cpp", "args": []string{"--grpc-endpoint", endpoint},
			"resources": []map[string]any{{"name": "llm-cache", "capacity": 1}},
			"operations": []map[string]any{{
				"name":    models.OperationOMNI,
				"inputs":  []map[string]any{{"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true}},
				"outputs": []map[string]any{{"name": "text", "contentTypes": []string{"TEXT"}, "required": true}},
			}},
		}},
		"workstations": []map[string]any{{
			"name": "execute-llm", "type": factorydefinitions.WorkstationTypeInference, "operation": models.OperationOMNI,
			"worker": "llm-worker", "body": "Return the model result.",
			"operationBindings": []map[string]any{{
				"slot":     "prompt",
				"selector": map[string]any{"type": "TEXT"},
			}},
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

type omniProtocolFixture struct {
	mu         sync.Mutex
	response   string
	failure    error
	calls      int
	release    <-chan struct{}
	called     chan struct{}
	callOnce   sync.Once
	canceled   chan struct{}
	cancelOnce sync.Once
}

func (fixture *omniProtocolFixture) Predict(ctx context.Context, request models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	fixture.mu.Lock()
	fixture.calls++
	response := fixture.response
	failure := fixture.failure
	release := fixture.release
	called := fixture.called
	fixture.mu.Unlock()
	fixture.callOnce.Do(func() {
		if called != nil {
			close(called)
		}
	})
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			fixture.cancelOnce.Do(func() {
				if fixture.canceled != nil {
					close(fixture.canceled)
				}
			})
			return models.InvocationProtocolResponse{}, ctx.Err()
		}
	}
	if failure != nil {
		return models.InvocationProtocolResponse{}, failure
	}
	return models.InvocationProtocolResponse{Text: response}, nil
}

func (fixture *omniProtocolFixture) SetError(err error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.failure = err
}

func (fixture *omniProtocolFixture) Calls() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}

func (fixture *omniProtocolFixture) BlockUntil(release <-chan struct{}) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.release = release
}

func (fixture *omniProtocolFixture) WaitForCall(t *testing.T) {
	t.Helper()
	fixture.mu.Lock()
	called := fixture.called
	fixture.mu.Unlock()
	if called == nil {
		t.Fatal("OMNI fixture call observer is not initialized")
	}
	select {
	case <-called:
	case <-time.After(omniFactoryFunctionalTimeout):
		t.Fatal("timed out waiting for OMNI protocol call")
	}
}

func (fixture *omniProtocolFixture) WaitForCancellation(t *testing.T) {
	t.Helper()
	fixture.mu.Lock()
	canceled := fixture.canceled
	fixture.mu.Unlock()
	if canceled == nil {
		t.Fatal("OMNI fixture cancellation observer is not initialized")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OMNI protocol context cancellation")
	}
}

type omniProtocolRouter struct {
	mu     sync.Mutex
	routes map[string]*omniProtocolFixture
}

func (router *omniProtocolRouter) Register(prompt string, fixture *omniProtocolFixture) {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.routes[prompt] = fixture
}

func (router *omniProtocolRouter) Predict(ctx context.Context, request models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error) {
	router.mu.Lock()
	var fixture *omniProtocolFixture
	for prompt, candidate := range router.routes {
		if request.Prompt == prompt || strings.Contains(request.Prompt, prompt) {
			fixture = candidate
			break
		}
	}
	router.mu.Unlock()
	if fixture == nil {
		return models.InvocationProtocolResponse{}, fmt.Errorf("no OMNI fixture route for prompt %q", request.Prompt)
	}
	return fixture.Predict(ctx, request)
}

type modelHostLauncher struct {
	mu             sync.Mutex
	endpoint       string
	starts         int
	stops          int
	startsByTarget map[string]int
	stopsByTarget  map[string]int
}

func (launcher *modelHostLauncher) Start(_ context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	launcher.starts++
	endpoint := strings.TrimSpace(spec.HealthEndpoint)
	if endpoint == "" {
		endpoint = launcher.endpoint
	}
	if launcher.startsByTarget == nil {
		launcher.startsByTarget = make(map[string]int)
	}
	launcher.startsByTarget[endpoint]++
	launcher.mu.Unlock()
	return &modelHostProcess{endpoint: endpoint, launcher: launcher, stopped: make(chan struct{})}, nil
}

func (launcher *modelHostLauncher) Counts() (int, int) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts, launcher.stops
}

type modelHostProcess struct {
	endpoint string
	launcher *modelHostLauncher
	stopped  chan struct{}
	once     sync.Once
}

func (process *modelHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *modelHostProcess) Wait() error {
	<-process.stopped
	return nil
}
func (process *modelHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		close(process.stopped)
		process.launcher.mu.Lock()
		process.launcher.stops++
		if process.launcher.stopsByTarget == nil {
			process.launcher.stopsByTarget = make(map[string]int)
		}
		process.launcher.stopsByTarget[process.endpoint]++
		process.launcher.mu.Unlock()
	})
	return nil
}

type modelHostProtocolNegotiator struct{}

func (modelHostProtocolNegotiator) Negotiate(_ context.Context, _ string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: "localai-backend-v1", Backend: request.Backend, Ready: true}, nil
}

type modelHostCompatibilityChecker struct{}

func (modelHostCompatibilityChecker) Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error {
	return nil
}

type rejectingAssetHTTP struct{}

func (rejectingAssetHTTP) Do(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected model asset network request")
}

type modelAssetFileSystem struct{ home string }

func (filesystem modelAssetFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (filesystem modelAssetFileSystem) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
func (filesystem modelAssetFileSystem) UserHomeDir() (string, error)          { return filesystem.home, nil }
func (filesystem modelAssetFileSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}
func (filesystem modelAssetFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
func (filesystem modelAssetFileSystem) Remove(path string) error { return os.Remove(path) }
func (filesystem modelAssetFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (filesystem modelAssetFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}
func (filesystem modelAssetFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}
func (filesystem modelAssetFileSystem) Open(path string) (io.ReadCloser, error) { return os.Open(path) }

func writeBuiltinModelCache(t *testing.T, home string) {
	t.Helper()
	if err := writeBuiltinModelCacheAt(home); err != nil {
		t.Fatalf("write model cache: %v", err)
	}
}

func writeBuiltinModelCacheAt(home string) error {
	name := "gemma-4-E4B-it-Q4_K_M.gguf"
	body := []byte("functional model fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", omniModelSource, name, len(body), digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", ".you-content-addressed", "model", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		return fmt.Errorf("create model snapshot: %w", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o644); err != nil {
		return fmt.Errorf("write model snapshot: %w", err)
	}
	metadata := map[string]any{
		"kind": "model", "identity": identity, "source": omniModelSource, "sourceKey": omniModelSource,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal model metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), data, 0o644); err != nil {
		return fmt.Errorf("write model metadata: %w", err)
	}
	return nil
}

func llamaBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Bytes:    28,
		SHA256:   "9285e7ffc76aaadf4dfcc6b2de5e23c6b01d4e7068e8f2dd65673626cc5de4ed",
	}
}

func writeBackendCache(t *testing.T, home string, selection serviceedges.ModelBackendArtifactSelection) {
	t.Helper()
	if err := writeBackendCacheAt(home, selection); err != nil {
		t.Fatalf("write backend cache: %v", err)
	}
}

func writeBackendCacheAt(home string, selection serviceedges.ModelBackendArtifactSelection) error {
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://localai-llamacpp/release://" + urlHash
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, selection.SHA256)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", "backend-artifacts", ".you-content-addressed", "backend", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		return fmt.Errorf("create backend snapshot: %w", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), []byte("localai-llamacpp/linux-amd64"), 0o644); err != nil {
		return fmt.Errorf("write backend snapshot: %w", err)
	}
	metadata := map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256}},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal backend metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), data, 0o644); err != nil {
		return fmt.Errorf("write backend metadata: %w", err)
	}
	return nil
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

func requiredString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointer(value string) *string {
	return &value
}

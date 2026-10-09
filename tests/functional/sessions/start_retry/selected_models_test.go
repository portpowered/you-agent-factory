package start_retry_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const selectedModelsRevision = "0123456789abcdef0123456789abcdef01234567"
const selectedModelsBackend = "localai-llamacpp"

type selectedModelsScenario struct {
	initialOpeningScenario
	peerHome   string
	backend    serviceedges.ModelBackendArtifactSelection
	mu         sync.Mutex
	starts     map[string]int
	negotiated map[string]int
}

func newSelectedModelsScenario(t *testing.T) *selectedModelsScenario {
	t.Helper()
	scenario := &selectedModelsScenario{initialOpeningScenario: initialOpeningScenarioWithConfig(t, initialOpeningFactoryConfig()),
		peerHome: t.TempDir(), starts: make(map[string]int), negotiated: make(map[string]int)}
	body := []byte("selected backend fixture")
	scenario.backend = serviceedges.ModelBackendArtifactSelection{Name: "fixture-backend.tar.gz", Location: "https://github.com/selected/fixture/releases/download/v1/fixture-backend.tar.gz", Bytes: int64(len(body)), SHA256: fmt.Sprintf("%x", sha256.Sum256(body))}
	for _, entry := range []struct{ dir, home, id string }{{scenario.candidateDir, scenario.home, scenario.candidateID}, {scenario.peerDir, scenario.peerHome, scenario.peerID}} {
		config := selectedModelsFactoryConfig(entry.id)
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(entry.dir, "factory.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		support.WriteWorkstationConfig(t, entry.dir, "process", "---\ntype: INFERENCE_RUN\n---\nReturn the model result.\n")
		source := "hf://selected/" + entry.id
		settings, err := json.Marshal(map[string]any{"models": map[string]any{"llm": map[string]any{"source": source}}})
		if err != nil {
			t.Fatal(err)
		}
		settingsPath := filepath.Join(entry.home, ".you-agent-factory", "config.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
			t.Fatal(err)
		}
		cache := filepath.Join(entry.home, ".agent-factory", "models")
		seedSelectedModelAsset(t, cache, "model", source+"@"+selectedModelsRevision, "selected.gguf", []byte(entry.id))
		urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(scenario.backend.Location)))
		seedSelectedModelAsset(t, filepath.Join(cache, "backend-artifacts"), "backend", "backend://"+selectedModelsBackend+"/release://"+urlHash, scenario.backend.Name, body)
	}
	return scenario
}

func selectedModelsFactoryConfig(id string) map[string]any {
	config := initialOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	config["resources"] = []map[string]any{{"name": "llm-cache", "type": "MODEL", "capacity": 1, "model": "llm", "backend": selectedModelsBackend, "loadPolicy": "ON_DEMAND"}}
	config["workers"] = []map[string]any{{"name": "worker-a", "type": "INFERENCE_WORKER", "model": "llm", "modelProvider": "CODEX", "modelLocality": "LOCAL", "command": "llama-cpp",
		"args": []string{"--grpc-endpoint", "http://" + id + ".invalid"}, "resources": []map[string]any{{"name": "llm-cache", "capacity": 1}},
		"operations": []map[string]any{{"name": "OMNI", "inputs": []map[string]any{{"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true}}, "outputs": []map[string]any{{"name": "text", "contentTypes": []string{"TEXT"}, "required": true}}}}}}
	station := config["workstations"].([]map[string]any)[0]
	station["type"], station["operation"] = "INFERENCE_RUN", "OMNI"
	station["operationBindings"] = []map[string]any{{"slot": "prompt", "selector": map[string]any{"type": "TEXT"}}}
	return config
}

// Cache fixtures model installed customer assets. All runtime reads still go
// through production Models; unexpected downloads fail at the exact HTTP edge.
func seedSelectedModelAsset(t *testing.T, cache, kind, source, name string, body []byte) {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("%s|%s|%s:%d:%s", kind, source, name, len(body), digest)
	snapshot := filepath.Join(cache, ".you-content-addressed", kind, fmt.Sprintf("%x", sha256.Sum256([]byte(identity))))
	if err := os.MkdirAll(snapshot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{"kind": kind, "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (scenario *selectedModelsScenario) Start(_ context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(spec.HealthEndpoint, "http://"), ".invalid")
	home := scenario.home
	if id == scenario.peerID {
		home = scenario.peerHome
	} else if id != scenario.candidateID {
		return nil, fmt.Errorf("unselected model endpoint %q", spec.HealthEndpoint)
	}
	body, err := os.ReadFile(spec.ModelPath)
	if err != nil || string(body) != id || !strings.HasPrefix(spec.ModelPath, home+string(filepath.Separator)) {
		return nil, fmt.Errorf("selected endpoint %s received another scope's model asset", id)
	}
	scenario.mu.Lock()
	scenario.starts[id]++
	scenario.mu.Unlock()
	return &selectedModelsProcess{endpoint: spec.HealthEndpoint, stopped: make(chan struct{})}, nil
}

type selectedModelsProcess struct {
	endpoint string
	stopped  chan struct{}
	once     sync.Once
}

func (process *selectedModelsProcess) HealthEndpoint() string { return process.endpoint }
func (process *selectedModelsProcess) Wait() error            { <-process.stopped; return nil }
func (process *selectedModelsProcess) Stop(context.Context) error {
	process.once.Do(func() { close(process.stopped) })
	return nil
}

func (scenario *selectedModelsScenario) Negotiate(_ context.Context, endpoint string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(endpoint, "http://"), ".invalid")
	body, err := os.ReadFile(request.ModelPath)
	if err != nil || string(body) != id || request.ModelName != "llm" || request.Revision != selectedModelsRevision {
		return serviceedges.ModelHostProtocolNegotiationResult{}, fmt.Errorf("protocol model selection differs from selected endpoint %s", id)
	}
	scenario.mu.Lock()
	scenario.negotiated[id]++
	scenario.mu.Unlock()
	return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: request.ProtocolVersion, Backend: request.Backend, Ready: true}, nil
}

func (*selectedModelsScenario) Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error {
	return nil
}
func (*selectedModelsScenario) Predict(_ context.Context, request models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error) {
	id, _, _ := strings.Cut(request.Prompt, " selected Work")
	return models.InvocationProtocolResponse{Text: id + " COMPLETE"}, nil
}
func (scenario *selectedModelsScenario) Do(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/health" {
		return nil, fmt.Errorf("unexpected model network/download request %s", request.URL)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
}

func testSelectedModelsAcquisition(t *testing.T, sessions factorysessions.Service, scenario *selectedModelsScenario) {
	t.Helper()
	request := scenario.request()
	request.RuntimeSelection.ModelCacheDirectory = filepath.Join(scenario.home, ".agent-factory", "models")
	startInitialOpeningSession(t, sessions, request)
	if err := selectedProviderInvoke(t.Context(), sessions, scenario.candidateID); err != nil {
		history := initialOpeningHistory(t, sessions, scenario.candidateID)
		data, _ := json.Marshal(history.History[len(history.History)-2:])
		t.Fatalf("%v; public history: %s", err, data)
	}
	candidateHistory := initialOpeningHistory(t, sessions, scenario.candidateID)
	peer := scenario.request()
	peer.SessionID, peer.FolderPath = scenario.peerID, scenario.peerDir
	peer.RuntimeSelection.DefinitionSourcePath = filepath.Join(scenario.peerDir, "factory.json")
	peer.RuntimeSelection.ExecutionBaseDir, peer.RuntimeSelection.RuntimeInstanceID = scenario.peerDir, uuid.NewString()
	peer.RuntimeSelection.SystemConfigHome = scenario.peerHome
	peer.RuntimeSelection.ModelCacheDirectory = filepath.Join(scenario.peerHome, ".agent-factory", "models")
	startInitialOpeningSession(t, sessions, peer)
	for _, id := range []string{scenario.peerID, scenario.candidateID} {
		if err := selectedProviderInvoke(t.Context(), sessions, id); err != nil {
			t.Fatal(err)
		}
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.candidateID, candidateHistory)
	for _, id := range []string{scenario.candidateID, scenario.peerID} {
		assertSelectedModelsObservations(t, sessions, id)
	}
	scenario.mu.Lock()
	for _, id := range []string{scenario.candidateID, scenario.peerID} {
		if scenario.starts[id] != 1 || scenario.negotiated[id] != 1 {
			t.Errorf("model selection %s starts=%d negotiations=%d, want one per distinct scope", id, scenario.starts[id], scenario.negotiated[id])
		}
	}
	scenario.mu.Unlock()
	peerHistory := initialOpeningHistory(t, sessions, scenario.peerID)
	closeInitialOpeningSession(t, sessions, scenario.candidateID)
	if err := selectedProviderInvoke(t.Context(), sessions, scenario.peerID); err != nil {
		t.Fatal(err)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
}

func assertSelectedModelsObservations(t *testing.T, sessions factorysessions.Service, id string) {
	t.Helper()
	history := initialOpeningHistory(t, sessions, id)
	seen := make(map[string]int)
	for _, event := range history.History {
		if event.Type != "MODEL_REQUEST" && event.Type != "MODEL_RESPONSE" {
			continue
		}
		if event.Context.SessionID == nil || *event.Context.SessionID != id || event.Context.DispatchID == nil || event.Context.RequestID == nil {
			t.Fatalf("model observation %s lost its session/dispatch/request attribution: %#v", event.Type, event.Context)
		}
		seen[string(event.Type)]++
	}
	if seen["MODEL_REQUEST"] == 0 || seen["MODEL_REQUEST"] != seen["MODEL_RESPONSE"] {
		t.Fatalf("session %s model observations = %v, want paired scoped request/result events", id, seen)
	}
}

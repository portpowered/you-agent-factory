package composition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const parallelConcurrentDispatchWorkflow = `return (async function () {
  const results = await parallel([
    { prompt: "summarize alpha", label: "child-alpha" },
    { prompt: "summarize beta", label: "child-beta" },
  ]);
  return { results };
})();`

// TestJavaScriptParallelDispatchesChildrenConcurrently proves JavaScript parallel
// keeps more than one external child call in flight at the same time through the
// public Factory Session and dispatch surfaces, using controllable provider edges
// instead of wall-clock sleeps to observe concurrency.
func TestJavaScriptParallelDispatchesChildrenConcurrently(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, parallelCompositionFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	homeDir := writeParallelCompositionGlobalConfig(t)

	provider := newGatedParallelChildProvider()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env: append(os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Edges: serviceedges.Edges{ProviderOverride: provider},
	})
	baseURL := strings.TrimSuffix(server.URL(), "/")

	started := startParallelCompositionWorkflowAsync(t, baseURL)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}

	waitForParallelCompositionInFlightDispatches(t, baseURL, sessionID, 2, 5*time.Second)
	provider.releaseAll()

	completed := waitForParallelCompositionSessionStatus(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		10*time.Second,
	)
	if completed.ResultSummary == nil ||
		completed.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", completed.ResultSummary)
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/dispatches",
	)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 public child dispatches", len(dispatches.Dispatches))
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
	}
	if provider.peakActive() < 2 {
		t.Fatalf("provider peak active child calls = %d, want at least 2 concurrent external calls", provider.peakActive())
	}
}

func parallelCompositionFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeParallelCompositionGlobalConfig(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "openai", "workerModel": "default-model"}
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	return homeDir
}

func startParallelCompositionWorkflowAsync(
	t *testing.T,
	baseURL string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "parallel-composition-concurrent-dispatch",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   parallelConcurrentDispatchWorkflow,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal parallel workflow request: %v", err)
	}

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		baseURL+"/factory-sessions/async",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build async parallel workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start async parallel workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start async parallel workflow status = %d: %s", response.StatusCode, body.String())
	}

	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode async parallel workflow response: %v", err)
	}
	return started
}

func waitForParallelCompositionInFlightDispatches(
	t *testing.T,
	baseURL, sessionID string,
	want int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readParallelCompositionSession(t, baseURL, sessionID)
		if session.Progress != nil &&
			intValueOrZero(session.Progress.InFlightDispatches) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := readParallelCompositionSession(t, baseURL, sessionID)
	t.Fatalf(
		"session %s inFlightDispatches = %#v, want at least %d within %s",
		sessionID,
		session.Progress,
		want,
		timeout,
	)
}

func waitForParallelCompositionSessionStatus(
	t *testing.T,
	baseURL, sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readParallelCompositionSession(t, baseURL, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := readParallelCompositionSession(t, baseURL, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func readParallelCompositionSession(
	t *testing.T,
	baseURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	return support.GetJSON[factoryapi.FactorySessionDurableReadModel](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
}

func intValueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

type gatedParallelChildProvider struct {
	mu         sync.Mutex
	active     int
	peak       int
	release    chan struct{}
	releaseOnce sync.Once
}

func newGatedParallelChildProvider() *gatedParallelChildProvider {
	return &gatedParallelChildProvider{
		release: make(chan struct{}),
	}
}

func (p *gatedParallelChildProvider) releaseAll() {
	p.releaseOnce.Do(func() {
		close(p.release)
	})
}

func (p *gatedParallelChildProvider) peakActive() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

func (p *gatedParallelChildProvider) Infer(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	p.mu.Unlock()

	select {
	case <-p.release:
	case <-ctx.Done():
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
		return workerexecution.InferenceResponse{}, ctx.Err()
	}

	p.mu.Lock()
	p.active--
	p.mu.Unlock()

	label := parallelChildLabelFromRequest(req)
	return workerexecution.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"parallel-child:%s:COMPLETE","label":%q}`, label, label),
	}, nil
}

func parallelChildLabelFromRequest(req workerexecution.ProviderInferenceRequest) string {
	for _, token := range req.InputTokens {
		payload, ok := token.(map[string]any)
		if !ok {
			continue
		}
		color, ok := payload["color"].(map[string]any)
		if !ok {
			continue
		}
		tags, ok := color["tags"].(map[string]any)
		if !ok {
			continue
		}
		if label, ok := tags["label"].(string); ok && label != "" {
			return label
		}
	}
	message := strings.TrimSpace(req.UserMessage)
	if message == "" {
		return "child"
	}
	return message
}

var _ workerprovider.Provider = (*gatedParallelChildProvider)(nil)

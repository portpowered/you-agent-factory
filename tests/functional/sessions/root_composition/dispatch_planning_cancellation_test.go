package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const dispatchPlanningCancellationWorkflow = `return (async function () {
  await agent.run({
    prompt: "hold this dispatch until the Factory Session is cancelled",
    label: "dispatch-planning-cancellation",
  });
  return "unreachable after cancellation";
})();`

// TestFactoryRuntimeDispatchPlanningCancellationReachesPublishedWorkerThroughPublicDurableControl
// holds one JavaScript child dispatch inside the provider boundary, then uses
// the public durable-session cancel control to stop the runtime. The provider
// observes the session-qualified Worker Session identity and context
// cancellation directly, while the durable dispatch listing proves the
// dispatch was RUNNING before the control (published, but with no result).
func TestFactoryRuntimeDispatchPlanningCancellationReachesPublishedWorkerThroughPublicDurableControl(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	provider := newDispatchPlanningCancellationProvider()
	dir := support.ScaffoldFactory(t, dispatchPlanningCancellationFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderOverride: provider},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startDispatchPlanningCancellationSession(t, server.URL())
	if started.SessionId == "" {
		t.Fatalf("async Factory Session response has empty session id: %#v", started)
	}

	request := provider.waitStarted(t)
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("durable dispatch count = %d, want one published child dispatch: %#v", len(dispatches.Dispatches), dispatches.Dispatches)
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.Status != factoryapi.FactoryDispatchStatusRUNNING {
		t.Fatalf("durable dispatch status = %q, want RUNNING before cancellation", dispatch.Status)
	}
	if dispatch.Id == "" {
		t.Fatal("durable dispatch ID is empty")
	}
	wantWorkerSessionID := started.SessionId + "/" + dispatch.Id
	if request.Correlation.DispatchID != wantWorkerSessionID {
		t.Fatalf("provider worker-session ID = %q, want session-qualified public dispatch ID %q", request.Correlation.DispatchID, wantWorkerSessionID)
	}

	cancel := postDispatchPlanningCancellation(t, server.URL(), started.SessionId, "dispatch-planning-cancel-once")
	if cancel.Operation != factoryapi.FactorySessionLifecycleControlKindCancel ||
		cancel.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		cancel.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		t.Fatalf("cancel response = %#v, want accepted CANCELING control", cancel)
	}

	cancelled := provider.waitCancelled(t)
	if cancelled.Correlation.DispatchID != wantWorkerSessionID {
		t.Fatalf("cancelled provider worker-session ID = %q, want %q", cancelled.Correlation.DispatchID, wantWorkerSessionID)
	}
	if provider.cancellationCount() != 1 {
		t.Fatalf("provider cancellation observations = %d, want one", provider.cancellationCount())
	}

	// A second supported stop request reaches the runtime while the first
	// cancellation is already in flight. The public lifecycle state makes it a
	// deterministic no-op, and the provider edge remains canceled exactly once.
	replayed := postDispatchPlanningCancellation(t, server.URL(), started.SessionId, "dispatch-planning-cancel-twice")
	if replayed.Operation != factoryapi.FactorySessionLifecycleControlKindCancel ||
		replayed.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("repeated cancel response = %#v, want NO_OP while cancellation is in flight", replayed)
	}
	if provider.cancellationCount() != 1 {
		t.Fatalf("provider cancellation observations after replay = %d, want one", provider.cancellationCount())
	}
}

func dispatchPlanningCancellationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "dispatch-planning-cancellation",
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
		"workers": []map[string]string{{"name": "worker-a"}},
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

func startDispatchPlanningCancellationSession(
	t *testing.T,
	baseURL string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "dispatch-planning-cancellation-session",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   dispatchPlanningCancellationWorkflow,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal dispatch-planning cancellation request: %v", err)
	}

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/async"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build dispatch-planning cancellation request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start dispatch-planning cancellation session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start dispatch-planning cancellation session status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode dispatch-planning cancellation session response: %v", err)
	}
	return started
}

func postDispatchPlanningCancellation(
	t *testing.T,
	baseURL string,
	sessionID string,
	requestID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	requestBody, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{RequestId: &requestID})
	if err != nil {
		t.Fatalf("marshal dispatch-planning cancellation control: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/cancel"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("build dispatch-planning cancellation control: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("cancel dispatch-planning Factory Session: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read dispatch-planning cancellation response: %v", err)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		t.Fatalf("cancel dispatch-planning Factory Session status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode dispatch-planning cancellation response: %v", err)
	}
	return decoded
}

type dispatchPlanningCancellationProvider struct {
	testutil.NativeProvider
	started   chan providers.ExecuteRequest
	cancelled chan providers.ExecuteRequest

	mu            sync.Mutex
	cancellations int
}

func newDispatchPlanningCancellationProvider() *dispatchPlanningCancellationProvider {
	provider := &dispatchPlanningCancellationProvider{
		started:   make(chan providers.ExecuteRequest, 1),
		cancelled: make(chan providers.ExecuteRequest, 1),
	}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (p *dispatchPlanningCancellationProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	select {
	case p.started <- request:
	case <-ctx.Done():
		return providers.ExecuteResult{}, ctx.Err()
	}

	<-ctx.Done()
	p.mu.Lock()
	p.cancellations++
	p.mu.Unlock()
	p.cancelled <- request
	return providers.ExecuteResult{}, ctx.Err()
}

func (p *dispatchPlanningCancellationProvider) waitStarted(t *testing.T) providers.ExecuteRequest {
	t.Helper()
	select {
	case request := <-p.started:
		return request
	case <-t.Context().Done():
		t.Fatalf("provider did not observe a published dispatch before test cancellation: %v", t.Context().Err())
		return providers.ExecuteRequest{}
	}
}

func (p *dispatchPlanningCancellationProvider) waitCancelled(t *testing.T) providers.ExecuteRequest {
	t.Helper()
	select {
	case request := <-p.cancelled:
		return request
	case <-t.Context().Done():
		t.Fatalf("provider did not observe deterministic dispatch cancellation: %v", t.Context().Err())
		return providers.ExecuteRequest{}
	}
}

func (p *dispatchPlanningCancellationProvider) cancellationCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cancellations
}

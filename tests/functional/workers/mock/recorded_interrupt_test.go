package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// This direct customer journey owns its host and gate. All native execution
// is replaced by a denying command edge before the host starts.
func TestRecordedInterruptPreservesMockExecutionPolicy(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, map[string]any{"workers": []map[string]string{{"name": "worker"}}})
	support.WriteAgentConfig(t, dir, "worker", "---\ntype: MODEL_WORKER\nmodelProvider: codex\nmodel: mock-model\n---\n")
	gate := support.NewMockWorkerGate(t)
	gateConfig := gate.Config(30 * time.Second)
	tokens := int64(17)
	deny := &interruptDenyNative{}
	environment := sharedWorkersMockEnvironment(t, writeSharedWorkersMockOperatorHome(t))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env: environment,
		MockWorkersConfig: &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
			ID: "unconditional", RunType: workers.MockWorkerRunTypeAccept,
			GateConfig: gateConfig, Usage: &workers.MockWorkerUsageConfig{Provider: "codex", Model: "mock-model", InputTokens: &tokens},
		}}},
		Edges: serviceedges.Edges{ProviderCommandRunner: deny, ScriptCommandRunner: deny},
	})
	support.WaitForRuntimeIdle(t, server.URL(), 10*time.Second)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	execution := map[string]any{
		"workstationName": workers.ProviderInvocationRoute, "workerType": "processor", "runnerId": "codex",
		"executorProvider": "codex", "modelProvider": "codex", "model": "mock-model", "reasoningEffort": "high",
		"workingDirectory": dir, "workingDirectoryAuthored": true, "userMessage": "source message",
		"dispatch": map[string]any{"dispatchId": "mock-source-dispatch", "workstationName": workers.ProviderInvocationRoute, "workerType": "processor", "execution": map[string]any{"workIds": []string{"mock-work"}}},
	}
	interruptPost(t, ctx, server.URL()+"/worker-sessions", map[string]any{
		"requestId": "mock-source-request", "workerSessionId": "mock-source", "execution": execution,
	}, http.StatusAccepted)
	gate.WaitForArrival(t, 10*time.Second)
	if err := os.Remove(gateConfig.ArrivedFile); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"requestId": "mock-interrupt", "successorWorkerSessionId": "mock-successor", "replacementMessage": "replacement message", "resumeMode": "recorded"}
	cli := support.FakeInputs(ctx, []string{"you", "--remote", "--server", server.URL(), "--json", "worker-sessions", "interrupt", "mock-source",
		"--request-id", "mock-interrupt", "--successor-worker-session-id", "mock-successor", "--replacement-message", "replacement message", "--resume-mode", "recorded", "--async"})
	cli.Input.Env, cli.Input.WorkingDirectory = environment, dir
	if err := server.Execute(t, cli.Input); err != nil {
		t.Fatalf("CLI recorded interrupt: %v, stdout=%s stderr=%s", err, cli.Stdout(), cli.Stderr())
	}
	var first factoryapi.WorkerSessionInterruptResponse
	if err := json.Unmarshal([]byte(cli.Stdout()), &first); err != nil || !first.Accepted {
		t.Fatalf("CLI admission = %+v, %v", first, err)
	}
	gate.WaitForArrival(t, 10*time.Second)
	source := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/mock-source")
	successor := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/mock-successor")
	if source.State != "CANCELED" || successor.State != "RUNNING" || source.SuccessorWorkerSessionId == nil || *source.SuccessorWorkerSessionId != "mock-successor" || successor.PredecessorWorkerSessionId == nil || *successor.PredecessorWorkerSessionId != "mock-source" {
		t.Fatalf("stop/order/lineage = source %+v, successor %+v", source, successor)
	}
	if successor.Model == nil || *successor.Model != "mock-model" || successor.ReasoningEffort == nil || *successor.ReasoningEffort != "high" {
		t.Fatalf("successor lost settings: %+v", successor)
	}
	checkMockInterruptReplay(t, ctx, server.URL(), request, first)
	checkMockInterruptCompletion(t, server.URL(), gate, source, tokens, deny)
}

func checkMockInterruptReplay(t *testing.T, ctx context.Context, baseURL string, request map[string]any, first factoryapi.WorkerSessionInterruptResponse) {
	t.Helper()
	endpoint := baseURL + "/worker-sessions/mock-source/interrupt"
	replay := interruptPost(t, ctx, endpoint, request, http.StatusAccepted)
	var replayed factoryapi.WorkerSessionInterruptResponse
	if err := json.Unmarshal(replay, &replayed); err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("HTTP replay changed CLI admission result: %+v / %+v, %v", first, replayed, err)
	}
	t.Run("concurrent replay", func(t *testing.T) {
		for index := range 2 {
			t.Run(strconv.Itoa(index), func(t *testing.T) {
				t.Parallel()
				var result factoryapi.WorkerSessionInterruptResponse
				body := interruptPost(t, ctx, endpoint, request, http.StatusAccepted)
				if err := json.Unmarshal(body, &result); err != nil || !reflect.DeepEqual(first, result) {
					t.Fatalf("concurrent replay = %+v, %v", result, err)
				}
			})
		}
	})
	for key, changed := range map[string]string{"replacementMessage": "changed message", "resumeMode": "provider", "successorWorkerSessionId": "different-successor"} {
		original := request[key]
		request[key] = changed
		interruptPost(t, ctx, endpoint, request, http.StatusConflict)
		request[key] = original
	}
	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, baseURL+"/worker-sessions?scope=direct")
	if len(listed.Sessions) != 2 {
		t.Fatalf("direct sessions after replay/conflict = %+v, want one source and one successor", listed.Sessions)
	}
}

func checkMockInterruptCompletion(t *testing.T, baseURL string, gate *support.MockWorkerGate, source factoryapi.WorkerSessionObservation, tokens int64, deny *interruptDenyNative) {
	t.Helper()
	gate.Release()
	final, err := support.WaitForObservation(10*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
		return support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/mock-successor"), nil
	}, func(observation factoryapi.WorkerSessionObservation) bool {
		return mockInterruptStateUsageReady(observation, "COMPLETED", tokens)
	})
	if err != nil || final.State != "COMPLETED" || final.TokenUsage == nil || final.TokenUsage.InputTokens == nil || int64(*final.TokenUsage.InputTokens) != tokens {
		t.Fatalf("successor completion/usage = %+v, %v", final, err)
	}
	after := support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/mock-source")
	if !reflect.DeepEqual(after.TokenUsage, source.TokenUsage) {
		t.Fatalf("successor changed source usage: before %+v, after %+v", source.TokenUsage, after.TokenUsage)
	}
	if got := deny.attempts.Load(); got != 0 {
		t.Fatalf("native launch attempts = %d, want zero", got)
	}
}

// Captured usage can lag live terminal publication. Keep observing the public
// contract until both are present, while surfacing FAILED immediately.
func mockInterruptStateUsageReady(observation factoryapi.WorkerSessionObservation, state string, tokens int64) bool {
	return observation.State == "FAILED" || string(observation.State) == state &&
		observation.TokenUsage != nil && observation.TokenUsage.InputTokens != nil && int64(*observation.TokenUsage.InputTokens) == tokens
}

type interruptDenyNative struct{ attempts atomic.Int64 }

func (deny *interruptDenyNative) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	deny.attempts.Add(1)
	return platformprocess.CommandResult{}, errors.New("native execution denied by recorded mock fixture")
}

func interruptPost(t testing.TB, ctx context.Context, endpoint string, payload any, wantStatus int) []byte {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != wantStatus {
		t.Fatalf("POST status = %d, want %d: %s, %v", response.StatusCode, wantStatus, body, err)
	}
	return body
}

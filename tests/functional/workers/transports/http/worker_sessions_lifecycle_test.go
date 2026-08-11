package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestWorkerSessionHTTPDisconnectKeepsAdmittedWorkerAlive(t *testing.T) {
	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	server := startDirectWorkerSessionServer(t, runner)

	ctx, cancel := context.WithCancel(context.Background())
	response := postDirectWorkerSession(t, ctx, server.URL(), "disconnect-request", "disconnect-session", "disconnect-dispatch")
	response.Body.Close()
	cancel()

	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /worker-sessions status = %d, want 202", response.StatusCode)
	}
	if err := runner.waitStarted(); err != nil {
		t.Fatal(err)
	}
	if runner.wasCanceled() {
		t.Fatal("admitted worker was canceled when the submitting context closed")
	}

	close(gate)
	if err := runner.waitCompleted(); err != nil {
		t.Fatal(err)
	}
	if runner.wasCanceled() {
		t.Fatal("admitted worker was canceled before its gated completion")
	}
	replay := postDirectWorkerSession(t, context.Background(), server.URL(), "disconnect-request", "disconnect-session", "disconnect-dispatch")
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusAccepted {
		t.Fatalf("same-key replay status = %d, want 202", replay.StatusCode)
	}
	var replayPayload factoryapi.WorkerSessionStartResponse
	if err := json.NewDecoder(replay.Body).Decode(&replayPayload); err != nil {
		t.Fatalf("decode same-key replay: %v", err)
	}
	if replayPayload.WorkerSessionId != "disconnect-session" || replayPayload.RequestId != "disconnect-request" {
		t.Fatalf("same-key replay = %#v, want original accepted identity", replayPayload)
	}
	if runner.callCount() != 1 {
		t.Fatalf("worker command calls = %d, want one after disconnect and replay", runner.callCount())
	}
	functionalevidence.Covers(t, "rest/startWorkerSession")
}

func TestWorkerSessionHTTPShutdownJoinsAdmittedWorker(t *testing.T) {
	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	server := startDirectWorkerSessionServer(t, runner)

	response := postDirectWorkerSession(t, context.Background(), server.URL(), "shutdown-request", "shutdown-session", "shutdown-dispatch")
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /worker-sessions status = %d, want 202", response.StatusCode)
	}
	if err := runner.waitStarted(); err != nil {
		t.Fatal(err)
	}

	server.Stop(t)
	if !runner.wasCanceled() {
		t.Fatal("server shutdown did not cancel the admitted worker")
	}
	if runner.callCount() != 1 {
		t.Fatalf("worker command calls = %d, want one during joined shutdown", runner.callCount())
	}
}

func startDirectWorkerSessionServer(t *testing.T, runner platformprocess.CommandRunner) *support.FunctionalAPIServer {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "direct-worker-lifecycle")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		MockWorkersConfig:         directWorkerMockConfig(),
		WaitForServiceModeRuntime: true,
		Edges:                     serviceEdgesWithWorkerGate(runner),
	})
}

func directWorkerMockConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName:      "processor",
		WorkstationName: "process",
		RunType:         workers.MockWorkerRunTypeScript,
		ScriptConfig:    &workers.MockWorkerScriptConfig{Command: "functional-gated-worker"},
	}}}
}

func serviceEdgesWithWorkerGate(runner platformprocess.CommandRunner) serviceedges.Edges {
	return serviceedges.Edges{ProviderCommandRunner: runner, ScriptCommandRunner: runner}
}

func postDirectWorkerSession(
	t *testing.T,
	ctx context.Context,
	baseURL, requestID, sessionID, dispatchID string,
) *http.Response {
	t.Helper()
	payload := factoryapi.WorkerSessionStartRequest{
		RequestId:       requestID,
		WorkerSessionId: sessionID,
		Execution: factoryapi.WorkerSessionResolvedExecution{
			WorkstationName: "process",
			WorkerType:      functionalStringPtr("processor"),
			Dispatch: factoryapi.WorkerSessionResolvedDispatch{
				DispatchId:      dispatchID,
				WorkstationName: "process",
				WorkerType:      functionalStringPtr("processor"),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal direct Worker Session request: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/worker-sessions", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("construct direct Worker Session request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /worker-sessions: %v", err)
	}
	return response
}

func functionalStringPtr(value string) *string { return &value }

type functionalWorkerGate struct {
	gate         <-chan struct{}
	started      chan struct{}
	completed    chan struct{}
	mu           sync.Mutex
	calls        int
	canceled     bool
	startOnce    sync.Once
	completeOnce sync.Once
}

func newFunctionalWorkerGate(gate <-chan struct{}) *functionalWorkerGate {
	return &functionalWorkerGate{gate: gate, started: make(chan struct{}), completed: make(chan struct{})}
}

func (r *functionalWorkerGate) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.startOnce.Do(func() { close(r.started) })
	r.mu.Unlock()
	select {
	case <-r.gate:
		r.completeOnce.Do(func() { close(r.completed) })
		return platformprocess.CommandResult{Stdout: []byte("functional worker completed")}, nil
	case <-ctx.Done():
		r.mu.Lock()
		r.canceled = true
		r.mu.Unlock()
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func (r *functionalWorkerGate) waitCompleted() error {
	select {
	case <-r.completed:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("functional worker did not reach deterministic completion")
	}
}

func (r *functionalWorkerGate) waitStarted() error {
	select {
	case <-r.started:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("functional worker did not reach the deterministic execution gate")
	}
}

func (r *functionalWorkerGate) wasCanceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.canceled
}

func (r *functionalWorkerGate) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

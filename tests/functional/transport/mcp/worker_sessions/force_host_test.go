package workersessions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Only the process effect is controlled. Providers, Workers, recording,
// supervision and the three customer transports use the production graph.
type forceHostRunner struct {
	started chan (<-chan struct{})
	signals atomic.Int32
}

func (r *forceHostRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	control := &forceHostControl{stop: make(chan struct{}), done: make(chan struct{}), signals: &r.signals}
	defer close(control.done)
	if req.OwnedProcessObserver != nil {
		req.OwnedProcessObserver(control)
	}
	r.started <- control.done
	select {
	case <-control.stop:
		return platformprocess.CommandResult{ExitCode: -1}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

type forceHostControl struct {
	stop, done chan struct{}
	signals    *atomic.Int32
	once       sync.Once
}

func (c *forceHostControl) ForceKill(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c.once.Do(func() { c.signals.Add(1); close(c.stop) })
	select {
	case <-c.done:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func runRealHostForceControls(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-host")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := &forceHostRunner{started: make(chan (<-chan struct{}), 4)}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	admission := controlHostRunner{started: runner.started}
	sibling := admitControlWorker(t, ctx, host.URL(), "force-sibling", admission)
	for index, transport := range []string{"http", "cli", "mcp"} {
		id := "force-" + transport
		done := admitControlWorker(t, ctx, host.URL(), id, admission)
		observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
		if observation["providerSessionAvailable"] != false {
			t.Fatalf("force fixture unexpectedly published a provider reference: %v", observation)
		}
		attempt := observation["attemptId"].(string)
		endpoint := host.URL() + "/worker-sessions/" + id + "/terminate"
		postHostJSON(t, ctx, endpoint, map[string]any{"force": true, "requestId": "stale-" + id, "expectedAttemptId": "stale"}, http.StatusConflict)
		if runner.signals.Load() != int32(index) {
			t.Fatal("stale force signaled a provider attempt")
		}
		payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		args := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		var result any
		switch transport {
		case "http":
			result = postHostJSON(t, ctx, endpoint, payload, http.StatusOK)
		case "cli":
			result = executeForceCLI(t, host, id, attempt)
		case "mcp":
			result = callWorker(t, ctx, session, "control", args)["result"]
		}
		assertOwnedControl(t, result.(map[string]any), id, "TERMINATE", "APPLIED")
		if result.(map[string]any)["forced"] != true || result.(map[string]any)["state"] != "TERMINATED" {
			t.Fatalf("force response omitted joined force facts: %v", result)
		}
		waitControlSignal(t, done)
		assertJSONEqual(t, result, postHostJSON(t, ctx, endpoint, payload, http.StatusOK))
		assertJSONEqual(t, result, executeForceCLI(t, host, id, attempt))
		assertJSONEqual(t, result, callWorker(t, ctx, session, "control", args)["result"])
		postHostJSON(t, ctx, endpoint, map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": "changed-attempt"}, http.StatusConflict)
		assertForceObservation(t, host, id)
		select {
		case <-sibling:
			t.Fatal("force stopped sibling execution")
		default:
		}
	}
	if runner.signals.Load() != 3 {
		t.Fatalf("force/replays signaled %d times, want one per exact attempt", runner.signals.Load())
	}
	assertFactoryForceControl(t, host, dir, runner)
	select {
	case <-sibling:
		t.Fatal("Factory force stopped direct sibling execution")
	default:
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "force-sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
}

func assertFactoryForceControl(t *testing.T, host *support.FunctionalAPIServer, dir string, runner *forceHostRunner) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	support.SubmitSessionWorkAt(t, host.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "hold Factory force worker"})
	var done <-chan struct{}
	select {
	case done = <-runner.started:
	case <-t.Context().Done():
		t.Fatal("Factory provider boundary not reached")
	}
	fleet := getHost(t, host.URL()+"/worker-sessions?scope=factory").(map[string]any)
	workers := fleet["sessions"].([]any)
	if len(workers) != 1 {
		t.Fatalf("Factory force fleet = %v", fleet)
	}
	observation := workers[0].(map[string]any)
	id, attempt := observation["workerSessionId"].(string), observation["attemptId"].(string)
	if observation["factorySessionId"] != opened.Session.Id || observation["direct"] != false || observation["providerSessionAvailable"] != false {
		t.Fatalf("Factory force lost exact scope: %v", observation)
	}
	endpoint := host.URL() + "/worker-sessions/" + id + "/terminate"
	payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	result := postHostJSON(t, t.Context(), endpoint, payload, http.StatusOK)
	control := result.(map[string]any)
	if control["workerSessionId"] != id || control["dispatchId"] != id || control["action"] != "TERMINATE" || control["outcome"] != "APPLIED" || control["forced"] != true {
		t.Fatalf("Factory force result lost exact dispatch: %v", control)
	}
	waitControlSignal(t, done)
	assertJSONEqual(t, result, executeForceCLI(t, host, id, attempt))
	assertForceObservation(t, host, id)
	if runner.signals.Load() != 4 {
		t.Fatalf("Factory force/replay signaled %d times, want 4 total", runner.signals.Load())
	}
}

func executeForceCLI(t *testing.T, host *support.FunctionalAPIServer, id, attempt string) any {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "terminate", id,
		"--force", "--request-id", "kill-" + id, "--expected-attempt-id", attempt, "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI force: %v stderr=%s", err, inputs.Stderr())
	}
	var result any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertForceObservation(t *testing.T, host *support.FunctionalAPIServer, id string) {
	t.Helper()
	observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
	if observation["state"] != "TERMINATED" || observation["terminalCause"] != "OPERATOR_KILL" || observation["providerSessionAvailable"] != false {
		t.Fatalf("joined force observation = %v", observation)
	}
}

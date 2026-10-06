package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Only the process effect is controlled. Providers, Workers, recording,
// supervision and the three customer transports use the production graph.
type forceHostRunner struct {
	started chan (<-chan struct{})
	mode    string
	entered chan struct{}
	release <-chan struct{}
	signals atomic.Int32
	calls   atomic.Int32
}

func (r *forceHostRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	control := &forceHostControl{stop: make(chan struct{}), done: make(chan struct{}), signals: &r.signals, calls: &r.calls, mode: r.mode,
		entered: r.entered, release: r.release}
	defer close(control.done)
	if req.OwnedProcessObserver != nil && r.mode != "unattached" {
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
	calls      *atomic.Int32
	mode       string
	entered    chan struct{}
	release    <-chan struct{}
	once       sync.Once
}

func (c *forceHostControl) ForceKill(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c.calls.Add(1)
	switch c.mode {
	case "declined":
		return false, nil
	case "failed":
		return false, errors.New("secret-from-host")
	}
	if c.release != nil {
		close(c.entered)
		select {
		case <-c.release:
		case <-ctx.Done():
			return false, ctx.Err()
		}
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
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	}
	host := support.StartFunctionalAPIServer(t, config)
	session, ctx := startMCP(t, process, host.URL())
	var completed []completedForceControl
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
		assertForceInputsRejected(t, host, session, ctx, id, endpoint)
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
		completed = append(completed, completedForceControl{id: id, attempt: attempt, result: result})
		select {
		case <-sibling:
			t.Fatal("force stopped sibling execution")
		default:
		}
	}
	if runner.signals.Load() != 3 {
		t.Fatalf("force/replays signaled %d times, want one per exact attempt", runner.signals.Load())
	}
	completed = append(completed, assertFactoryForceControl(t, host, dir, runner))
	select {
	case <-sibling:
		t.Fatal("Factory force stopped direct sibling execution")
	default:
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "force-sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
	// Join the original owner before reopening its durable profile. Recovery
	// must read saved outcomes without reconstructing process capabilities.
	host.Close(t)
	reopened := support.StartFunctionalAPIServer(t, config)
	recoveredSession, recoveredCtx := startMCP(t, process, reopened.URL())
	for _, saved := range completed {
		saved.assertRecovered(t, reopened, recoveredSession, recoveredCtx)
	}
	if runner.signals.Load() != 4 || runner.calls.Load() != 4 {
		t.Fatalf("restart repeated force effects: calls=%d signals=%d", runner.calls.Load(), runner.signals.Load())
	}
}

func assertFactoryForceControl(t *testing.T, host *support.FunctionalAPIServer, dir string, runner *forceHostRunner) completedForceControl {
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
	return completedForceControl{id: id, attempt: attempt, result: result}
}

type completedForceControl struct {
	id, attempt string
	result      any
}

func (saved completedForceControl) assertRecovered(t *testing.T, host *support.FunctionalAPIServer, session *mcp.ClientSession, ctx context.Context) {
	t.Helper()
	assertForceObservation(t, host, saved.id)
	endpoint := host.URL() + "/worker-sessions/" + saved.id + "/terminate"
	payload := map[string]any{"force": true, "requestId": "kill-" + saved.id, "expectedAttemptId": saved.attempt}
	args := map[string]any{"operation": "KILL", "workerSessionId": saved.id, "requestId": "kill-" + saved.id, "expectedAttemptId": saved.attempt}
	assertJSONEqual(t, saved.result, postHostJSON(t, ctx, endpoint, payload, http.StatusOK))
	assertJSONEqual(t, saved.result, executeForceCLI(t, host, saved.id, saved.attempt))
	assertJSONEqual(t, saved.result, callWorker(t, ctx, session, "control", args)["result"])
	assertForceObservation(t, host, saved.id)
	// A committed outcome grants read-only authority for the original tuple.
	// It cannot validate a changed tuple or create a fresh effect on an archive.
	payload["expectedAttemptId"] = "changed-attempt"
	postHostJSON(t, ctx, endpoint, payload, http.StatusConflict)
	args["expectedAttemptId"] = "changed-attempt"
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", args), "worker_session.conflict", false)
	assertForceCLIErrorCode(t, host, saved.id, "WORKER_SESSION_CONTROL_CONFLICT",
		"--force", "--request-id", "kill-"+saved.id, "--expected-attempt-id", "changed-attempt")
	payload["expectedAttemptId"], payload["requestId"] = saved.attempt, "fresh-"+saved.id
	postHostJSON(t, ctx, endpoint, payload, http.StatusNotFound)
	args["expectedAttemptId"], args["requestId"] = saved.attempt, "fresh-"+saved.id
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", args), "worker_session.not_found", false)
	assertForceCLIErrorCode(t, host, saved.id, "NOT_FOUND",
		"--force", "--request-id", "fresh-"+saved.id, "--expected-attempt-id", saved.attempt)
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

type forceFailureFixture struct {
	host    *support.FunctionalAPIServer
	session *mcp.ClientSession
	ctx     context.Context
	runner  *forceHostRunner
}

// Each immutable failure edge owns its host/profile. The three transports
// reuse that host and replay each intent. This controls only command effects
// and does not prove any OS-tree boundary.
func runForceHostFailure(t *testing.T, process support.Process, mode string) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-"+mode)
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := &forceHostRunner{started: make(chan (<-chan struct{}), 4), mode: mode}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	f := forceFailureFixture{host: host, session: session, ctx: ctx, runner: runner}
	sibling := admitControlWorker(t, ctx, host.URL(), "failure-sibling", controlHostRunner{started: runner.started})
	for index, transport := range []string{"http", "cli", "mcp"} {
		f.check(t, index, transport)
		assertForceStillActive(t, host, "failure-sibling", sibling)
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "failure-sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
}

func (f forceFailureFixture) check(t *testing.T, index int, transport string) {
	t.Helper()
	id := "failure-" + transport
	done := admitControlWorker(t, f.ctx, f.host.URL(), id, controlHostRunner{started: f.runner.started})
	before := getHost(t, f.host.URL()+"/worker-sessions/"+id).(map[string]any)
	attempt := before["attemptId"].(string)
	// Rotate the first transport so each admits a fresh operation, then recover
	// that same operation through the others, with no additional command effect.
	for offset := range 3 {
		selected := []string{"http", "cli", "mcp"}[(index+offset)%3]
		f.control(t, selected, id, attempt, before["state"])
	}
	assertForceStillActive(t, f.host, id, done)
	wantCalls := int32(index + 1)
	if f.runner.mode == "unattached" {
		wantCalls = 0
	}
	if f.runner.calls.Load() != wantCalls || f.runner.signals.Load() != 0 {
		t.Fatalf("force calls=%d signals=%d; want calls=%d signals=0", f.runner.calls.Load(), f.runner.signals.Load(), wantCalls)
	}
	// Unconfirmed force must leave ordinary safety stopping usable.
	ordinary := callWorker(t, f.ctx, f.session, "control", map[string]any{"workerSessionId": id, "operation": "TERMINATE"})["result"].(map[string]any)
	assertOwnedControl(t, ordinary, id, "TERMINATE", "APPLIED")
	if _, present := ordinary["forced"]; present {
		t.Fatalf("ordinary terminate acquired force semantics: %v", ordinary)
	}
	waitControlSignal(t, done)
	observation := getHost(t, f.host.URL()+"/worker-sessions/"+id).(map[string]any)
	if observation["terminalCause"] == "OPERATOR_KILL" {
		t.Fatalf("unconfirmed force invented kill causation: %v", observation)
	}
	// Terminality cannot rewrite a committed unsupported/failed operation or
	// confer fresh effect authority on a retry of that operation.
	for _, selected := range []string{"http", "cli", "mcp"} {
		f.control(t, selected, id, attempt, before["state"])
	}
	if f.runner.calls.Load() != wantCalls || f.runner.signals.Load() != 0 {
		t.Fatal("terminal recovery repeated an unconfirmed force effect")
	}
}

func (f forceFailureFixture) control(t *testing.T, transport, id, attempt string, state any) {
	t.Helper()
	failed := f.runner.mode == "failed"
	args := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	var result map[string]any
	switch transport {
	case "http":
		status := http.StatusOK
		if failed {
			status = http.StatusServiceUnavailable
		}
		payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		result = postHostJSON(t, f.ctx, f.host.URL()+"/worker-sessions/"+id+"/terminate", payload, status).(map[string]any)
		if failed {
			if result["code"] != "WORKER_SESSION_CONTROL_FAILED" {
				t.Fatalf("force HTTP failure = %v", result)
			}
			assertNoForceSecret(t, result)
			return
		}
	case "cli":
		if failed {
			assertForceCLIError(t, f.host, id, attempt)
			return
		}
		result = executeForceCLI(t, f.host, id, attempt).(map[string]any)
	case "mcp":
		if failed {
			failure := callTool(t, f.ctx, f.session, "you.worker_session.control", args)
			assertToolError(t, failure, "worker_session.unavailable", true)
			return
		}
		result = callWorker(t, f.ctx, f.session, "control", args)["result"].(map[string]any)
	}
	assertOwnedControl(t, result, id, "TERMINATE", "UNSUPPORTED")
	if result["forced"] != true || result["state"] != state {
		t.Fatalf("unsupported force changed state or omitted force facts: %v", result)
	}
}

func assertForceCLIError(t *testing.T, host *support.FunctionalAPIServer, id, attempt string) {
	t.Helper()
	assertForceCLIErrorCode(t, host, id, "WORKER_SESSION_CONTROL_FAILED",
		"--force", "--request-id", "kill-"+id, "--expected-attempt-id", attempt)
}

func assertForceCLIErrorCode(t *testing.T, host *support.FunctionalAPIServer, id, code string, flags ...string) {
	t.Helper()
	argv := append([]string{"you", "worker-sessions", "terminate", id, "--server", host.URL(), "--json"}, flags...)
	inputs := support.FakeInputs(t.Context(), argv)
	err := host.Execute(t, inputs.Input)
	var typed *workersessionscli.CLIError
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("CLI force failure = %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	assertNoForceSecret(t, []any{err.Error(), inputs.Stdout(), inputs.Stderr()})
}

func assertForceInputsRejected(t *testing.T, host *support.FunctionalAPIServer, session *mcp.ClientSession, ctx context.Context, id, endpoint string) {
	t.Helper()
	postHostJSON(t, ctx, endpoint, map[string]any{"force": true, "requestId": "stale-" + id, "expectedAttemptId": "stale"}, http.StatusConflict)
	assertForceCLIErrorCode(t, host, id, "WORKER_SESSION_CONTROL_CONFLICT",
		"--force", "--request-id", "stale-"+id, "--expected-attempt-id", "stale")
	stale := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "stale-" + id, "expectedAttemptId": "stale"}
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", stale), "worker_session.conflict", false)
	postHostJSON(t, ctx, endpoint, map[string]any{"force": true, "requestId": "invalid-" + id}, http.StatusBadRequest)
	assertForceCLIErrorCode(t, host, id, "WORKER_SESSION_CONTROL_INVALID", "--request-id", "invalid-"+id)
	invalid := map[string]any{"operation": "KILL", "workerSessionId": id, "expectedAttemptId": "stale"}
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", invalid), "worker_session.invalid_request", false)
}

func assertNoForceSecret(t *testing.T, result any) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-from-host") {
		t.Fatalf("force error leaked command details: %s", encoded)
	}
}

func assertForceStillActive(t *testing.T, host *support.FunctionalAPIServer, id string, done <-chan struct{}) {
	t.Helper()
	observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
	if observation["state"] != "RUNNING" || observation["terminalCause"] == "OPERATOR_KILL" {
		t.Fatalf("unconfirmed force changed live observation: %v", observation)
	}
	select {
	case <-done:
		t.Fatal("unconfirmed force stopped an execution")
	default:
	}
}

func runDetachedForceControl(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-detached")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	// Release the effect before host cleanup even when an assertion aborts.
	defer unblock()
	runner := &forceHostRunner{started: make(chan (<-chan struct{}), 1), entered: make(chan struct{}), release: release}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	id := "detached-force"
	done := admitControlWorker(t, ctx, host.URL(), id, controlHostRunner{started: runner.started})
	before := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
	attempt := before["attemptId"].(string)
	payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	endpoint := host.URL() + "/worker-sessions/" + id + "/terminate"
	cancel, response := beginForceHTTPRequest(t, ctx, endpoint, payload)
	defer cancel()
	waitControlSignal(t, runner.entered)
	// The owned effect has entered but cannot complete before the explicit
	// release. A successful HTTP response here would promise false terminality.
	select {
	case err := <-response:
		t.Fatalf("force returned before owned completion: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-response:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("force observer disconnect = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("force HTTP observer did not detach")
	}
	assertForceStillActive(t, host, id, done)
	unblock()
	args := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	result := callWorker(t, ctx, session, "control", args)["result"].(map[string]any)
	assertOwnedControl(t, result, id, "TERMINATE", "APPLIED")
	if result["forced"] != true || result["state"] != "TERMINATED" {
		t.Fatalf("detached force replay lost joined facts: %v", result)
	}
	waitControlSignal(t, done)
	assertJSONEqual(t, result, executeForceCLI(t, host, id, attempt))
	assertJSONEqual(t, result, postHostJSON(t, ctx, endpoint, payload, http.StatusOK))
	assertForceObservation(t, host, id)
	if runner.calls.Load() != 1 || runner.signals.Load() != 1 {
		t.Fatalf("detached/replayed force calls=%d signals=%d, want one", runner.calls.Load(), runner.signals.Load())
	}
}

// HTTP cancellation models observer disconnect without terminating the host.
// The goroutine returns errors through a channel and owns its response body.
func beginForceHTTPRequest(t *testing.T, ctx context.Context, endpoint string, payload any) (context.CancelFunc, <-chan error) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	observer, cancel := context.WithCancel(ctx)
	request, err := http.NewRequestWithContext(observer, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response := make(chan error, 1)
	go func() {
		result, requestErr := http.DefaultClient.Do(request)
		if result != nil {
			_ = result.Body.Close()
		}
		response <- requestErr
	}()
	return cancel, response
}

package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// The same immutable command edge serves source/resume and the occupied peer.
// Only the peer waits; source revival must finish before the peer is released.
type durableRevivalCommandRunner struct {
	source *testutil.ProviderCommandRunner
	peer   *t7GatedProviderRunner
}

func (runner *durableRevivalCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *durableRevivalCommandRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if strings.Contains(string(request.Stdin), "durable occupied peer input") {
		return runner.peer.RunStreaming(ctx, request, observe)
	}
	result, err := runner.source.Run(ctx, request)
	if observe != nil && len(result.Stdout) != 0 {
		observe(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func writeDurableRevivalCapacity(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var definition map[string]any
	if err := json.Unmarshal(content, &definition); err != nil {
		t.Fatal(err)
	}
	resource := []map[string]any{{"name": "executor-slot", "capacity": 1}}
	definition["resources"] = resource
	definition["workstations"].([]any)[0].(map[string]any)["resources"] = resource
	writeInvokeContinueJSON(t, path, definition)
}

func assertDurableRevivalPeerCompletion(t *testing.T, host invokeContinueStartedProcess, session, id string, runner *t7GatedProviderRunner) {
	t.Helper()
	t7AssertFactorySibling(t, t.Context(), host.baseURL, id, session, "RUNNING")
	if runner.CallCount() != 1 {
		t.Fatalf("revival changed peer admission: calls=%d", runner.CallCount())
	}
	close(runner.release)
	t19AwaitSignal(t, t.Context(), runner.stopped, "Factory peer completed independently")
	support.WaitForSessionTerminalStatus(t, host.baseURL, session, 30*time.Second)
	t7AssertFactorySibling(t, t.Context(), host.baseURL, id, session, "COMPLETED")
}

// F-10 retains real healthy capture history while the external store returns
// missing or stale execution authority. Restart, observation and refusal cross
// production wiring; no Worker Sessions or Provider Sessions peer is replaced.
func assertDurableRevivalRecipeRefusal(t *testing.T, host invokeContinueStartedProcess, home, dir, sourceID string, runner *testutil.ProviderCommandRunner) {
	t.Helper()
	awaitContinuationRestartLogs(t, host, home, dir, sourceID)
	status, logs := t7HTTP(t, t.Context(), http.MethodGet, host.baseURL+"/worker-sessions/"+sourceID+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(logs, `"health":"COMPLETE"`) {
		t.Fatalf("recipe refusal lost healthy history: %s", logs)
	}
	observation := support.GetJSON[api.WorkerSessionObservation](t, host.baseURL+"/worker-sessions/"+sourceID)
	if observation.State != "COMPLETED" || observation.Revivable == nil || *observation.Revivable || observation.SuccessorWorkerSessionId != nil {
		t.Fatalf("unavailable recipe granted revival authority: %+v", observation)
	}
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, host.baseURL+"/worker-sessions?history=archived")
	if len(rows.Sessions) != 1 || !reflect.DeepEqual(observation, rows.Sessions[0]) {
		t.Fatalf("archived recipe capability disagrees with show: %+v", rows)
	}
	assertDurableRevivalCLIRefusal(t, host, home, dir, sourceID)
	status, body := t7HTTP(t, t.Context(), http.MethodPost, host.baseURL+"/worker-sessions/"+sourceID+"/continue",
		map[string]any{"resolveHead": true, "requestId": "recipe-http-request", "successorWorkerSessionId": "recipe-http-successor", "followUpInput": "follow-up"})
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED") || runner.CallCount() != 1 {
		t.Fatalf("recipe refusal: status=%d body=%s calls=%d", status, body, runner.CallCount())
	}
	after := support.GetJSON[api.WorkerSessionObservation](t, host.baseURL+"/worker-sessions/"+sourceID)
	if !reflect.DeepEqual(observation, after) {
		t.Fatalf("recipe refusal mutated source: before=%+v after=%+v", observation, after)
	}
}

func assertDurableRevivalCLIRefusal(t *testing.T, host invokeContinueStartedProcess, home, dir, sourceID string) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		flags := []string{"you", "--json"}
		if remote {
			flags = append(flags, "--remote", "--server", host.baseURL)
		}
		request := support.FakeInputs(t.Context(), append(flags, "worker-sessions", "continue", sourceID,
			"--head", "--request-id", "recipe-refusal-request", "--successor-worker-session-id", "recipe-refusal-successor", "--user-message", "follow-up"))
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		if err := host.process.Execute(request.Input); err == nil {
			t.Fatal("unavailable recipe admitted a successor")
		}
		assertDirectWorkerSessionCLIError(t, request, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED")
		if strings.Contains(request.Stderr()+request.Stdout(), "private-revival") {
			t.Fatal("recipe diagnostics exposed private persistence detail")
		}
	}
}

// F6-04 admits actual Factory Work beside a direct invocation through one
// assembled host. Each attempt has its own command gate and captured identity.
func TestT7DirectStopLeavesFactorySiblingRunning(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.cancel", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	target := fixture.scenario(t, "t7-factory-target")
	defer target.close(t)
	peer := fixture.scenario(t, "t7-factory")
	defer peer.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	targetRunner := target.providerRunner.(*t7GatedProviderRunner)
	peerRunner := peer.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, targetRunner)()
	defer t7ReleaseAndJoin(t, ctx, peerRunner)()
	t7WriteFactorySibling(t, peer.workingDirectory)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, peer.workingDirectory)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	work := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{
		WorkTypeName: "task", Payload: "T7 Factory sibling input",
	})
	if work.WorkId == nil {
		t.Fatal("Factory Work has no ID")
	}
	t19AwaitSignal(t, ctx, peerRunner.started, "Factory sibling started")
	endpoint := fixture.baseURL + "/factory-sessions/" + opened.Session.Id + "/worker-sessions?workId=" + *work.WorkId
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, endpoint)
	if len(rows.Sessions) != 1 {
		t.Fatalf("Factory Worker Sessions = %#v", rows)
	}
	peerID := rows.Sessions[0].WorkerSessionId
	id, _ := t7StartGatedAsync(t, ctx, fixture, target)
	t19AwaitSignal(t, ctx, targetRunner.started, "direct invocation started")
	t7AssertFactorySibling(t, ctx, fixture.baseURL, peerID, opened.Session.Id, "RUNNING")
	stop := t7RemoteCLIInputs(target, ctx, fixture.baseURL, "cancel", id)
	if err := fixture.process.Execute(stop.Input); err != nil {
		t.Fatalf("direct cancel: %v: %s", err, stop.Stderr())
	}
	t7AssertStopResult(t, stop.Stdout(), id, "cancel")
	t19AwaitSignal(t, ctx, targetRunner.stopped, "direct joined")
	t7AssertSingleTerminal(t, ctx, fixture.baseURL, id, "CANCELED")
	t7AssertFactorySibling(t, ctx, fixture.baseURL, peerID, opened.Session.Id, "RUNNING")
	close(peerRunner.release)
	t19AwaitSignal(t, ctx, peerRunner.stopped, "Factory command joined")
	body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+peerID, nil)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, `"state":"COMPLETED"`) })
	if err != nil {
		t.Fatalf("Factory sibling completion: %v: %s", err, body)
	}
	t7AssertFactorySibling(t, ctx, fixture.baseURL, peerID, opened.Session.Id, "COMPLETED")
	status, logs := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+peerID+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(logs, "T7 Factory sibling COMPLETE") || strings.Contains(logs, id) {
		t.Fatalf("Factory sibling captured logs = %d %s", status, logs)
	}
	command := peerRunner.Requests()[0]
	if command.Command != "codex" || !strings.Contains(strings.Join(command.Args, " "), "--model opaque-factory-model") ||
		!strings.Contains(strings.Join(command.Args, " "), `model_reasoning_effort="high"`) ||
		!strings.Contains(string(command.Stdin), "T7 Factory sibling input") || targetRunner.CallCount() != 1 || peerRunner.CallCount() != 1 {
		t.Fatalf("Factory command settings or isolated attempt counts: command=%s args=%q prompt=%q target=%d peer=%d", command.Command, command.Args, command.Stdin, targetRunner.CallCount(), peerRunner.CallCount())
	}
}

func t7AssertFactorySibling(t *testing.T, ctx context.Context, baseURL, id, session, state string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id, nil)
	var observation api.WorkerSessionObservation
	if err := json.Unmarshal([]byte(body), &observation); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || observation.Direct || observation.WorkerSessionId != id || observation.FactorySessionId == nil ||
		*observation.FactorySessionId != session || string(observation.State) != state || observation.Model == nil || *observation.Model != "opaque-factory-model" {
		t.Fatalf("Factory sibling observation = %d %s", status, body)
	}
}

func t7WriteFactorySibling(t *testing.T, directoryPath string) {
	t.Helper()
	// Copy authored inputs only: the running host contains recordings that
	// parallel attempts can atomically rename while a directory walk is active.
	if err := copyInvokeContinueDirectory(support.LegacyFixtureDir(t, "executor_success"), directoryPath); err != nil {
		t.Fatal(err)
	}
	// Only the explicitly submitted sibling Work should start this Factory.
	support.ClearSeedInputs(t, directoryPath)
	directory, _ := json.Marshal(directoryPath)
	worker := "---\ntype: MODEL_WORKER\nexecutorProvider: CODEX\nmodel: opaque-factory-model\nmodelProvider: codex\nreasoningEffort: high\nstopToken: COMPLETE\n---\nFactory sibling instruction.\n"
	if err := os.WriteFile(filepath.Join(directoryPath, "workers", "worker", "AGENTS.md"), []byte(worker), 0o600); err != nil {
		t.Fatal(err)
	}
	station := fmt.Sprintf("---\ntype: MODEL_WORKSTATION\nrunner: codex\nworkingDirectory: %s\n---\nFactory sibling input: {{ (index .Inputs 0).Payload }}\n", directory)
	if err := os.WriteFile(filepath.Join(directoryPath, "workstations", "process", "AGENTS.md"), []byte(station), 0o600); err != nil {
		t.Fatal(err)
	}
}

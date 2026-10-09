package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// Only fixture identities, the host-visible directory and explicit Factory
// Session correlation differ from the customer-published request.
func TestPublishedWorkerSessionDirectJourney(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "published-direct")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	id := scenarioScopedID(scenario, "published-session")
	path := publishedDirectExecution(t, scenario, id)
	invoke := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path, "--user-message", "Reply with DIRECT_EXAMPLE_OK.", "--async")
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("published admission: %v %s", err, invoke.Stderr())
	}
	var admitted directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, invoke.Stdout(), &admitted)
	if !admitted.Accepted || admitted.WorkerSessionID != id {
		t.Fatalf("canonical admission = %#v", admitted)
	}
	t19AwaitSignal(t, ctx, runner.started, "published provider startup")
	command := runner.Requests()[0]
	args := strings.Join(command.Args, " ")
	if command.Command != "codex" || command.WorkDir != scenario.workingDirectory || string(command.Stdin) != "Reply with DIRECT_EXAMPLE_OK." || !strings.Contains(args, "--model gpt-5") || !strings.Contains(args, `model_reasoning_effort="high"`) || !strings.Contains(args, `developer_instructions="Return the requested short reply."`) {
		t.Fatalf("published provider parameters differ: command=%q args=%q directory=%q", command.Command, command.Args, command.WorkDir)
	}
	show := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "show", "--worker-session-id", admitted.WorkerSessionID)
	if err := fixture.process.Execute(show.Input); err != nil || !strings.Contains(show.Stdout(), `"state":"RUNNING"`) {
		t.Fatalf("published live show: %v %s", err, show.Stdout())
	}
	logs := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", admitted.WorkerSessionID, "--view", "logs")
	if err := fixture.process.Execute(logs.Input); err != nil || !strings.Contains(logs.Stdout(), id) {
		t.Fatalf("published live logs: %v %s", err, logs.Stdout())
	}
	close(runner.release)
	joined := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(joined.Input); err != nil {
		t.Fatalf("published terminal join: %v %s", err, joined.Stderr())
	}
	ended := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "show", "--worker-session-id", id)
	if err := fixture.process.Execute(ended.Input); err != nil || !strings.Contains(ended.Stdout(), `"state":"COMPLETED"`) {
		t.Fatalf("published terminal show: %v %s", err, ended.Stdout())
	}
	terminal := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", "logs")
	if err := fixture.process.Execute(terminal.Input); err != nil || !strings.Contains(terminal.Stdout(), "T7 detached attempt completed") || !strings.Contains(terminal.Stdout(), `"health":"COMPLETE"`) || runner.CallCount() != 1 {
		t.Fatalf("published captured result: %v %s calls=%d", err, terminal.Stdout(), runner.CallCount())
	}
}

func publishedDirectExecution(t *testing.T, scenario *invokeContinueScenario, id string) string {
	t.Helper()
	document := publishedExecutionDocument(t)
	document["requestId"], document["workerSessionId"] = id+"-request", id
	execution := document["execution"].(map[string]any)
	execution["workingDirectory"] = scenario.workingDirectory
	execution["factorySessionId"] = scenario.session.id
	dispatch := execution["dispatch"].(map[string]any)
	dispatch["dispatchId"] = id + "-attempt"
	dispatch["execution"].(map[string]any)["requestId"] = id + "-request"
	path := filepath.Join(scenario.workingDirectory, "execution.json")
	writeInvokeContinueJSON(t, path, document)
	return path
}

// Keep this docs fixture in the test compilation rather than the production graph.
func publishedExecutionDocument(t testing.TB) map[string]any {
	t.Helper()
	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/reference/operations.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"), "Save this complete request as `execution.json`:")
	if !ok {
		t.Fatal("published execution request is missing")
	}
	_, block, ok := strings.Cut(section, "```json\n")
	if !ok {
		t.Fatal("published execution JSON is missing")
	}
	block, _, ok = strings.Cut(block, "\n```")
	if !ok {
		t.Fatal("published execution JSON is incomplete")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(block), &document); err != nil {
		t.Fatalf("published execution JSON: %v", err)
	}
	return document
}

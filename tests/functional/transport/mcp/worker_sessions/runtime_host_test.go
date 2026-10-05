package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func runRealHostFactory(t *testing.T, process support.Process) {
	t.Helper()
	host, runner, dir := startInterruptHost(t, nil)
	session, ctx := startMCP(t, process, host.URL())
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	sessionID := opened.Session.Id
	support.SubmitSessionWorkAt(t, host.URL(), sessionID, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: map[string]string{"instruction": "hold this Factory worker"},
	})
	waitProviderRequest(t, ctx, runner)
	listed := callWorker(t, ctx, session, "list", map[string]any{"scope": "factory"})["result"].(map[string]any)
	assertRuntimeObservationParity(t, listed, getHost(t, host.URL()+"/worker-sessions?scope=factory"))
	workers := listed["sessions"].([]any)
	if len(workers) != 1 {
		t.Fatalf("Factory fleet: %v", listed)
	}
	worker := workers[0].(map[string]any)
	id := worker["workerSessionId"].(string)
	if worker["direct"] != false || worker["factorySessionId"] != sessionID || worker["providerSessionAvailable"] != true {
		t.Fatalf("Factory Worker association: %v", worker)
	}
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id)
	read := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)
	assertRuntimeObservationParity(t, read["session"], getHost(t, endpoint))
	assertFactoryCLIParity(t, host, id, getHost(t, endpoint))
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.read", map[string]any{"workerSessionId": id, "view": "transcript"}), "worker_session.conflict", false)
	args := map[string]any{"workerSessionId": id, "operation": "INTERRUPT", "requestId": "factory-interrupt", "successorWorkerSessionId": "unsupported-successor", "replacementMessage": "replacement"}
	// Existing Factory dispatches lack direct interrupt supervision. HTTP maps
	// the validation-phase rejection to 400 before the specific conflict code.
	// Preserve host policy and prove rejection has no provider effect.
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", args), "worker_session.invalid_request", false)
	postHostJSON(t, ctx, endpoint+"/interrupt", map[string]any{"requestId": "factory-interrupt", "successorWorkerSessionId": "unsupported-successor", "replacementMessage": "replacement"}, http.StatusBadRequest)
	assertProviderCallCount(t, runner, 1)
	if source := getHost(t, endpoint).(map[string]any); source["state"] != "RUNNING" {
		t.Fatalf("rejected Factory interrupt changed source: %v", source)
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "TERMINATE"})
	waitControlSignal(t, runner.sourceStopped)
	transcript := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id, "view": "transcript"})["result"].(map[string]any)
	assertTranscriptEqual(t, transcript["transcript"], getHost(t, endpoint+"/transcript"))
}

func assertFactoryCLIParity(t *testing.T, host *support.FunctionalAPIServer, id string, expected any) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "show", "--worker-session-id", id, "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI Factory show: %v stderr=%s", err, inputs.Stderr())
	}
	var actual any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &actual); err != nil {
		t.Fatal(err)
	}
	normalizeActiveDuration(t, actual)
	normalizeActiveDuration(t, expected)
	// CLI serialization uses generated optional members, including nulls.
	// Decode both observations into that contract before comparing values.
	observations := make([]factoryapi.WorkerSessionObservation, 2)
	for index, value := range []any{actual, expected} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &observations[index]); err != nil {
			t.Fatal(err)
		}
	}
	assertJSONEqual(t, observations[0], observations[1])
}

// The independently root-built host is the real selected HTTP boundary. Only
// provider execution is controlled; no provider session association is emitted.
type controlHostRunner struct{ started chan (<-chan struct{}) }

func (r controlHostRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	done := make(chan struct{})
	r.started <- done
	<-ctx.Done()
	close(done)
	return platformprocess.CommandResult{}, ctx.Err()
}

func runRealHostControls(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-control-host")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 4)}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	session, ctx := startMCP(t, process, host.URL())
	sibling := admitControlWorker(t, ctx, host.URL(), "sibling", runner)
	for _, operation := range []string{"CANCEL", "TERMINATE"} {
		id := "mcp-" + strings.ToLower(operation)
		done := admitControlWorker(t, ctx, host.URL(), id, runner)
		listed := callWorker(t, ctx, session, "list", map[string]any{"scope": "direct"})["result"]
		assertRuntimeObservationParity(t, listed, getHost(t, host.URL()+"/worker-sessions?scope=direct"))
		before := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"]
		assertRuntimeObservationParity(t, before, getHost(t, host.URL()+"/worker-sessions/"+id))
		assertFactoryCLIParity(t, host, id, getHost(t, host.URL()+"/worker-sessions/"+id))
		args := map[string]any{"workerSessionId": id, "operation": operation}
		applied := callWorker(t, ctx, session, "control", args)["result"].(map[string]any)
		assertOwnedControl(t, applied, id, operation, "APPLIED")
		waitControlSignal(t, done)
		retry := callWorker(t, ctx, session, "control", args)["result"]
		assertOwnedControl(t, retry.(map[string]any), id, operation, "NOOP")
		assertJSONEqual(t, retry, requestHost(t, http.MethodPost, host.URL()+"/worker-sessions/"+id+"/"+strings.ToLower(operation)))
		assertCLIControlParity(t, host, retry, id, strings.ToLower(operation))
		read := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id, "view": "events", "limit": 1})["result"].(map[string]any)
		assertJSONEqual(t, read["session"], getHost(t, host.URL()+"/worker-sessions/"+id))
		page := read["events"].(map[string]any)
		if len(page["events"].([]any)) != 1 || page["truncated"] != true {
			t.Fatalf("real retained stream did not report bounded prefix: %v", page)
		}
		select {
		case <-sibling:
			t.Fatal("control stopped the sibling provider execution")
		default:
		}
		peer := getHost(t, host.URL()+"/worker-sessions/sibling").(map[string]any)
		if peer["state"] != "RUNNING" && peer["state"] != "STARTING" {
			t.Fatalf("sibling state: %v", peer)
		}
	}
	unknown := callTool(t, ctx, session, "you.worker_session.read", map[string]any{"workerSessionId": "missing-runtime-worker"})
	assertToolError(t, unknown, "worker_session.not_found", false)
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.control", map[string]any{"workerSessionId": "missing-runtime-worker", "operation": "CANCEL"}), "worker_session.not_found", false)
	assertFleetPages(t, ctx, session, host)
	assertFactoryWithoutReference(t, ctx, session, host, dir, runner)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
}

func assertFactoryWithoutReference(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, dir string, runner controlHostRunner) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	support.SubmitSessionWorkAt(t, host.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "Factory worker without a provider reference"})
	var done <-chan struct{}
	select {
	case done = <-runner.started:
	case <-ctx.Done():
		t.Fatal("Factory provider boundary not reached")
	}
	fleet := callWorker(t, ctx, session, "list", map[string]any{"scope": "factory"})["result"].(map[string]any)
	assertRuntimeObservationParity(t, fleet, getHost(t, host.URL()+"/worker-sessions?scope=factory"))
	workers := fleet["sessions"].([]any)
	if len(workers) != 1 {
		t.Fatalf("no-reference Factory fleet: %v", fleet)
	}
	worker := workers[0].(map[string]any)
	id := worker["workerSessionId"].(string)
	if worker["direct"] != false || worker["providerSessionAvailable"] != false || worker["factorySessionId"] != opened.Session.Id {
		t.Fatalf("no-reference Factory observation: %v", worker)
	}
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id)
	read := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)
	assertRuntimeObservationParity(t, read["session"], getHost(t, endpoint))
	assertFactoryCLIParity(t, host, id, getHost(t, endpoint))
	all := callWorker(t, ctx, session, "list", map[string]any{})["result"].(map[string]any)
	if len(all["sessions"].([]any)) != 4 {
		t.Fatalf("combined direct/Factory fleet omitted workers: %v", all)
	}
	assertRuntimeObservationParity(t, all, getHost(t, host.URL()+"/worker-sessions"))
	assertCLIListParity(t, host, []string{"you", "worker-sessions", "list", "--server", host.URL(), "--json"}, getHost(t, host.URL()+"/worker-sessions"))
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "TERMINATE"})
	waitControlSignal(t, done)
}

func assertFleetPages(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer) {
	t.Helper()
	seen := map[string]bool{}
	token := ""
	for page := 0; page < 3; page++ {
		args := map[string]any{"limit": 1}
		query := url.Values{"limit": {"1"}}
		cliArgs := []string{"you", "worker-sessions", "list", "--server", host.URL(), "--json", "--limit", "1"}
		if token != "" {
			args["nextToken"] = token
			query.Set("nextToken", token)
			cliArgs = append(cliArgs, "--next-token", token)
		}
		result := callWorker(t, ctx, session, "list", args)["result"].(map[string]any)
		assertRuntimeObservationParity(t, result, getHost(t, host.URL()+"/worker-sessions?"+query.Encode()))
		assertCLIListParity(t, host, cliArgs, getHost(t, host.URL()+"/worker-sessions?"+query.Encode()))
		workers := result["sessions"].([]any)
		if len(workers) != 1 {
			t.Fatalf("page %d: %v", page, result)
		}
		id := workers[0].(map[string]any)["workerSessionId"].(string)
		if seen[id] {
			t.Fatalf("pagination repeated %s", id)
		}
		seen[id] = true
		token, _ = result["paginationContext"].(map[string]any)["nextToken"].(string)
		if (page == 2) != (token == "") {
			t.Fatalf("page %d cursor: %v", page, result)
		}
	}
	if !seen["sibling"] || !seen["mcp-cancel"] || !seen["mcp-terminate"] {
		t.Fatalf("pagination lost literal worker identities: %v", seen)
	}
	args := map[string]any{"scope": "direct", "state": []string{"COMPLETED"}}
	empty := callWorker(t, ctx, session, "list", args)["result"].(map[string]any)
	if len(empty["sessions"].([]any)) != 0 {
		t.Fatalf("empty filtered fleet: %v", empty)
	}
	endpoint := host.URL() + "/worker-sessions?scope=direct&state=COMPLETED"
	assertJSONEqual(t, empty, getHost(t, endpoint))
	assertCLIListParity(t, host, []string{"you", "worker-sessions", "list", "--server", host.URL(), "--json", "--scope", "direct", "--state", "COMPLETED"}, empty)
}

func assertCLIListParity(t *testing.T, host *support.FunctionalAPIServer, args []string, expected any) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI list: %v stderr=%s", err, inputs.Stderr())
	}
	var actual any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &actual); err != nil {
		t.Fatal(err)
	}
	normalizeActiveDuration(t, actual)
	normalizeActiveDuration(t, expected)
	pages := make([]factoryapi.ListWorkerSessionsResponse, 2)
	for index, value := range []any{actual, expected} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &pages[index]); err != nil {
			t.Fatal(err)
		}
	}
	assertJSONEqual(t, pages[0], pages[1])
}

// ACTIVE_CLOCK duration advances between public reads. Check each sample's
// contract independently, then compare all remaining fields exactly.
func assertRuntimeObservationParity(t *testing.T, actual, expected any) {
	t.Helper()
	normalizeActiveDuration(t, actual)
	normalizeActiveDuration(t, expected)
	assertJSONEqual(t, actual, expected)
}

func normalizeActiveDuration(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		if value["durationBasis"] == "ACTIVE_CLOCK" {
			duration, ok := value["durationMillis"].(float64)
			if !ok || duration < 0 {
				t.Fatalf("invalid active duration: %v", value)
			}
			delete(value, "durationMillis")
		}
		for _, child := range value {
			normalizeActiveDuration(t, child)
		}
	case []any:
		for _, child := range value {
			normalizeActiveDuration(t, child)
		}
	}
}

func admitControlWorker(t *testing.T, ctx context.Context, host, id string, runner controlHostRunner) <-chan struct{} {
	t.Helper()
	payload := map[string]any{
		"requestId": "request-" + id, "workerSessionId": id,
		"execution": map[string]any{"workstationName": "process", "workerType": "processor", "dispatch": map[string]any{"dispatchId": "attempt-" + id, "workstationName": "process", "workerType": "processor"}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/worker-sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start status %d: %s", response.StatusCode, body)
	}
	select {
	case done := <-runner.started:
		return done
	case <-ctx.Done():
		t.Fatal("provider boundary was not reached")
	}
	return nil
}

func waitControlSignal(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("owned provider execution did not stop")
	}
}

func assertOwnedControl(t *testing.T, result map[string]any, id, operation, outcome string) {
	t.Helper()
	if result["workerSessionId"] != id || result["dispatchId"] != "attempt-"+id || result["action"] != operation || result["outcome"] != outcome {
		t.Fatalf("exact target control result: %v", result)
	}
}

func assertCLIControlParity(t *testing.T, host *support.FunctionalAPIServer, expected any, id, operation string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", operation, id, "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI control: %v, stderr=%s", err, inputs.Stderr())
	}
	var actual any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &actual); err != nil {
		t.Fatalf("CLI JSON: %v, output=%s", err, inputs.Stdout())
	}
	assertJSONEqual(t, actual, expected)
}

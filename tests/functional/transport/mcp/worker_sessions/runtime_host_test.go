package workersessions_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	assertRuntimeObservationParity(t, listed, getHost(t, host.URL()+"/worker-sessions?history=all&scope=factory"))
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
		FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	sibling := admitControlWorker(t, ctx, host.URL(), "sibling", runner)
	for _, operation := range []string{"CANCEL", "TERMINATE"} {
		id := "mcp-" + strings.ToLower(operation)
		done := admitControlWorker(t, ctx, host.URL(), id, runner)
		listed := callWorker(t, ctx, session, "list", map[string]any{"scope": "direct"})["result"]
		assertRuntimeObservationParity(t, listed, getHost(t, host.URL()+"/worker-sessions?history=all&scope=direct"))
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
	assertRuntimeObservationParity(t, fleet, getHost(t, host.URL()+"/worker-sessions?history=all&scope=factory"))
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
	assertRuntimeObservationParity(t, all, getHost(t, host.URL()+"/worker-sessions?history=all"))
	assertCLIListParity(t, host, []string{"you", "worker-sessions", "list", "--server", host.URL(), "--json", "--history", "all"}, getHost(t, host.URL()+"/worker-sessions?history=all"))
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "TERMINATE"})
	waitControlSignal(t, done)
}

func assertFleetPages(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer) {
	t.Helper()
	seen := map[string]bool{}
	token := ""
	for page := 0; page < 3; page++ {
		args := map[string]any{"limit": 1}
		query := url.Values{"limit": {"1"}, "history": {"all"}}
		cliArgs := []string{"you", "worker-sessions", "list", "--server", host.URL(), "--json", "--history", "all", "--limit", "1"}
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
	endpoint := host.URL() + "/worker-sessions?history=all&scope=direct&state=COMPLETED"
	assertJSONEqual(t, empty, getHost(t, endpoint))
	assertCLIListParity(t, host, []string{"you", "worker-sessions", "list", "--server", host.URL(), "--json", "--history", "all", "--scope", "direct", "--state", "COMPLETED"}, empty)
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
	expected = cloneHistoryValue(t, expected)
	normalizeActiveDuration(t, actual)
	normalizeActiveDuration(t, expected)
	assertIndependentHistoryCursors(t, actual, expected)
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
	actual = cloneHistoryValue(t, actual)
	expected = cloneHistoryValue(t, expected)
	assertIndependentHistoryCursors(t, actual, expected)
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

// This scenario needs a stable admission window to assert fleet membership.
// Its host/profile is isolated while the other MCP scenarios run in parallel.
func runRealHostHistory(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-history-host")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 4)}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)}})
	session, ctx := startMCP(t, process, host.URL())
	firstDone := admitControlWorker(t, ctx, host.URL(), "history-a", runner)
	secondDone := admitControlWorker(t, ctx, host.URL(), "history-b", runner)
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	support.SubmitSessionWorkAt(t, host.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "hold history Factory worker"})
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatal("Factory provider boundary not reached")
	}
	active := historyParityPage(t, ctx, session, host, "active", "all", "")
	if len(active["sessions"].([]any)) != 3 {
		t.Fatalf("mixed active history = %v", active)
	}
	factoryCount := 0
	for _, value := range active["sessions"].([]any) {
		worker := value.(map[string]any)
		if worker["direct"] == false {
			assertRecordedProviderBeforeAssociation(t, worker)
			factoryCount++
			if worker["factorySessionId"] != opened.Session.Id {
				t.Fatalf("Factory attribution = %v", worker)
			}
		}
	}
	if factoryCount != 1 {
		t.Fatalf("Factory active count = %d", factoryCount)
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "history-a", "operation": "TERMINATE"})
	waitControlSignal(t, firstDone)
	archived := historyParityPage(t, ctx, session, host, "archived", "direct", "")
	if workers := archived["sessions"].([]any); len(workers) != 1 || workers[0].(map[string]any)["workerSessionId"] != "history-a" {
		t.Fatalf("archived history = %v", archived)
	}
	page := callWorker(t, ctx, session, "list", map[string]any{"history": "all", "scope": "direct", "limit": 1})["result"].(map[string]any)
	token := page["paginationContext"].(map[string]any)["nextToken"].(string)
	if workers := page["sessions"].([]any); len(workers) != 1 || workers[0].(map[string]any)["workerSessionId"] != "history-a" {
		t.Fatalf("first frozen page = %v", page)
	}
	laterDone := admitControlWorker(t, ctx, host.URL(), "history-c", runner)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "history-b", "operation": "TERMINATE"})
	waitControlSignal(t, secondDone)
	frozen := historyParityPage(t, ctx, session, host, "all", "direct", token)
	workers := frozen["sessions"].([]any)
	if len(workers) != 1 || workers[0].(map[string]any)["workerSessionId"] != "history-b" || workers[0].(map[string]any)["state"] != "RUNNING" {
		t.Fatalf("frozen continuation = %v", frozen)
	}
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.list", map[string]any{"history": "archived", "scope": "direct", "nextToken": token}), "worker_session.invalid_request", false)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "history-c", "operation": "TERMINATE"})
	waitControlSignal(t, laterDone)
}

func assertRecordedProviderBeforeAssociation(t *testing.T, worker map[string]any) {
	t.Helper()
	if worker["provider"] != "codex" || worker["providerSessionAvailable"] != false {
		t.Fatalf("provider binding before association lost: %v", worker)
	}
}

func historyParityPage(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, history, scope, token string) map[string]any {
	t.Helper()
	args := map[string]any{"history": history, "scope": scope}
	query := url.Values{"history": {history}, "scope": {scope}}
	cliArgs := []string{"you", "worker-sessions", "list", "--server", host.URL(), "--history", history, "--scope", scope, "--json"}
	if token != "" {
		args["nextToken"] = token
		query.Set("nextToken", token)
		cliArgs = append(cliArgs, "--next-token", token)
	}
	actual := callWorker(t, ctx, session, "list", args)["result"].(map[string]any)
	assertRuntimeObservationParity(t, actual, getHost(t, host.URL()+"/worker-sessions?"+query.Encode()))
	assertCLIListParity(t, host, cliArgs, actual)
	return actual
}

type historyWorkingDirectory string

func (dir historyWorkingDirectory) Getwd() (string, error) { return string(dir), nil }

// Reopening is sequential because both hosts use the same customer-selected
// capture directory. The prior-owner prefix is a controlled crash fixture;
// the ended sibling crosses real admission, capture and joined termination.
func runRealHostHistoryRecovery(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "history-recovery")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 1)}
	var store recordings.WorkerRecordingStore
	cfg := support.FunctionalAPIServerConfig{FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: serviceedges.Edges{
		ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir),
		WorkerRecordingStoreObserver: func(value recordings.WorkerRecordingStore) { store = value },
	}}
	host := support.StartFunctionalAPIServer(t, cfg)
	session, ctx := startMCP(t, process, host.URL())
	done := admitControlWorker(t, ctx, host.URL(), "ended-sibling", runner)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "ended-sibling", "operation": "TERMINATE"})
	waitControlSignal(t, done)
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	support.SubmitSessionWorkAt(t, host.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "Factory archive recovery"})
	select {
	case done = <-runner.started:
	case <-ctx.Done():
		t.Fatal("Factory recovery provider boundary not reached")
	}
	factoryPage := historyParityPage(t, ctx, session, host, "active", "factory", "")
	factoryWorkers := factoryPage["sessions"].([]any)
	if len(factoryWorkers) != 1 {
		t.Fatalf("Factory recovery admission: %v", factoryPage)
	}
	factoryID := factoryWorkers[0].(map[string]any)["workerSessionId"].(string)
	postHostJSON(t, ctx, host.URL()+"/factory-sessions/"+opened.Session.Id+"/pause", map[string]any{}, http.StatusOK)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": factoryID, "operation": "TERMINATE"})
	waitControlSignal(t, done)
	seedHistoryCrashCapture(t, ctx, store)
	prefix := getHost(t, host.URL()+"/worker-sessions/lost-worker/logs")
	host.Close(t)
	// Inject a torn final append and an undecodable complete line only after
	// the original writer is joined. Observe recovery through public reads.
	injectHistoryCaptureTails(t, dir)
	// Different owner epoch, identical durable root, no native provider files.
	reopened := support.StartFunctionalAPIServer(t, cfg)
	recoveredSession, recoveredCtx := startMCP(t, process, reopened.URL())
	active := historyParityPage(t, recoveredCtx, recoveredSession, reopened, "active", "all", "")
	if len(active["sessions"].([]any)) != 0 {
		t.Fatalf("restart restored an execution owner: %v", active)
	}
	archived := historyParityPage(t, recoveredCtx, recoveredSession, reopened, "archived", "all", "")
	if len(archived["sessions"].([]any)) != 3 {
		t.Fatalf("restart omitted captured siblings: %v", archived)
	}
	factoryArchive := historyParityPage(t, recoveredCtx, recoveredSession, reopened, "archived", "factory", "")
	assertArchivedProviderRecovery(t, reopened, recoveredCtx, recoveredSession, factoryArchive)
	if workers := factoryArchive["sessions"].([]any); len(workers) != 1 || workers[0].(map[string]any)["workerSessionId"] != factoryID || workers[0].(map[string]any)["factorySessionId"] != opened.Session.Id {
		t.Fatalf("restart lost Factory capture attribution: %v", factoryArchive)
	}
	selected := getHost(t, reopened.URL()+"/worker-sessions/lost-worker").(map[string]any)
	if selected["state"] != "FAILED" || selected["confirmationState"] != "UNCONFIRMED" || selected["recordingHealth"] != "INCOMPLETE" || selected["failure"].(map[string]any)["kind"] != "PROCESS_GONE" {
		t.Fatalf("unfinished history invented live or complete state: %v", selected)
	}
	for _, value := range archived["sessions"].([]any) {
		observation := value.(map[string]any)
		if observation["workerSessionId"] == "lost-worker" {
			assertJSONEqual(t, selected, observation)
		}
	}
	read := callWorker(t, recoveredCtx, recoveredSession, "read", map[string]any{"workerSessionId": "lost-worker"})["result"].(map[string]any)
	assertJSONEqual(t, selected, read["session"])
	assertFactoryCLIParity(t, reopened, "lost-worker", selected)
	assertJSONEqual(t, prefix, getHost(t, reopened.URL()+"/worker-sessions/lost-worker/logs"))
	assertToolError(t, callTool(t, recoveredCtx, recoveredSession, "you.worker_session.control", map[string]any{"workerSessionId": "lost-worker", "operation": "CANCEL"}), "worker_session.not_found", false)
	assertToolError(t, callTool(t, recoveredCtx, recoveredSession, "you.worker_session.read", map[string]any{"workerSessionId": "damaged-worker"}), "worker_session.internal_error", false)
	assertHistoryReadFailure(t, reopened, "damaged-worker", http.StatusInternalServerError, "PROJECTION_UNAVAILABLE")
	// An independent profile has no captured identity and cannot consume either
	// a fleet snapshot or log cursor belonging to the recovered profile.
	cfg.Edges.FactorySessionsWorkingDirectory = historyWorkingDirectory(t.TempDir())
	foreign := support.StartFunctionalAPIServer(t, cfg)
	foreignSession, foreignCtx := startMCP(t, process, foreign.URL())
	assertHistoryReadFailure(t, foreign, "lost-worker", http.StatusNotFound, "NOT_FOUND")
	assertToolError(t, callTool(t, foreignCtx, foreignSession, "you.worker_session.read", map[string]any{"workerSessionId": "lost-worker"}), "worker_session.not_found", false)
	page := callWorker(t, recoveredCtx, recoveredSession, "list", map[string]any{"history": "archived", "limit": 1})["result"].(map[string]any)
	token := page["paginationContext"].(map[string]any)["nextToken"].(string)
	assertToolError(t, callTool(t, foreignCtx, foreignSession, "you.worker_session.list", map[string]any{"history": "archived", "nextToken": token}), "worker_session.invalid_request", false)
}

func assertArchivedProviderRecovery(t *testing.T, host *support.FunctionalAPIServer, ctx context.Context, session *mcp.ClientSession, page map[string]any) {
	t.Helper()
	for _, value := range page["sessions"].([]any) {
		observation := value.(map[string]any)
		if observation["provider"] != "codex" {
			t.Fatalf("restart lost recorded provider: %v", observation)
		}
		id := observation["workerSessionId"].(string)
		selected := getHost(t, host.URL()+"/worker-sessions/"+url.PathEscape(id))
		assertJSONEqual(t, observation, selected)
		assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
		assertFactoryCLIParity(t, host, id, selected)
		assertArchivedHostTiming(t, host, id)
	}
}

func assertArchivedHostTiming(t *testing.T, host *support.FunctionalAPIServer, id string) {
	t.Helper()
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id)
	observation := support.GetJSON[factoryapi.WorkerSessionObservation](t, endpoint)
	logs := support.GetJSON[factoryapi.WorkerSessionLogPage](t, endpoint+"/logs")
	stamp := logs.Events[len(logs.Events)-1].Event.CapturedAt
	if stamp == nil || observation.StartedAt == nil || observation.EndedAt == nil || !observation.EndedAt.Equal(*stamp) || observation.DurationMillis == nil || *observation.DurationMillis != stamp.Sub(*observation.StartedAt).Milliseconds() || observation.DurationBasis != "RECORDED_TIMESTAMPS" {
		t.Fatalf("recovered Factory summary lost host capture timing: %+v terminal=%v", observation, stamp)
	}
}

func seedHistoryCrashCapture(t *testing.T, ctx context.Context, store recordings.WorkerRecordingStore) {
	t.Helper()
	opening := events.Record{
		ID:         events.RecordID{Topic: "worker-session/lost-worker/events", Position: 1},
		SourceType: "worker_session_lifecycle", SourceID: "lost-worker", SourceSequence: 1,
		SourceEventID: "started", SchemaID: "workers.draft.v1",
		Payload: json.RawMessage(`{"kind":"SESSION","phase":"STARTED","provenance":{"delivery":"SYNTHESIZED","fidelity":"LIFECYCLE_ONLY","nativeEventType":"worker_session_lifecycle","representation":"NOTIFICATION"},"payload":{"status":"STARTING","workerSessionId":"lost-worker","attemptId":"lost-attempt"}}`),
	}
	if err := store.PersistWorkerRecord(ctx, recordings.WorkerRecordingRecord{RecordingID: "lost-recording", WorkerSessionID: "lost-worker", Record: opening}); err != nil {
		t.Fatal(err)
	}
	damagedOpening := opening.Detached()
	damagedOpening.ID.Topic = "worker-session/damaged-worker/events"
	damagedOpening.SourceID = "damaged-worker"
	damagedOpening.Payload = bytes.ReplaceAll(opening.Payload, []byte("lost-worker"), []byte("damaged-worker"))
	if err := store.PersistWorkerRecord(ctx, recordings.WorkerRecordingRecord{RecordingID: "damaged-recording", WorkerSessionID: "damaged-worker", Record: damagedOpening}); err != nil {
		t.Fatal(err)
	}
}

func injectHistoryCaptureTails(t *testing.T, dir string) {
	t.Helper()
	for id, tail := range map[string]string{"lost-recording": `{"uncommitted":`, "damaged-recording": "not json\n"} {
		digest := sha256.Sum256([]byte(id))
		path := filepath.Join(dir, ".you-agent-factory", "worker-recordings", hex.EncodeToString(digest[:])+".worker.jsonl")
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.WriteString(tail)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("inject capture failure: %v %v", writeErr, closeErr)
		}
	}
}

func assertHistoryReadFailure(t *testing.T, host *support.FunctionalAPIServer, id string, status int, code string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.URL()+"/worker-sessions/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status || body["code"] != code {
		t.Fatalf("selected history error: %d %v", response.StatusCode, body)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "show", "--worker-session-id", id, "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err == nil || !strings.Contains(inputs.Stderr(), code) {
		t.Fatalf("CLI selected history error: %v %s", err, inputs.Stderr())
	}
}

func cloneHistoryValue(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var copy any
	if err := json.Unmarshal(encoded, &copy); err != nil {
		t.Fatal(err)
	}
	return copy
}

// Independent first-page requests allocate distinct signed snapshots. Compare
// the presence of continuation, then their public observations and bounds.
// The scenarios replay each returned token across HTTP, CLI and MCP separately.
func assertIndependentHistoryCursors(t *testing.T, actual, expected any) {
	t.Helper()
	left, lok := actual.(map[string]any)
	right, rok := expected.(map[string]any)
	if !lok || !rok || left["sessions"] == nil || right["sessions"] == nil {
		return
	}
	lp, lok := left["paginationContext"].(map[string]any)
	rp, rok := right["paginationContext"].(map[string]any)
	if !lok || !rok {
		return
	}
	lt, _ := lp["nextToken"].(string)
	rt, _ := rp["nextToken"].(string)
	if (lt == "") != (rt == "") {
		t.Fatalf("continuation presence differs: %v / %v", lp, rp)
	}
	delete(lp, "nextToken")
	delete(rp, "nextToken")
}

package workersessions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F-02 crosses real Work admission, provider supervision and synced capture.
// Two explicit Factory Sessions share one root-built host and one immutable
// command edge; this fleet-wide observation owns an isolated profile. Provider
// release is signal-driven. The public terminal-status observer waits for the
// runtime's committed result, since command return precedes capture completion.
func runArchivedWorkAttributionCompleted(t *testing.T, process support.Process) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "completed-attribution")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	runner := completingAttributionRunner{controlHostRunner{started: make(chan (<-chan struct{}), 2)}, release}
	var sessions support.FactorySessionStarter
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
		BeforeStart: func(_ testing.TB, p support.Process, _ root.Input) {
			sessions = p.(support.ApplicationProcess).FactorySessions()
		},
	})
	session, ctx := startMCP(t, process, host.URL())
	var scopes []string
	for _, name := range []string{"completed-alpha", "completed-beta"} {
		scopes = append(scopes, admitRecordedAttributionWork(t, ctx, sessions, host, runner.controlHostRunner, dir, name))
	}
	live := historyParityPage(t, ctx, session, host, "active", "factory", "")["sessions"].([]any)
	if len(live) != 2 {
		t.Fatalf("gated live membership: %v", live)
	}
	if archived := historyParityPage(t, ctx, session, host, "archived", "factory", ""); len(archived["sessions"].([]any)) != 0 {
		t.Fatalf("live owners leaked into archive: %v", archived)
	}
	first := callWorker(t, ctx, session, "list", map[string]any{"history": "all", "scope": "factory", "limit": 1})["result"].(map[string]any)
	token := first["paginationContext"].(map[string]any)["nextToken"].(string)
	if len(first["sessions"].([]any)) != 1 || token == "" {
		t.Fatalf("all view did not create bounded snapshot: %v", first)
	}
	frozen := historyParityPage(t, ctx, session, host, "all", "factory", token)
	if len(frozen["sessions"].([]any)) != 1 {
		t.Fatalf("all continuation membership: %v", frozen)
	}
	unblock()
	for _, scope := range scopes {
		status := support.WaitForSessionTerminalStatus(t, host.URL(), scope, 30*time.Second)
		if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
			t.Fatalf("provider completion did not finish named Work: %+v", status)
		}
	}
	for _, view := range []string{"archived", "all"} {
		page := historyParityPage(t, ctx, session, host, view, "factory", "")
		assertCompletedAttributionRows(t, page, live)
	}
	assertJSONEqual(t, frozen, historyParityPage(t, ctx, session, host, "all", "factory", token))
	if active := historyParityPage(t, ctx, session, host, "active", "factory", ""); len(active["sessions"].([]any)) != 0 {
		t.Fatalf("completed attempts remained active: %v", active)
	}
}

// The command-edge provider completes with an accepted envelope whose proposed
// Work state is invalid. Runtime rejects the business result after capture ends.
// Restart requires a fresh graph in the same private
// profile; Worker completion must remain distinct from failed Work.
func TestRestoredFactorySummaryCompletedCaptureWithFailedWork(t *testing.T) {
	t.Parallel()
	process := newSelectedHostClientProcess(t)
	dir := support.ScaffoldSingleStepFactory(t, "completed-worker-failed-work")
	configureFailedBusinessAttribution(t, dir)
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: AGENT_WORKER\nmodelProvider: CODEX\nmodel: gpt-5\nexecutorProvider: SCRIPT_WRAP\n---\nReturn a short reply.\n")
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: AGENT_RUN\noutcomeFormat: decision-envelope\nlimits:\n  maxExecutionTime: 1h\n---\nReturn an accepted decision envelope.\n")
	cfg := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Args:  []string{"--record", filepath.Join(dir, "failed-work.json")},
		Edges: serviceedges.Edges{ProviderCommandRunner: failedBusinessAttributionRunner{}, ScriptCommandRunner: rejectLocalProvider{t: t}, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	}
	host := support.StartFunctionalAPIServer(t, cfg)
	connection, ctx := startMCP(t, process, host.URL())
	scope := support.GetDefaultSession(t, host.URL()).Id
	name := "completed-worker"
	support.SubmitSessionWorkAt(t, host.URL(), scope, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Name: &name, Payload: "complete the Worker before processing the Work result"})
	status := support.WaitForSessionTerminalStatus(t, host.URL(), scope, 30*time.Second)
	if status.Categories.Failed != 1 {
		t.Fatalf("invalid proposed Work did not fail business Work: %+v", status)
	}
	// Archive is capture-derived even while the original runtime remains open.
	before := historyParityPage(t, ctx, connection, host, "archived", "factory", "")["sessions"].([]any)[0].(map[string]any)
	id := before["workerSessionId"].(string)
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id)
	assertCompletedFailedWorkCapture(t, before, scope, id)
	workID := before["workId"].(string)
	assertFailedAttributionWork(t, host, scope, workID)
	logs := getHost(t, endpoint+"/logs")
	encodedLogs, err := json.Marshal(logs)
	if err != nil || !strings.Contains(string(encodedLogs), "completed capture marker") {
		t.Fatalf("capture lost completed output: %v %v", logs, err)
	}
	transcript := getHost(t, endpoint+"/transcript")
	assertCompletedAttributionTranscript(t, transcript, id)
	host.Close(t)
	cfg.Args = []string{"--record", filepath.Join(dir, "failed-work.json")}
	siblingRunner := controlHostRunner{started: make(chan (<-chan struct{}), 1)}
	cfg.Edges.ProviderCommandRunner = siblingRunner
	host = support.StartFunctionalAPIServer(t, cfg)
	connection, ctx = startMCP(t, process, host.URL())
	assertFailedAttributionWork(t, host, support.GetDefaultSession(t, host.URL()).Id, workID)
	selected := assertRestoredCompletedAttributionReads(t, ctx, connection, host, before, logs, transcript)
	assertRestoredCompletedAttributionControls(t, ctx, connection, host, selected, siblingRunner)
}

func assertRestoredCompletedAttributionReads(t *testing.T, ctx context.Context, connection *mcp.ClientSession, host *support.FunctionalAPIServer, before map[string]any, logs, transcript any) map[string]any {
	t.Helper()
	id, scope := before["workerSessionId"].(string), before["factorySessionId"].(string)
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id)
	selected := getHost(t, endpoint).(map[string]any)
	assertCompletedFailedWorkCapture(t, selected, scope, id)
	for _, field := range []string{"attemptId", "startedAt", "endedAt", "durationMillis"} {
		if selected[field] == nil || selected[field] != before[field] {
			t.Fatalf("restart changed captured %s: before=%v after=%v", field, before, selected)
		}
	}
	assertJSONEqual(t, selected, getHost(t, host.URL()+"/factory-sessions/"+scope+"/worker-sessions/"+id))
	assertRuntimeObservationParity(t, selected, callWorker(t, ctx, connection, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
	assertFactoryCLIParity(t, host, id, selected)
	inputs := support.FakeInputs(ctx, []string{"you", "worker-sessions", "show", "--session", scope, "--worker-session-id", id, "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("original scope CLI: %v stderr=%s", err, inputs.Stderr())
	}
	var scoped map[string]any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &scoped); err != nil {
		t.Fatal(err)
	}
	assertCompletedFailedWorkCapture(t, scoped, scope, id)
	archived := historyParityPage(t, ctx, connection, host, "archived", "factory", "")["sessions"].([]any)
	if len(archived) != 1 {
		t.Fatalf("restart archive membership: %v", archived)
	}
	assertJSONEqual(t, selected, archived[0])
	assertJSONEqual(t, logs, getHost(t, endpoint+"/logs"))
	assertJSONEqual(t, transcript, getHost(t, endpoint+"/transcript"))
	assertTranscriptEqual(t, transcript, callWorker(t, ctx, connection, "read", map[string]any{"workerSessionId": id, "view": "transcript"})["result"].(map[string]any)["transcript"])
	assertCompletedAttributionCLITranscript(t, host, id, transcript)
	return selected
}

func assertRestoredCompletedAttributionControls(t *testing.T, ctx context.Context, connection *mcp.ClientSession, host *support.FunctionalAPIServer, selected map[string]any, siblingRunner controlHostRunner) {
	t.Helper()
	id := selected["workerSessionId"].(string)
	done := admitControlWorker(t, ctx, host.URL(), "new-unrelated-owner", siblingRunner)
	assertCompletedAttributionControls(t, host, id)
	for _, operation := range []string{"CANCEL", "TERMINATE", "KILL", "INTERRUPT"} {
		args := map[string]any{"workerSessionId": id, "operation": operation, "requestId": "archived-" + operation}
		if operation == "KILL" {
			args["expectedAttemptId"] = selected["attemptId"]
		}
		if operation == "INTERRUPT" {
			args["successorWorkerSessionId"], args["replacementMessage"] = "must-not-exist", "must not revive"
		}
		if operation == "CANCEL" || operation == "TERMINATE" {
			delete(args, "requestId")
			result := callWorker(t, ctx, connection, "control", args)["result"].(map[string]any)
			assertCompletedAttributionNoop(t, result, id)
			continue
		}
		assertToolError(t, callAction(t, ctx, connection, "CONTROL", args), "worker_session.not_found", false)
	}
	assertForceStillActive(t, host, "new-unrelated-owner", done)
	assertJSONEqual(t, selected, getHost(t, host.URL()+"/worker-sessions/"+url.PathEscape(id)))
	assertHistoryReadFailure(t, host, "must-not-exist", http.StatusNotFound, "NOT_FOUND")
}

func assertCompletedAttributionTranscript(t *testing.T, transcript any, id string) {
	t.Helper()
	row := transcript.(map[string]any)
	ref, ok := row["providerSession"].(map[string]any)
	if !ok || ref["provider"] != "codex" || ref["id"] != "1453c576-a18d-4a03-8099-ce6e92bd1a2b" || row["workerSessionId"] != id {
		t.Fatalf("associated transcript identity: %v", row)
	}
	entries, ok := row["entries"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("associated transcript lost captured content: %v", row)
	}
	for index, entry := range entries {
		item := entry.(map[string]any)
		if item["order"] != float64(index+1) || !strings.Contains(item["text"].(string), "completed capture marker") {
			t.Fatalf("associated transcript order/content: %v", entries)
		}
	}
}

func assertCompletedAttributionCLITranscript(t *testing.T, host *support.FunctionalAPIServer, id string, want any) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--server", host.URL(), "--view", "transcript", "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("associated transcript CLI: %v %s", err, inputs.Stderr())
	}
	var got any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &got); err != nil {
		t.Fatal(err)
	}
	assertTranscriptEqual(t, want, got)
}

func assertCompletedAttributionControls(t *testing.T, host *support.FunctionalAPIServer, id string) {
	t.Helper()
	for _, action := range []string{"cancel", "terminate"} {
		result := postHostJSON(t, t.Context(), host.URL()+"/worker-sessions/"+id+"/"+action, map[string]any{}, http.StatusOK).(map[string]any)
		assertCompletedAttributionNoop(t, result, id)
		inputs := support.FakeInputs(t.Context(), []string{"you", "--remote", "--server", host.URL(), "--json", "worker-sessions", action, id})
		if err := host.Execute(t, inputs.Input); err != nil {
			t.Fatalf("archived CLI %s: %v %s", action, err, inputs.Stderr())
		}
		if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
			t.Fatal(err)
		}
		assertCompletedAttributionNoop(t, result, id)
	}
	postHostJSON(t, t.Context(), host.URL()+"/worker-sessions/"+id+"/terminate", map[string]any{"force": true, "requestId": "archived-kill", "expectedAttemptId": id}, http.StatusNotFound)
	postHostJSON(t, t.Context(), host.URL()+"/worker-sessions/"+id+"/interrupt", interruptModePayload("must not revive", ""), http.StatusNotFound)
}

func assertCompletedAttributionNoop(t *testing.T, result map[string]any, id string) {
	t.Helper()
	if result["workerSessionId"] != id || result["outcome"] != "NOOP" {
		t.Fatalf("archived control acquired authority: %v", result)
	}
}

func assertFailedAttributionWork(t *testing.T, host *support.FunctionalAPIServer, scope, workID string) {
	t.Helper()
	row := getHost(t, host.URL()+"/factory-sessions/"+scope+"/work/"+url.PathEscape(workID)).(map[string]any)
	state, ok := row["state"].(map[string]any)
	if !ok || state["type"] != "FAILED" {
		t.Fatalf("Worker summary repair hid business Work failure: %v", row)
	}
}

type failedBusinessAttributionRunner struct{}

func configureFailedBusinessAttribution(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	station := config["workstations"].([]any)[0].(map[string]any)
	station["outcomeFormat"] = "decision-envelope"
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (failedBusinessAttributionRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	message := `{"decision":"ACCEPTED","output":"completed capture marker","recorded_output_work":[{"workTypeId":"task","state":"unknown-business-state","content":[{"type":"text","text":"invalid proposal"}]}]}`
	item, err := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"id": "result", "type": "agent_message", "text": message}})
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	output := "{\"type\":\"thread.started\",\"thread_id\":\"1453c576-a18d-4a03-8099-ce6e92bd1a2b\"}\n" + string(item) + "\n" +
		"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}\n"
	return platformprocess.CommandResult{Stdout: []byte(output)}, nil
}

func (runner failedBusinessAttributionRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if err == nil && observer != nil {
		observer(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func assertCompletedFailedWorkCapture(t *testing.T, row map[string]any, scope, id string) {
	t.Helper()
	if row["workerSessionId"] != id || row["factorySessionId"] != scope || row["state"] != "COMPLETED" || row["terminalCause"] != "COMPLETED" || row["recordingHealth"] != "COMPLETE" || row["provider"] != "codex" || row["model"] != "gpt-5" {
		t.Fatalf("completed Worker facts replaced by business Work failure: %v", row)
	}
	usage, ok := row["tokenUsage"].(map[string]any)
	// Codex's command stream reports input/output counters without a total.
	// The supplied declarative-artifact journey must separately prove total=18.
	if !ok || usage["inputTokens"] != float64(11) || usage["outputTokens"] != float64(7) {
		t.Fatalf("captured usage changed: %v", row)
	}
}

// H-08 owns a smaller isolated fleet window: 96 named scoped recordings
// share a host and one intentional restart. Fixture setup precedes measurement.
// The packet explicitly permits the five-second functional latency assertion.
func runArchivedWorkAttributionManyHistories(t *testing.T, process support.Process) {
	t.Parallel()
	host, sessions, runner, dir := startRecordedAttributionHost(t)
	connection, ctx := startMCP(t, process, host.URL())
	expected := make(map[string]retainedHistoryFacts)
	var paths []string
	for index := range 96 {
		name := fmt.Sprintf("retained-history-%02d", index)
		scope := admitRecordedAttributionWork(t, ctx, sessions, host, runner, dir, name)
		live := historyParityPage(t, ctx, connection, host, "active", "factory", "")["sessions"].([]any)
		if len(live) != 1 {
			t.Fatalf("seed active membership: %v", live)
		}
		id := live[0].(map[string]any)["workerSessionId"].(string)
		expected[id] = retainedHistoryFacts{name, "CANCELED", "COMPLETE"}
		postHostJSON(t, ctx, host.URL()+"/factory-sessions/"+scope+"/pause", map[string]any{}, 200)
		callWorker(t, ctx, connection, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
		support.CloseFactorySessionAt(t, host.URL(), scope)
		paths = append(paths, filepath.Join(dir, name+".json"))
	}
	host.Close(t)
	var total int
	for _, path := range paths {
		total += growRetainedWorkPayload(t, path, 1024)
	}
	if total > 8<<20 {
		t.Fatalf("small fixture exceeded 8 MiB: %d", total)
	}
	t.Logf("H-08 fixture: recordings=%d bytes=%d", len(paths), total)
	host = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
		BeforeStart: func(_ testing.TB, p support.Process, _ root.Input) {
			sessions = p.(support.ApplicationProcess).FactorySessions()
		},
	})
	connection, ctx = startMCP(t, process, host.URL())
	admitRecordedAttributionWork(t, ctx, sessions, host, runner, dir, "live-history-owner")
	active := historyParityPage(t, ctx, connection, host, "active", "factory", "")["sessions"].([]any)
	if len(active) != 1 {
		t.Fatalf("live owner membership after reopen: %v", active)
	}
	live := active[0].(map[string]any)
	allExpected := make(map[string]retainedHistoryFacts, len(expected)+1)
	for id, facts := range expected {
		allExpected[id] = facts
	}
	allExpected[live["workerSessionId"].(string)] = retainedHistoryFacts{"live-history-owner", live["state"].(string), live["recordingHealth"].(string)}
	assertMeasuredRetainedLists(t, host, expected, allExpected)
	assertManyRetainedContinuation(t, host, expected, allExpected)
	for _, query := range []string{"", "?history=active"} {
		page, err := readAttributionList(t.Context(), host.URL()+"/worker-sessions"+query)
		if err != nil {
			t.Fatal(err)
		}
		assertManyRetainedRows(t, page, map[string]retainedHistoryFacts{
			live["workerSessionId"].(string): allExpected[live["workerSessionId"].(string)],
		})
	}
}

type retainedHistoryFacts struct{ name, state, health string }

func assertMeasuredRetainedLists(t *testing.T, host *support.FunctionalAPIServer, archived, all map[string]retainedHistoryFacts) {
	t.Helper()
	for _, view := range []string{"archived", "all"} {
		expected := archived
		if view == "all" {
			expected = all
		}
		started := time.Now()
		page, err := readAttributionList(t.Context(), host.URL()+"/worker-sessions?history="+view+"&limit=100")
		elapsed := time.Since(started)
		t.Logf("%s consumed GET: %s", view, elapsed)
		if err != nil {
			t.Fatal(err)
		}
		//nolint:testsleep // H-08 explicitly requires consumed output within five seconds.
		if elapsed > 5*time.Second {
			t.Fatalf("%s GET took %s, require <=5s", view, elapsed)
		}
		assertManyRetainedRows(t, page, expected)
		if view != "archived" {
			continue
		}
		inputs := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "worker-sessions", "list", "--history", view, "--limit", "100", "--json"})
		started = time.Now()
		if err := host.Execute(t, inputs.Input); err != nil {
			t.Fatalf("%s CLI within unchanged timeout: %v %s", view, err, inputs.Stderr())
		}
		elapsed = time.Since(started)
		t.Logf("%s CLI: %s", view, elapsed)
		//nolint:testsleep // H-08 also bounds full archived CLI output.
		if elapsed > 5*time.Second {
			t.Fatalf("archived CLI took %s, require <=5s", elapsed)
		}
		page = nil
		if err := json.Unmarshal([]byte(inputs.Stdout()), &page); err != nil {
			t.Fatal(err)
		}
		assertManyRetainedRows(t, page, expected)
	}
}

func growRetainedWorkPayload(t *testing.T, path string, size int) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var events []factorydefinitions.FactoryEvent
	if err := json.Unmarshal(document["events"], &events); err != nil {
		t.Fatal(err)
	}
	found := false
	for index := range events {
		if events[index].Type != factorydefinitions.FactoryEventTypeWorkRequest {
			continue
		}
		var request work.WorkRequestEventPayload
		if err := json.Unmarshal(events[index].Payload, &request); err != nil || len(request.Works) != 1 {
			t.Fatalf("seed Work request: %v %+v", err, request)
		}
		request.Works[0].Payload = json.RawMessage(`"` + strings.Repeat("x", size) + `"`)
		events[index].Payload, err = json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		found = true
		break
	}
	if !found {
		t.Fatal("seed recording omitted Work request")
	}
	document["events"], err = json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return len(data)
}

func assertManyRetainedRows(t *testing.T, page map[string]any, expected map[string]retainedHistoryFacts) {
	t.Helper()
	rows := page["sessions"].([]any)
	if len(rows) != len(expected) {
		t.Fatalf("retained membership = %d, want %d", len(rows), len(expected))
	}
	seen := make(map[string]bool)
	for _, value := range rows {
		row := value.(map[string]any)
		id := row["workerSessionId"].(string)
		facts, exists := expected[id]
		if !exists || seen[id] || row["workName"] != facts.name || row["workId"] != "reused-work" || row["provider"] != "codex" || row["state"] != facts.state || row["recordingHealth"] != facts.health {
			t.Fatalf("retained row lost or duplicated scoped facts: %v", row)
		}
		seen[id] = true
	}
}

func assertManyRetainedContinuation(t *testing.T, host *support.FunctionalAPIServer, archived, all map[string]retainedHistoryFacts) {
	t.Helper()
	for _, view := range []string{"archived", "all"} {
		expected := archived
		if view == "all" {
			expected = all
		}
		endpoint := host.URL() + "/worker-sessions?history=" + view + "&limit=7"
		var rows []any
		for {
			page, err := readAttributionList(t.Context(), endpoint)
			if err != nil {
				t.Fatal(err)
			}
			rows = append(rows, page["sessions"].([]any)...)
			if len(rows) > len(expected) {
				t.Fatalf("continuation repeated or added membership: %d > %d", len(rows), len(expected))
			}
			token, _ := page["paginationContext"].(map[string]any)["nextToken"].(string)
			if token == "" {
				break
			}
			endpoint = host.URL() + "/worker-sessions?history=" + view + "&limit=7&nextToken=" + url.QueryEscape(token)
		}
		assertManyRetainedRows(t, map[string]any{"sessions": rows}, expected)
	}
}

type completingAttributionRunner struct {
	controlHostRunner
	release <-chan struct{}
}

func (runner completingAttributionRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte(`{"type":"thread.started","thread_id":"completed-attribution-provider"}`+"\n"))
	}
	result, err := runner.Run(ctx, request)
	if observer != nil && err == nil {
		observer(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func (runner completingAttributionRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	done := make(chan struct{})
	defer close(done)
	select {
	case runner.started <- done:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-runner.release:
	}
	output := strings.Join([]string{
		`{"type":"thread.started","thread_id":"completed-attribution-provider"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"completed-result","type":"agent_message","text":"COMPLETE"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":3}}`,
	}, "\n") + "\n"
	return platformprocess.CommandResult{Stdout: []byte(output)}, nil
}

func assertCompletedAttributionRows(t *testing.T, page map[string]any, live []any) {
	t.Helper()
	rows := page["sessions"].([]any)
	if len(rows) != len(live) {
		t.Fatalf("completed membership: %v", page)
	}
	expected := make(map[string]map[string]any, len(live))
	for _, value := range live {
		row := value.(map[string]any)
		expected[row["workerSessionId"].(string)] = row
	}
	for _, value := range rows {
		row := value.(map[string]any)
		id := row["workerSessionId"].(string)
		before, exists := expected[id]
		if !exists {
			t.Fatalf("duplicate or foreign completed attempt: %v", row)
		}
		delete(expected, id)
		for _, field := range []string{"workId", "workName", "factorySessionId", "provider"} {
			if row[field] != before[field] {
				t.Fatalf("completion changed %s: before=%v after=%v", field, before, row)
			}
		}
		if row["state"] != "COMPLETED" || row["recordingHealth"] != "COMPLETE" || row["terminalCause"] != "COMPLETED" {
			t.Fatalf("completion lost terminal capture facts: %v", row)
		}
		if row["provider"] != "codex" || row["confirmationState"] != "CONFIRMED" {
			t.Fatalf("completion lost provider attribution: %v", row)
		}
		usage, ok := row["tokenUsage"].(map[string]any)
		if !ok || usage["inputTokens"] != float64(4) || usage["outputTokens"] != float64(3) {
			t.Fatalf("completion lost captured provider usage: %v", row)
		}
	}
}

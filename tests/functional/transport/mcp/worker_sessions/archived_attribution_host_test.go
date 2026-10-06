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
	"testing"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type attributionMarkerRunner struct{ controlHostRunner }

func (r attributionMarkerRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte(`{"type":"thread.started","thread_id":"attribution-provider"}`+"\n"))
		observer(platformprocess.OutputStreamStdout, []byte(`{"type":"item.started","item":{"id":"archive-marker","type":"command_execution","command":"echo archived attribution marker","status":"in_progress"}}`+"\n"))
	}
	return r.Run(ctx, request)
}

func TestArchivedWorkAttributionEmptyAndLegacy(t *testing.T) {
	t.Parallel()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}})
	host, _, _, _ := startRecordedAttributionHost(t)
	session, ctx := startMCP(t, process, host.URL())
	page := historyParityPage(t, ctx, session, host, "archived", "all", "")
	if len(page["sessions"].([]any)) != 0 {
		t.Fatalf("empty archive: %v", page)
	}
	assertHistoryReadFailure(t, host, "unknown-worker", http.StatusNotFound, "NOT_FOUND")
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "unknown-worker"}), "worker_session.not_found", false)
	runCapturedMetadataRecovery(t, process)
}

func TestArchivedWorkAttributionDamagedHistory(t *testing.T) {
	t.Parallel()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}})
	for _, damage := range []string{"missing", "corrupt"} {
		t.Run(damage, func(t *testing.T) {
			t.Parallel()
			host, sessions, runner, dir := startRecordedAttributionHost(t)
			session, ctx := startMCP(t, process, host.URL())
			scopeID := admitRecordedAttributionWork(t, ctx, sessions, host, runner, dir, "damaged-history")
			rows := historyParityPage(t, ctx, session, host, "active", "factory", "")["sessions"].([]any)
			if len(rows) != 1 {
				t.Fatalf("damage fixture membership: %v", rows)
			}
			id := rows[0].(map[string]any)["workerSessionId"].(string)
			postHostJSON(t, ctx, host.URL()+"/factory-sessions/"+scopeID+"/pause", map[string]any{}, http.StatusOK)
			callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
			support.CloseFactorySessionAt(t, host.URL(), scopeID)
			endpoint := host.URL() + "/worker-sessions/" + id
			prefix := getHost(t, endpoint+"/logs")
			artifact := filepath.Join(dir, "damaged-history.json")
			if damage == "missing" {
				if err := os.Remove(artifact); err != nil {
					t.Fatal(err)
				}
				page := historyParityPage(t, ctx, session, host, "archived", "factory", "")
				closed := assertArchivedAttributionRow(t, page, id, rows[0].(map[string]any), false)
				selected := getHost(t, endpoint)
				assertJSONEqual(t, closed, selected)
				assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
				assertFactoryCLIParity(t, host, id, selected)
			} else {
				// The scope's writer is joined before corrupting its own board.
				if err := os.WriteFile(artifact, []byte("not a recording"), 0o600); err != nil {
					t.Fatal(err)
				}
				assertHistoryReadFailure(t, host, id, http.StatusInternalServerError, "INTERNAL_ERROR")
				assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": id}), "worker_session.internal_error", false)
				assertArchivedAttributionListFailure(t, ctx, host)
				assertToolError(t, callAction(t, ctx, session, "LIST", map[string]any{"history": "archived", "scope": "factory"}), "worker_session.internal_error", false)
			}
			assertJSONEqual(t, prefix, getHost(t, endpoint+"/logs"))
		})
	}
}

func assertArchivedAttributionListFailure(t *testing.T, ctx context.Context, host *support.FunctionalAPIServer) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL()+"/worker-sessions?history=archived&scope=factory", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var diagnostic factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&diagnostic); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusInternalServerError || diagnostic.Code != "INTERNAL_ERROR" {
		t.Fatalf("corrupt history list: %d %+v", response.StatusCode, diagnostic)
	}
	inputs := support.FakeInputs(ctx, []string{"you", "--server", host.URL(), "worker-sessions", "list", "--history", "archived", "--json"})
	if err := host.Execute(t, inputs.Input); err == nil || inputs.Stdout() != "" || !strings.Contains(inputs.Stderr(), "INTERNAL_ERROR") {
		t.Fatalf("corrupt history CLI list: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
}

// HTTP Open has no recording selector. The public session Start contract admits
// two distinct recorded scopes on one root-built host; all observations remain
// CLI/HTTP/MCP customer reads. Their reused Work ID is deliberately identical.
func TestArchivedWorkAttributionReusedWorkID(t *testing.T) {
	t.Parallel()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}})
	host, sessions, runner, dir := startRecordedAttributionHost(t)
	session, ctx := startMCP(t, process, host.URL())
	expected := make(map[string]string)
	for _, name := range []string{"scope-alpha", "scope-beta"} {
		scopeID := admitRecordedAttributionWork(t, ctx, sessions, host, runner, dir, name)
		expected[scopeID] = name
	}
	live := historyParityPage(t, ctx, session, host, "active", "factory", "")["sessions"].([]any)
	if len(live) != 2 {
		t.Fatalf("coexisting reused Work membership: %v", live)
	}
	for _, value := range live {
		row := value.(map[string]any)
		id, scopeID := row["workerSessionId"].(string), row["factorySessionId"].(string)
		if row["workId"] != "reused-work" || row["workName"] != expected[scopeID] {
			t.Fatalf("live scoped Work association: %v", row)
		}
		postHostJSON(t, ctx, host.URL()+"/factory-sessions/"+scopeID+"/pause", map[string]any{}, http.StatusOK)
		callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
		support.CloseFactorySessionAt(t, host.URL(), scopeID)
	}
	// A sibling live Work with the same ID cannot enrich either closed scope.
	admitRecordedAttributionWork(t, ctx, sessions, host, runner, dir, "live-shadow")
	archived := historyParityPage(t, ctx, session, host, "archived", "factory", "")["sessions"].([]any)
	if len(archived) != 2 {
		t.Fatalf("archived reused Work membership: %v", archived)
	}
	for index, value := range archived {
		row := value.(map[string]any)
		id, scopeID := row["workerSessionId"].(string), row["factorySessionId"].(string)
		assertReusedWorkAttribution(t, row, expected[scopeID], live[index].(map[string]any)["workerSessionId"])
		selected := getHost(t, host.URL()+"/worker-sessions/"+url.PathEscape(id))
		assertJSONEqual(t, row, selected)
		assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
		assertFactoryCLIParity(t, host, id, selected)
	}
	first := callWorker(t, ctx, session, "list", map[string]any{"history": "archived", "scope": "factory", "limit": 1})["result"].(map[string]any)
	if rows := first["sessions"].([]any); len(rows) != 1 {
		t.Fatalf("first archived page: %v", first)
	} else {
		assertJSONEqual(t, rows[0], archived[0])
	}
	token := first["paginationContext"].(map[string]any)["nextToken"].(string)
	last := historyParityPage(t, ctx, session, host, "archived", "factory", token)["sessions"].([]any)
	if len(last) != 1 {
		t.Fatalf("last archived page: %v", last)
	}
	assertJSONEqual(t, last[0], archived[1])
}

func startRecordedAttributionHost(t *testing.T) (*support.FunctionalAPIServer, support.FactorySessionStarter, controlHostRunner, string) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "scoped-attribution")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 2)}
	var sessions support.FactorySessionStarter
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
		BeforeStart: func(_ testing.TB, p support.Process, _ root.Input) {
			sessions = p.(support.ApplicationProcess).FactorySessions()
		},
	})
	return host, sessions, runner, dir
}

// Two durable profiles are different immutable storage shapes. Their public
// fleet reads own isolated observation windows while sharing one MCP process.
func TestArchivedWorkAttributionProfileIsolation(t *testing.T) {
	t.Parallel()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}})
	var hosts []*support.FunctionalAPIServer
	var ids []string
	for _, name := range []string{"profile-alpha", "profile-beta"} {
		host, sessions, runner, dir := startRecordedAttributionHost(t)
		session, ctx := startMCP(t, process, host.URL())
		scopeID := admitRecordedAttributionWork(t, ctx, sessions, host, runner, dir, name)
		live := historyParityPage(t, ctx, session, host, "active", "factory", "")["sessions"].([]any)
		if len(live) != 1 {
			t.Fatalf("profile active membership: %v", live)
		}
		id := live[0].(map[string]any)["workerSessionId"].(string)
		postHostJSON(t, ctx, host.URL()+"/factory-sessions/"+scopeID+"/pause", map[string]any{}, http.StatusOK)
		callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
		support.CloseFactorySessionAt(t, host.URL(), scopeID)
		rows := historyParityPage(t, ctx, session, host, "archived", "factory", "")["sessions"].([]any)
		if len(rows) != 1 {
			t.Fatalf("profile archived membership: %v", rows)
		}
		row := rows[0].(map[string]any)
		assertReusedWorkAttribution(t, row, name, id)
		selected := getHost(t, host.URL()+"/worker-sessions/"+id)
		assertJSONEqual(t, row, selected)
		assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
		assertFactoryCLIParity(t, host, id, selected)
		hosts, ids = append(hosts, host), append(ids, id)
	}
	for index, host := range hosts {
		foreignID := ids[1-index]
		session, ctx := startMCP(t, process, host.URL())
		assertHistoryReadFailure(t, host, foreignID, http.StatusNotFound, "NOT_FOUND")
		assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": foreignID}), "worker_session.not_found", false)
	}
}

func admitRecordedAttributionWork(t *testing.T, ctx context.Context, sessions support.FactorySessionStarter, host *support.FunctionalAPIServer, runner controlHostRunner, dir, name string) string {
	t.Helper()
	scopeID := uuid.NewString()
	result, err := sessions.Start(ctx, factorysessions.SessionStartRequest{
		SessionID: scopeID, Mode: factorysessions.SessionOperationModeLive, FolderPath: dir, ActivationOnly: true,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			Mode: factorysessions.SessionRuntimeModeService, SystemConfigHome: t.TempDir(),
			DefinitionSourcePath: filepath.Join(dir, "factory.json"), ExecutionBaseDir: dir, RuntimeInstanceID: uuid.NewString(),
			Recording: factorysessions.SessionRecordingSelection{RecordPath: filepath.Join(dir, name+".json")},
		},
	})
	if err != nil || result.SessionID != scopeID {
		t.Fatalf("recorded scope admission: %+v %v", result, err)
	}
	inputs := support.FakeInputs(ctx, []string{"you", "--server", host.URL(), "--session", scopeID, "submit", "batch",
		fmt.Sprintf(`{"requestId":"reused-request","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"reused-work","name":%q,"workTypeName":"task","payload":"hold"}]}`, name)})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("reused Work admission: %v %s", err, inputs.Stderr())
	}
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatal("recorded scope did not reach provider edge")
	}
	return scopeID
}

func assertReusedWorkAttribution(t *testing.T, row map[string]any, name string, workerID any) {
	t.Helper()
	if row["workName"] != name || row["workId"] != "reused-work" || row["terminalCause"] != "OPERATOR_CANCEL" || row["recordingHealth"] != "COMPLETE" || row["workerSessionId"] != workerID {
		t.Fatalf("archive changed scoped name/identity/order/health: %v", row)
	}
}

// The operator-authorized F-N1 split protects typed unavailable attribution
// after a real unrecorded named Work admission and joined close. The fleet endpoint
// is global, so the scenario owns its host/profile observation window. The
// reusable MCP process can serve peers; only this scenario's lifecycle is
// sequential. Reuse the existing parity helpers rather than a second harness.
func TestArchivedWorkAttributionNamedClose(t *testing.T) {
	t.Parallel()
	for _, recorded := range []bool{false, true} {
		name := "unrecorded"
		if recorded {
			name = "recorded"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runArchivedWorkAttributionNamedClose(t, recorded)
		})
	}
}

func runArchivedWorkAttributionNamedClose(t *testing.T, recorded bool) {
	t.Helper()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}})
	dir := support.ScaffoldSingleStepFactory(t, "archived-attribution")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 1)}
	cfg := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Args:  []string{"--record", filepath.Join(dir, "board.json")},
		Edges: serviceedges.Edges{ProviderCommandRunner: attributionMarkerRunner{runner}, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	}
	host := support.StartFunctionalAPIServer(t, cfg)
	session, ctx := startMCP(t, process, host.URL())
	scopeID := ""
	if recorded {
		scopeID = support.GetDefaultSession(t, host.URL()).Id
	} else {
		scopeID = support.OpenFactorySessionAt(t, host.URL(), dir).Session.Id
	}
	name := "archive-alpha"
	support.SubmitSessionWorkAt(t, host.URL(), scopeID, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Name: &name, Payload: "hold named Work until cancellation",
	})
	var done <-chan struct{}
	select {
	case done = <-runner.started:
	case <-ctx.Done():
		t.Fatal("named Work did not reach provider edge")
	}
	active := historyParityPage(t, ctx, session, host, "active", "factory", "")
	rows := active["sessions"].([]any)
	if len(rows) != 1 {
		t.Fatalf("named active membership: %v", active)
	}
	live := rows[0].(map[string]any)
	id, ok := live["workerSessionId"].(string)
	if !ok || live["workName"] != name || live["factorySessionId"] != scopeID || live["workId"] == nil {
		t.Fatalf("named live association: %v", live)
	}
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id)
	assertAdmittedAttributionName(t, getHost(t, host.URL()+"/factory-sessions/"+scopeID+"/work/"+url.PathEscape(live["workId"].(string))), name)
	assertFactoryCLIParity(t, host, id, getHost(t, endpoint))
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
	waitControlSignal(t, done)
	logs := getHost(t, endpoint+"/logs")
	assertAttributionMarker(t, logs)
	if recorded {
		// The recorded host owns its default scope. Orderly host closure joins
		// that scope; a fresh host reads the same profile without a live Work.
		host.Close(t)
		cfg.Args = nil
		host = support.StartFunctionalAPIServer(t, cfg)
		session, ctx = startMCP(t, process, host.URL())
		endpoint = host.URL() + "/worker-sessions/" + url.PathEscape(id)
	} else {
		support.CloseFactorySessionAt(t, host.URL(), scopeID)
	}
	archived := historyParityPage(t, ctx, session, host, "archived", "factory", "")
	closed := assertArchivedAttributionRow(t, archived, id, live, recorded)
	selected := getHost(t, endpoint)
	assertJSONEqual(t, closed, selected)
	assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
	assertFactoryCLIParity(t, host, id, selected)
	assertJSONEqual(t, logs, getHost(t, endpoint+"/logs"))
	assertArchivedAttributionTable(t, ctx, host, name, recorded)
}

func assertAdmittedAttributionName(t *testing.T, work any, name string) {
	t.Helper()
	if work.(map[string]any)["name"] != name {
		t.Fatalf("admitted Work name: %v", work)
	}
}

func assertAttributionMarker(t *testing.T, logs any) {
	t.Helper()
	data, err := json.Marshal(logs)
	if err != nil || !strings.Contains(string(data), "archived attribution marker") {
		t.Fatalf("captured marker missing: %v %v", logs, err)
	}
}

func assertArchivedAttributionTable(t *testing.T, ctx context.Context, host *support.FunctionalAPIServer, name string, recorded bool) {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{"you", "--server", host.URL(), "worker-sessions", "list", "--history", "archived"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("closed unavailable Work name: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	for _, line := range strings.Split(strings.TrimSpace(inputs.Stdout()), "\n")[1:] {
		fields := strings.Fields(line)
		wantName, wantProvider := "-", "-"
		if recorded {
			wantName, wantProvider = name, "codex"
		}
		if len(fields) < 4 || fields[0] != wantName || fields[3] != wantProvider {
			t.Fatalf("human unavailable Work name/provider: %s", line)
		}
	}
}

func assertArchivedAttributionRow(t *testing.T, archived map[string]any, id string, live map[string]any, recorded bool) map[string]any {
	t.Helper()
	rows := archived["sessions"].([]any)
	var closed map[string]any
	for _, row := range rows {
		candidate := row.(map[string]any)
		if candidate["workerSessionId"] == id {
			closed = candidate
		}
	}
	if closed == nil {
		t.Fatalf("closed archived membership: %v", archived)
	}
	for _, key := range []string{"workerSessionId", "workId", "factorySessionId"} {
		if closed[key] != live[key] {
			t.Fatalf("close changed %s: live=%v closed=%v", key, live, closed)
		}
	}
	if recorded && (closed["workName"] != live["workName"] || closed["provider"] != live["provider"]) {
		t.Fatalf("recorded scope lost authoritative attribution: live=%v closed=%v", live, closed)
	}
	if !recorded && (closed["workName"] != nil || closed["provider"] != nil) {
		t.Fatalf("unrecorded scope supplied unavailable attribution: %v", closed)
	}
	if closed["terminalCause"] != "OPERATOR_CANCEL" || closed["recordingHealth"] != "COMPLETE" {
		t.Fatalf("closed capture cause/health: %v", closed)
	}
	return closed
}

package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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

func TestArchivedWorkAttributionJourneys(t *testing.T) {
	t.Parallel()
	process := newSelectedHostClientProcess(t)
	for _, scenario := range []struct {
		name string
		run  func(*testing.T, support.Process)
	}{
		{"empty and legacy", runArchivedWorkAttributionEmptyAndLegacy},
		{"damaged history", runArchivedWorkAttributionDamagedHistory},
		{"reused Work identity", runArchivedWorkAttributionReusedWorkID},
		{"profile isolation", runArchivedWorkAttributionProfileIsolation},
		{"named close", runArchivedWorkAttributionNamedCloseJourneys},
		{"canceled history read", runArchivedWorkAttributionCanceledRead},
		{"provider completion preserves frozen history", runArchivedWorkAttributionCompleted},
	} {
		t.Run(scenario.name, func(t *testing.T) { scenario.run(t, process) })
	}
}

func runArchivedWorkAttributionEmptyAndLegacy(t *testing.T, process support.Process) {
	t.Parallel()
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

// F-07 owns a real recorded scope and gates only its artifact-read effect.
// The host and real decoder/store are shared by canceled, surviving and fresh
// requests; no source scan or private cache state is part of the observer.
func runArchivedWorkAttributionCanceledRead(t *testing.T, process support.Process) {
	t.Parallel()
	gate := &attributionReadGate{}
	host, sessions, runner, dir := startRecordedAttributionReadHost(t, gate.read)
	session, ctx := startMCP(t, process, host.URL())
	scopeID := admitRecordedAttributionWork(t, ctx, sessions, host, runner, dir, "canceled-history")
	live := historyParityPage(t, ctx, session, host, "active", "factory", "")["sessions"].([]any)
	if len(live) != 1 {
		t.Fatalf("cancellation fixture membership: %v", live)
	}
	row := live[0].(map[string]any)
	id := row["workerSessionId"].(string)
	postHostJSON(t, ctx, host.URL()+"/factory-sessions/"+scopeID+"/pause", map[string]any{}, http.StatusOK)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
	support.CloseFactorySessionAt(t, host.URL(), scopeID)
	// Both cancellation attempts own the same retained scope and read gate.
	// Serialize this observation window while reusing one root-built host.
	for _, view := range []string{"archived", "all"} {
		t.Run(view, func(t *testing.T) {
			entered, release := gate.arm(filepath.Join(dir, "canceled-history.json"))
			t.Cleanup(release)
			assertCanceledAttributionList(t, ctx, session, host, view, id, row, entered, release)
		})
	}
}

func assertCanceledAttributionList(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, view, id string, row map[string]any, entered <-chan struct{}, release func()) {
	t.Helper()
	bounded, done := context.WithTimeout(ctx, 30*time.Second)
	defer done()
	var requests sync.WaitGroup
	t.Cleanup(func() { release(); requests.Wait() })
	canceled, cancel := context.WithCancel(bounded)
	defer cancel()
	endpoint := host.URL() + "/worker-sessions?history=" + view + "&scope=factory"
	first := beginAttributionList(canceled, endpoint, &requests)
	select {
	case <-entered:
	case <-bounded.Done():
		t.Fatalf("history read did not reach scenario gate: %v", bounded.Err())
	}
	cancel()
	select {
	case result := <-first:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("canceled list error = %v, want context.Canceled", result.err)
		}
	case <-bounded.Done():
		t.Fatal("canceled caller did not leave blocked history read")
	}
	// Independent active reads must remain usable while history IO is
	// blocked. A surviving archived/all request owns a separate context.
	active := historyParityPage(t, bounded, session, host, "active", "factory", "")
	if len(active["sessions"].([]any)) != 0 {
		t.Fatalf("closed owner returned as active: %v", active)
	}
	survivor := beginAttributionList(bounded, endpoint, &requests)
	release()
	select {
	case result := <-survivor:
		if result.err != nil {
			t.Fatalf("independent history list: %v", result.err)
		}
		if len(result.page["sessions"].([]any)) != 1 {
			t.Fatalf("independent list lost or duplicated retained row: %v", result.page)
		}
		assertArchivedAttributionRow(t, result.page, id, row, true)
	case <-bounded.Done():
		t.Fatal("surviving history request did not complete")
	}
	assertRetainedAttributionViews(t, bounded, session, host, id, row, true)
}

type attributionReadGate struct {
	mu      sync.Mutex
	path    string
	entered chan struct{}
	release chan struct{}
}

func (g *attributionReadGate) arm(path string) (<-chan struct{}, func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.path = path
	g.entered, g.release = make(chan struct{}), make(chan struct{})
	release := g.release
	return g.entered, sync.OnceFunc(func() { close(release) })
}

func (g *attributionReadGate) read(path string) ([]byte, error) {
	g.mu.Lock()
	blocked := g.path != "" && filepath.Clean(path) == filepath.Clean(g.path)
	release := g.release
	if blocked {
		g.path = ""
		close(g.entered)
	}
	g.mu.Unlock()
	if blocked {
		<-release
	}
	return os.ReadFile(path)
}

type attributionListResult struct {
	page map[string]any
	err  error
}

func beginAttributionList(ctx context.Context, endpoint string, requests *sync.WaitGroup) <-chan attributionListResult {
	result := make(chan attributionListResult, 1)
	requests.Add(1)
	go func() {
		defer requests.Done()
		page, err := readAttributionList(ctx, endpoint)
		result <- attributionListResult{page, err}
	}()
	return result
}

func readAttributionList(ctx context.Context, endpoint string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("history list status %d", response.StatusCode)
	}
	var page map[string]any
	err = json.NewDecoder(response.Body).Decode(&page)
	return page, err
}

func runArchivedWorkAttributionDamagedHistory(t *testing.T, process support.Process) {
	t.Parallel()
	for _, damage := range []string{"missing", "corrupt", "conflicting association"} {
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
			original, err := os.ReadFile(artifact)
			if err != nil {
				t.Fatal(err)
			}
			// Prime both views before removing/replacing the joined writer's
			// artifact. A cached name must not hide the unavailable source.
			retained := assertRetainedAttributionViews(t, ctx, session, host, id, rows[0].(map[string]any), true)
			if damage == "missing" {
				if err := os.Remove(artifact); err != nil {
					t.Fatal(err)
				}
				closed := assertRetainedAttributionViews(t, ctx, session, host, id, rows[0].(map[string]any), false)
				if closed["workName"] != nil {
					t.Fatalf("missing history fabricated a Work name: %v", closed)
				}
				selected := getHost(t, endpoint)
				assertJSONEqual(t, closed, selected)
				assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
				assertFactoryCLIParity(t, host, id, selected)
			} else {
				// The scope's writer is joined before corrupting its own board.
				replacement := []byte("not a recording")
				if damage == "conflicting association" {
					// The identical repeated association is accepted by the same
					// decoder, proving the appended frame itself is valid.
					if err := os.WriteFile(artifact, appendedAttributionAssociation(t, original, false), 0o600); err != nil {
						t.Fatal(err)
					}
					assertRetainedAttributionViews(t, ctx, session, host, id, rows[0].(map[string]any), true)
					// F-06: a valid event frame contradicts the accepted dispatch's
					// Worker association after both retained views have been cached.
					// Observe safe customer errors, never an arbitrary cached name.
					replacement = appendedAttributionAssociation(t, original, true)
				}
				if err := os.WriteFile(artifact, replacement, 0o600); err != nil {
					t.Fatal(err)
				}
				assertHistoryReadFailure(t, host, id, http.StatusInternalServerError, "INTERNAL_ERROR")
				assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": id}), "worker_session.internal_error", false)
				for _, view := range []string{"archived", "all"} {
					assertAttributionListFailure(t, ctx, host, view)
					assertToolError(t, callAction(t, ctx, session, "LIST", map[string]any{"history": view, "scope": "factory"}), "worker_session.internal_error", false)
				}
			}
			// Restoration is observed on this same process, without restarting
			// or changing capture identity, through fresh CLI/HTTP/MCP lists.
			if err := os.WriteFile(artifact, original, 0o600); err != nil {
				t.Fatal(err)
			}
			restored := assertRetainedAttributionViews(t, ctx, session, host, id, rows[0].(map[string]any), true)
			assertJSONEqual(t, retained, restored)
			assertJSONEqual(t, prefix, getHost(t, endpoint+"/logs"))
		})
	}
}

func appendedAttributionAssociation(t *testing.T, original []byte, conflicting bool) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(original, &document); err != nil {
		t.Fatal(err)
	}
	var events []factorydefinitions.FactoryEvent
	if err := json.Unmarshal(document["events"], &events); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type != factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc {
			continue
		}
		last := events[len(events)-1]
		event.Id = uuid.NewString()
		event.Context.Sequence = len(events)
		event.Context.Tick = last.Context.Tick + 1
		event.Context.EventTime = last.Context.EventTime.Add(time.Millisecond)
		if conflicting {
			event.Payload = json.RawMessage(`{"workerSessionId":"conflicting-worker"}`)
		}
		events = append(events, event)
		var err error
		document["events"], err = json.Marshal(events)
		if err != nil {
			t.Fatal(err)
		}
		replacement, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		return replacement
	}
	t.Fatal("recorded fixture has no dispatch association to contradict")
	return nil
}

func assertRetainedAttributionViews(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, id string, live map[string]any, named bool) map[string]any {
	t.Helper()
	var archived map[string]any
	for _, view := range []string{"archived", "all"} {
		page := historyParityPage(t, ctx, session, host, view, "factory", "")
		if len(page["sessions"].([]any)) != 1 {
			t.Fatalf("%s lost or duplicated the retained row: %v", view, page)
		}
		// Preserve the current unavailable optional-name/provider
		// representation and the authoritative capture identity/lifecycle.
		row := assertArchivedAttributionRow(t, page, id, live, named)
		if archived != nil {
			assertJSONEqual(t, archived, row)
		}
		archived = row
	}
	return archived
}

func assertAttributionListFailure(t *testing.T, ctx context.Context, host *support.FunctionalAPIServer, view string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL()+"/worker-sessions?history="+view+"&scope=factory", nil)
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
	inputs := support.FakeInputs(ctx, []string{"you", "--server", host.URL(), "worker-sessions", "list", "--history", view, "--json"})
	if err := host.Execute(t, inputs.Input); err == nil || inputs.Stdout() != "" || !strings.Contains(inputs.Stderr(), "INTERNAL_ERROR") {
		t.Fatalf("corrupt history CLI list: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
}

// HTTP Open has no recording selector. The public session Start contract admits
// two distinct recorded scopes on one root-built host; all observations remain
// CLI/HTTP/MCP customer reads. Their reused Work ID is deliberately identical.
func runArchivedWorkAttributionReusedWorkID(t *testing.T, process support.Process) {
	t.Parallel()
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
	assertFrozenAttributionAfterSiblingClose(t, ctx, session, host)
}

func assertFrozenAttributionAfterSiblingClose(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer) {
	t.Helper()
	expected := make(map[string][]any)
	tokens := make(map[string]string)
	continuations := make(map[string]map[string]any)
	for _, view := range []string{"archived", "all"} {
		expected[view] = historyParityPage(t, ctx, session, host, view, "factory", "")["sessions"].([]any)
		first := callWorker(t, ctx, session, "list", map[string]any{"history": view, "scope": "factory", "limit": 1})["result"].(map[string]any)
		if rows := first["sessions"].([]any); len(rows) != 1 {
			t.Fatalf("first %s page: %v", view, first)
		} else {
			// Independent snapshots can observe a different live duration.
			// Replaying the same token below still compares every field exactly.
			assertRuntimeObservationParity(t, rows[0], expected[view][0])
		}
		tokens[view] = first["paginationContext"].(map[string]any)["nextToken"].(string)
		continuations[view] = historyParityPage(t, ctx, session, host, view, "factory", tokens[view])
		if len(continuations[view]["sessions"].([]any)) != len(expected[view])-1 {
			t.Fatalf("%s continuation lost initial membership: %v", view, continuations[view])
		}
	}
	// The existing live sibling becomes archived after snapshot creation.
	// Both views must retain their original membership and row content,
	// including the all-view snapshot's former live sibling.
	shadow := historyParityPage(t, ctx, session, host, "active", "factory", "")["sessions"].([]any)
	if len(shadow) != 1 {
		t.Fatalf("live shadow membership: %v", shadow)
	}
	shadowRow := shadow[0].(map[string]any)
	shadowID, shadowScope := shadowRow["workerSessionId"].(string), shadowRow["factorySessionId"].(string)
	postHostJSON(t, ctx, host.URL()+"/factory-sessions/"+shadowScope+"/pause", map[string]any{}, http.StatusOK)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": shadowID, "operation": "CANCEL"})
	support.CloseFactorySessionAt(t, host.URL(), shadowScope)
	fresh := historyParityPage(t, ctx, session, host, "archived", "factory", "")["sessions"].([]any)
	if len(fresh) != 3 {
		t.Fatalf("fresh archive did not observe committed sibling: %v", fresh)
	}
	for _, view := range []string{"archived", "all"} {
		page := historyParityPage(t, ctx, session, host, view, "factory", tokens[view])
		assertJSONEqual(t, continuations[view], page)
	}
}

func startRecordedAttributionHost(t *testing.T) (*support.FunctionalAPIServer, support.FactorySessionStarter, controlHostRunner, string) {
	t.Helper()
	return startRecordedAttributionReadHost(t, nil)
}

func startRecordedAttributionReadHost(t *testing.T, readFile recordings.RecordingReadFile) (*support.FunctionalAPIServer, support.FactorySessionStarter, controlHostRunner, string) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "scoped-attribution")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 2)}
	var sessions support.FactorySessionStarter
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir), RecordingReadFile: readFile},
		BeforeStart: func(_ testing.TB, p support.Process, _ root.Input) {
			sessions = p.(support.ApplicationProcess).FactorySessions()
		},
	})
	return host, sessions, runner, dir
}

// Two durable profiles are different immutable storage shapes. Their public
// fleet reads own isolated observation windows while sharing one MCP process.
func runArchivedWorkAttributionProfileIsolation(t *testing.T, process support.Process) {
	t.Parallel()
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
func runArchivedWorkAttributionNamedCloseJourneys(t *testing.T, process support.Process) {
	t.Parallel()
	for _, scenario := range []struct {
		name                string
		recorded, ownerLost bool
	}{
		{name: "unrecorded"},
		{name: "recorded", recorded: true},
		{name: "owner lost retains named Work", recorded: true, ownerLost: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			runArchivedWorkAttributionNamedClose(t, process, scenario.recorded, scenario.ownerLost)
		})
	}
}

func runArchivedWorkAttributionNamedClose(t *testing.T, process support.Process, recorded, ownerLost bool) {
	t.Helper()
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
	done := admitNamedAttributionWork(t, ctx, host, runner, scopeID, name)
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
		if ownerLost {
			retainAttributionCapturePrefix(t, dir, id)
		}
		cfg.Args = nil
		host = support.StartFunctionalAPIServer(t, cfg)
		session, ctx = startMCP(t, process, host.URL())
		endpoint = host.URL() + "/worker-sessions/" + url.PathEscape(id)
	} else {
		support.CloseFactorySessionAt(t, host.URL(), scopeID)
	}
	if ownerLost {
		assertNamedOwnerLostHistory(t, ctx, session, host, id, live)
		return
	}
	archived := historyParityPage(t, ctx, session, host, "archived", "factory", "")
	closed := assertArchivedAttributionRow(t, archived, id, live, recorded)
	all := historyParityPage(t, ctx, session, host, "all", "factory", "")
	allRow := assertArchivedAttributionRow(t, all, id, live, recorded)
	assertJSONEqual(t, closed, allRow)
	selected := getHost(t, endpoint)
	assertJSONEqual(t, closed, selected)
	assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
	assertFactoryCLIParity(t, host, id, selected)
	assertJSONEqual(t, logs, getHost(t, endpoint+"/logs"))
	assertArchivedAttributionTable(t, ctx, host, name, recorded)
}

func admitNamedAttributionWork(t *testing.T, ctx context.Context, host *support.FunctionalAPIServer, runner controlHostRunner, scopeID, name string) <-chan struct{} {
	t.Helper()
	support.SubmitSessionWorkAt(t, host.URL(), scopeID, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Name: &name, Payload: "hold named Work until cancellation",
	})
	var done <-chan struct{}
	select {
	case done = <-runner.started:
	case <-ctx.Done():
		t.Fatal("named Work did not reach provider edge")
	}
	return done
}

// F-03 seeds the last committed pre-terminal capture at the file edge only
// after the original host has joined. The Factory artifact remains intact;
// recovery must use its exact association rather than a live Work or sibling.
func retainAttributionCapturePrefix(t *testing.T, dir, workerID string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, ".you-agent-factory", "worker-recordings", "*.worker.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var prefix []byte
		for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'}) {
			var entry struct {
				WorkerSessionID string `json:"workerSessionId"`
				Kind            string `json:"kind"`
				Record          *struct {
					Payload struct {
						Phase string `json:"phase"`
					} `json:"payload"`
				} `json:"record"`
			}
			if err := json.Unmarshal(line, &entry); err != nil {
				t.Fatal(err)
			}
			if entry.WorkerSessionID != workerID {
				break
			}
			if entry.Kind != "record" || entry.Record == nil || entry.Record.Payload.Phase == "CANCELED" || entry.Record.Payload.Phase == "COMPLETED" || entry.Record.Payload.Phase == "FAILED" {
				if len(prefix) == 0 {
					t.Fatal("capture has no committed opening prefix")
				}
				if err := os.WriteFile(path, prefix, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			prefix = append(append(prefix, line...), '\n')
		}
	}
	t.Fatal("named capture has no removable terminal boundary")
}

func assertNamedOwnerLostHistory(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, id string, live map[string]any) {
	t.Helper()
	active := historyParityPage(t, ctx, session, host, "active", "factory", "")
	if len(active["sessions"].([]any)) != 0 {
		t.Fatalf("recovery invented an active owner: %v", active)
	}
	selected := getHost(t, host.URL()+"/worker-sessions/"+url.PathEscape(id)).(map[string]any)
	if selected["state"] != "FAILED" || selected["confirmationState"] != "UNCONFIRMED" || selected["recordingHealth"] != "INCOMPLETE" || selected["failure"].(map[string]any)["kind"] != "PROCESS_GONE" {
		t.Fatalf("owner loss invented completion or confirmation: %v", selected)
	}
	if selected["workName"] != "archive-alpha" || selected["provider"] != "codex" || selected["endedAt"] != nil {
		t.Fatalf("owner loss lost authored facts or invented a terminal timestamp: %v", selected)
	}
	for _, field := range []string{"workerSessionId", "factorySessionId", "workId", "workName", "provider"} {
		if selected[field] != live[field] {
			t.Fatalf("owner loss changed retained %s: got=%v live=%v", field, selected, live)
		}
	}
	for _, view := range []string{"archived", "all"} {
		page := historyParityPage(t, ctx, session, host, view, "factory", "")
		rows := page["sessions"].([]any)
		if len(rows) != 1 {
			t.Fatalf("%s lost or duplicated recovered membership: %v", view, page)
		}
		assertJSONEqual(t, selected, rows[0])
	}
	assertFactoryCLIParity(t, host, id, selected)
	assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
	assertAttributionMarker(t, getHost(t, host.URL()+"/worker-sessions/"+url.PathEscape(id)+"/logs"))
	// Both hosts run in this test process. A changed supervisor epoch does not
	// prove OS death or acquire authority to stop a historical child.
	for _, operation := range []string{"cancel", "terminate"} {
		assertUnwitnessedHistoryControlRefused(t, host, id, operation)
	}
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

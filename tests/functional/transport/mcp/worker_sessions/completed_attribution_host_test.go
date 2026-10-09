package workersessions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

// F-08 owns an isolated fleet observation window: 24 real recorded sessions
// share a host, then one reopened host serves every measured read. Fixture IO
// happens before measurement. The customer-requested latency criterion is the
// explicit functional exception in this lane's PRD, not a generic load test.
// Factory artifacts exceed the candidate byte dimensions (174187553 total,
// 21771842 maximum). That candidate counted capture journals/snapshots, so
// these dimensions alone do not prove matching format, event count or skew.
func runArchivedWorkAttributionManyHistories(t *testing.T, process support.Process) {
	t.Parallel()
	host, sessions, runner, dir := startRecordedAttributionHost(t)
	connection, ctx := startMCP(t, process, host.URL())
	expected := make(map[string]retainedHistoryFacts)
	var paths []string
	for index := range 24 {
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
	// Grow a known Work payload at the filesystem fixture boundary only after
	// all writers have joined. Preserve actual identity, ordering, association,
	// names and captured terminal/provider facts; no unknown padding fields.
	var total int
	for index, path := range paths {
		size := 7 << 20
		if index == 0 {
			size = 22 << 20
		}
		total += growRetainedWorkPayload(t, path, size)
	}
	t.Logf("retained fixture: recordings=%d bytes=%d; largest payload=%d", len(paths), total, 22<<20)
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
		for _, sample := range []string{"first", "fresh"} {
			started := time.Now()
			page, err := readAttributionList(t.Context(), host.URL()+"/worker-sessions?history="+view)
			elapsed := time.Since(started)
			t.Logf("%s %s consumed GET: %s", view, sample, elapsed)
			if err != nil {
				t.Fatalf("%s %s list: %v", view, sample, err)
			}
			//nolint:testsleep // F-08 explicitly requires each consumed GET below two seconds.
			if elapsed >= 2*time.Second {
				t.Fatalf("%s %s consumed GET took %s, require <2s", view, sample, elapsed)
			}
			assertManyRetainedRows(t, page, expected)
		}
		inputs := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "worker-sessions", "list", "--history", view, "--limit", "100", "--json"})
		started := time.Now()
		if err := host.Execute(t, inputs.Input); err != nil {
			t.Fatalf("%s CLI within unchanged timeout: %v %s", view, err, inputs.Stderr())
		}
		t.Logf("%s CLI: %s", view, time.Since(started))
		var page map[string]any
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

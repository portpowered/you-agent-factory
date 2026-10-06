package workersessions_test

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
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
	assertFactoryCLIParity(t, host, id, getHost(t, endpoint))
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
	waitControlSignal(t, done)
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

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
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}})
	dir := support.ScaffoldSingleStepFactory(t, "archived-attribution")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 1)}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Args:  []string{"--record", filepath.Join(dir, "board.json")},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	name := "archive-alpha"
	support.SubmitSessionWorkAt(t, host.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{
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
	if !ok || live["workName"] != name || live["factorySessionId"] != opened.Session.Id || live["workId"] == nil {
		t.Fatalf("named live association: %v", live)
	}
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id)
	assertFactoryCLIParity(t, host, id, getHost(t, endpoint))
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "CANCEL"})
	waitControlSignal(t, done)
	support.CloseFactorySessionAt(t, host.URL(), opened.Session.Id)
	archived := historyParityPage(t, ctx, session, host, "archived", "factory", "")
	closed := assertUnrecordedArchivedRow(t, archived, id, live)
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
		if len(fields) < 4 || fields[0] != "-" || fields[3] != "-" {
			t.Fatalf("human unavailable Work name/provider: %s", line)
		}
	}
}

func assertUnrecordedArchivedRow(t *testing.T, archived map[string]any, id string, live map[string]any) map[string]any {
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
	if closed["workName"] != nil || closed["provider"] != nil {
		t.Fatalf("unrecorded scope supplied unavailable attribution: %v", closed)
	}
	if closed["terminalCause"] != "OPERATOR_CANCEL" || closed["recordingHealth"] != "COMPLETE" {
		t.Fatalf("closed capture cause/health: %v", closed)
	}
	return closed
}

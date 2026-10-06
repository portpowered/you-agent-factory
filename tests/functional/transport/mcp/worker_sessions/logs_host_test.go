package workersessions_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// CLI/HTTP/MCP parity owns these public reads. The root-built host captures an
// admitted controlled provider; no provider files or executable are consulted.
func runCapturedLogsPolling(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "logs-polling")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 2)}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	stopped := admitControlWorker(t, ctx, host.URL(), "polled", runner)
	siblingStopped := admitControlWorker(t, ctx, host.URL(), "sibling", runner)
	page := logsParityPage(t, ctx, session, host, "polled", "")
	token, ok := page["nextToken"].(string)
	if !ok || token == "" || len(page["events"].([]any)) != 1 {
		t.Fatalf("live prefix not resumable: %v", page)
	}
	firstPosition := logPosition(t, page["events"].([]any)[0])
	empty := logsParityPage(t, ctx, session, host, "polled", token)
	if len(empty["events"].([]any)) != 0 || empty["committedPosition"] != page["committedPosition"] {
		t.Fatalf("unchanged live head duplicated records: %v", empty)
	}
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "sibling", "view": "logs", "nextToken": token}), "worker_session.invalid_request", false)
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "polled", "view": "logs", "nextToken": "invalid"}), "worker_session.invalid_request", false)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "polled", "operation": "TERMINATE"})
	waitControlSignal(t, stopped)
	assertTerminalLogsPolling(t, ctx, session, host, token, firstPosition)
	if sibling := getHost(t, host.URL()+"/worker-sessions/sibling").(map[string]any); sibling["state"] != "RUNNING" {
		t.Fatalf("polling affected sibling: %v", sibling)
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "sibling", "operation": "TERMINATE"})
	waitControlSignal(t, siblingStopped)
}

func assertTerminalLogsPolling(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, token string, firstPosition float64) {
	t.Helper()
	var page map[string]any
	var positions = []float64{firstPosition}
	for attempts := 0; attempts < 10; attempts++ {
		page = logsParityPage(t, ctx, session, host, "polled", token)
		for _, event := range page["events"].([]any) {
			position := logPosition(t, event)
			if position != positions[len(positions)-1]+1 {
				t.Fatalf("poll lost/repeated committed position: %v -> %v", positions, position)
			}
			positions = append(positions, position)
		}
		if page["nextToken"] == nil {
			break
		}
		token = page["nextToken"].(string)
	}
	if page["nextToken"] != nil || page["health"] != "COMPLETE" || float64(len(positions)) != page["committedPosition"] {
		t.Fatalf("terminal capture not drained: %v positions=%v", page, positions)
	}
	stable := getHost(t, host.URL()+"/worker-sessions/polled/logs").(map[string]any)
	if stable["committedPosition"] != page["committedPosition"] || len(stable["events"].([]any)) != len(positions) {
		t.Fatalf("terminal watermark changed: %v", stable)
	}
}

func logPosition(t *testing.T, value any) float64 {
	t.Helper()
	return value.(map[string]any)["event"].(map[string]any)["position"].(float64)
}

func assertIncompleteLogsPolling(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer) {
	t.Helper()
	if page := logsParityPage(t, ctx, session, host, "lost-worker", ""); page["health"] != "INCOMPLETE" || page["nextToken"] == nil {
		t.Fatalf("MCP logs hid incomplete recovered history: %v", page)
	}
}

func logsParityPage(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, id, token string) map[string]any {
	t.Helper()
	args := map[string]any{"workerSessionId": id, "view": "logs", "limit": 1}
	query := url.Values{"limit": {"1"}}
	cliArgs := []string{"you", "--server", host.URL(), "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--limit", "1", "--output", "json"}
	if token != "" {
		args["nextToken"], query["nextToken"] = token, []string{token}
		cliArgs = append(cliArgs, "--next-token", token)
	}
	result := callWorker(t, ctx, session, "read", args)["result"].(map[string]any)
	if len(result) != 2 || result["session"].(map[string]any)["workerSessionId"] != id {
		t.Fatalf("logs view members: %v", result)
	}
	page := result["logs"].(map[string]any)
	assertJSONEqual(t, page, getHost(t, host.URL()+"/worker-sessions/"+url.PathEscape(id)+"/logs?"+query.Encode()))
	inputs := support.FakeInputs(ctx, cliArgs)
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI logs: %v stderr=%s", err, inputs.Stderr())
	}
	var cliPage map[string]any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cliPage); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, page, cliPage)
	return page
}

package workersessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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

// The sole provider invocation emits a small capture spanning default paging.
// Public termination freezes that capture before parallel read comparisons.
type limitHostRunner struct {
	started chan (<-chan struct{})
	calls   atomic.Int32
}

func (r *limitHostRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.RunStreaming(ctx, req, nil)
}
func (r *limitHostRunner) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte(`{"type":"thread.started","thread_id":"session_fixture_codex_success"}`+"\n"))
		for i := 0; i < 101; i++ {
			observer(platformprocess.OutputStreamStdout, []byte(fmt.Sprintf(`{"type":"item.completed","item":{"id":"message-%d","type":"agent_message","text":"captured message %d"}}`+"\n", i, i)))
		}
	}
	done := make(chan struct{})
	r.started <- done
	<-ctx.Done()
	close(done)
	return platformprocess.CommandResult{}, ctx.Err()
}

func runLogsLimitValidation(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "logs-limit")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := &limitHostRunner{started: make(chan (<-chan struct{}), 1)}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	done := admitControlWorker(t, t.Context(), host.URL(), "limit-worker", controlHostRunner{started: runner.started})
	requestHost(t, http.MethodPost, host.URL()+"/worker-sessions/limit-worker/terminate")
	waitControlSignal(t, done)
	target, err := url.Parse(host.URL())
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	var requests atomic.Int32
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Error("read matrix attempted a mutation")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(relay.Close)
	session, ctx := startMCP(t, process, relay.URL)
	for _, limit := range []string{"0", "-1", "1001"} {
		before := requests.Load()
		assertLimitCLIError(t, process, relay.URL, "limit-worker", []string{"--limit", limit}, "BAD_REQUEST", true)
		if requests.Load() != before {
			t.Fatal("local invalid limit contacted host")
		}
	}
	assertLimitHTTPError(t, relay.URL+"/worker-sessions/limit-worker/logs?limit=0", http.StatusBadRequest, "BAD_REQUEST")
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "limit-worker", "view": "logs", "limit": 0}), "worker_session.invalid_request", false)
	for _, limit := range []string{"", "1", "100", "1000"} {
		t.Run("page-limit="+limit, func(t *testing.T) {
			t.Parallel()
			page := limitParityPage(t, process, ctx, session, relay.URL, limit)
			assertLimitPageSize(t, page, limit)
		})
	}
	t.Run("errors-and-transcript", func(t *testing.T) {
		t.Parallel()
		assertLimitReadCompatibility(t, process, ctx, session, relay.URL)
		if runner.calls.Load() != 1 {
			t.Fatalf("read invoked provider: %d calls", runner.calls.Load())
		}
	})
}

func limitParityPage(t *testing.T, process support.Process, ctx context.Context, session *mcp.ClientSession, server, limit string) map[string]any {
	t.Helper()
	args := map[string]any{"workerSessionId": "limit-worker", "view": "logs"}
	query := ""
	flags := []string{}
	if limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil {
			t.Fatal(err)
		}
		args["limit"] = n
		query = "?limit=" + limit
		flags = append(flags, "--limit", limit)
	}
	page := callWorker(t, ctx, session, "read", args)["result"].(map[string]any)["logs"].(map[string]any)
	assertJSONEqual(t, page, getHost(t, server+"/worker-sessions/limit-worker/logs"+query))
	inputs := support.FakeInputs(ctx, append([]string{"you", "--server", server, "worker-sessions", "read", "--worker-session-id", "limit-worker", "--view", "logs", "--output", "json"}, flags...))
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("CLI corrected read: %v %s", err, inputs.Stderr())
	}
	var cliPage map[string]any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cliPage); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, page, cliPage)
	if limit == "" {
		assertJSONEqual(t, page, getHost(t, server+"/worker-sessions/limit-worker/logs?limit=100"))
	}
	return page
}

func assertLimitCLIError(t *testing.T, process support.Process, server, id string, flags []string, code string, bounds bool) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "--json", "--server", server, "worker-sessions", "read", "--worker-session-id", id, "--view", "logs"}, flags...))
	err := process.Execute(inputs.Input)
	var typed interface{ CLIErrorCode() string }
	if !errors.As(err, &typed) || typed.CLIErrorCode() != code || errors.Is(err, context.Canceled) || inputs.Stdout() != "" {
		t.Fatalf("CLI error=%v stdout=%q stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	decoder := json.NewDecoder(strings.NewReader(inputs.Stderr()))
	var diagnostic map[string]any
	if err := decoder.Decode(&diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic["code"] != code {
		t.Fatalf("diagnostic: %v", diagnostic)
	}
	if bounds && (diagnostic["family"] != "BAD_REQUEST" || diagnostic["message"] != "--limit must be between 1 and 1000") {
		t.Fatalf("bounds diagnostic: %v", diagnostic)
	}
	if err := decoder.Decode(&diagnostic); err != io.EOF {
		t.Fatalf("extra diagnostic: %v", err)
	}
}

func assertLimitHTTPError(t *testing.T, endpoint string, status int, code string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status || body["code"] != code || body["events"] != nil {
		t.Fatalf("HTTP status=%d body=%v", response.StatusCode, body)
	}
}

func assertLimitReadCompatibility(t *testing.T, process support.Process, ctx context.Context, session *mcp.ClientSession, server string) {
	t.Helper()
	for _, test := range []struct {
		id, token, cli, mcp string
		status              int
	}{
		{"limit-worker", "malformed", "BAD_REQUEST", "worker_session.invalid_request", http.StatusBadRequest},
		{"missing-limit-worker", "", "WORKER_SESSION_NOT_FOUND", "worker_session.not_found", http.StatusNotFound},
	} {
		flags := []string{"--limit", "1"}
		args := map[string]any{"workerSessionId": test.id, "view": "logs", "limit": 1}
		query := "?limit=1"
		if test.token != "" {
			flags = append(flags, "--next-token", test.token)
			args["nextToken"] = test.token
			query += "&nextToken=" + test.token
		}
		assertLimitCLIError(t, process, server, test.id, flags, test.cli, false)
		assertLimitHTTPError(t, server+"/worker-sessions/"+test.id+"/logs"+query, test.status, test.cli)
		assertToolError(t, callAction(t, ctx, session, "READ", args), test.mcp, false)
	}
	transcript := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": "limit-worker", "view": "transcript"})["result"].(map[string]any)["transcript"]
	assertTranscriptEqual(t, transcript, getHost(t, server+"/worker-sessions/limit-worker/transcript"))
	inputs := support.FakeInputs(ctx, []string{"you", "--server", server, "worker-sessions", "read", "--worker-session-id", "limit-worker", "--output", "json"})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("default transcript: %v %s", err, inputs.Stderr())
	}
	var actual any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &actual); err != nil {
		t.Fatal(err)
	}
	assertTranscriptEqual(t, transcript, actual)
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("closed host received request") }))
	closed.Close()
	unavailable, unavailableCtx := startMCP(t, process, closed.URL)
	assertLimitCLIError(t, process, closed.URL, "limit-worker", []string{"--limit", "1"}, "FACTORY_UNREACHABLE", false)
	assertLimitCLIError(t, process, closed.URL, "limit-worker", []string{"--limit", "0"}, "BAD_REQUEST", true)
	assertToolError(t, callAction(t, unavailableCtx, unavailable, "READ", map[string]any{"workerSessionId": "limit-worker", "view": "logs", "limit": 1}), "worker_session.host_unavailable", true)
}

func assertLimitPageSize(t *testing.T, page map[string]any, limit string) {
	t.Helper()
	count := len(page["events"].([]any))
	if (limit == "" || limit == "100") && count != 100 || limit == "1" && count != 1 || limit == "1000" && (count <= 100 || count >= 1000) {
		t.Fatalf("limit %q event count %d", limit, count)
	}
	if limit != "1000" && page["nextToken"] == nil {
		t.Fatal("bounded prefix has no cursor")
	}
	if limit == "1000" && page["nextToken"] != nil {
		t.Fatal("complete page has continuation")
	}
}

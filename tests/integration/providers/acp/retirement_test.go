package acp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// These two real-boundary cells share one prebuilt CLI. Each daemon owns its
// explicit durable sessions. Cells run sequentially to cap owned children at
// the runner, daemon and two Node peers; this is a host budget, not a product lock.
func testDeliveredACPEarlyFailureRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	binary, node, peer := deliveredRetirementTools(t)
	directory := t.TempDir()
	env := builtcliacceptance.ProcessEnvForIsolatedHome(directory)
	journal := filepath.Join(directory, "launches.txt")
	launch := fmt.Sprintf("%q %q initialize-disconnect-once %q", node, peer, journal)
	_, stderr, err := invokeCLI(ctx, binary, directory, env,
		"workers", "acp", "add", "--name", "opencode", "--transport", "stdio", "--argument", launch)
	if err != nil {
		t.Fatalf("register peer: %v %s", err, stderr)
	}
	baseURL := startDeliveredACPDaemon(t, ctx, binary, directory, env)
	client := &http.Client{Timeout: 20 * time.Second}
	defer client.CloseIdleConnections()
	first := invokeDeliveredACPWorkflow(t, ctx, client, baseURL, "early-failure")
	if first.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("early failure = %#v", first)
	}
	assertDeliveredLaunchJournal(t, journal, "initialize-disconnect\n")
	assertDeliveredLaunchJournal(t, journal+".exits", "initialize-disconnect\n")
	var dispatches factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(deliveredACPRead(t, ctx, client, baseURL+"/factory-sessions/"+first.SessionId+"/dispatches"), &dispatches); err != nil {
		t.Fatal(err)
	}
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].FailureDetail == nil {
		t.Fatalf("early failure dispatches = %#v", dispatches)
	}
	failure := dispatches.Dispatches[0].FailureDetail
	if !strings.Contains(failure.Message, `ACP provider "opencode" disconnected before responding; retry the request`) {
		t.Fatalf("initialize failure classification = %#v", failure)
	}
	second := invokeDeliveredACPWorkflow(t, ctx, client, baseURL, "early-recovery")
	if second.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded || first.SessionId == second.SessionId {
		t.Fatalf("fresh recovery = %#v", second)
	}
	assertDeliveredSessionResult(t, ctx, client, baseURL, second.SessionId, "delivered EOF primary result")
	assertDeliveredLaunchJournal(t, journal, "initialize-disconnect\nsuccess\n")
	readDeliveredSessionEvents(t, ctx, client, baseURL, first.SessionId)
	readDeliveredSessionEvents(t, ctx, client, baseURL, second.SessionId)
}

func testDeliveredACPFactoryCancelPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	binary, node, peer := deliveredRetirementTools(t)
	directory := t.TempDir()
	env := builtcliacceptance.ProcessEnvForIsolatedHome(directory)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	deadline, _ := ctx.Deadline()
	if err := listener.(*net.TCPListener).SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cancelled", "survivor"} {
		launch := fmt.Sprintf("%q %q controlled %s %s", node, peer, listener.Addr(), id)
		_, stderr, err := invokeCLI(ctx, binary, directory, env,
			"workers", "acp", "add", "--name", id, "--transport", "stdio", "--argument", launch)
		if err != nil {
			t.Fatalf("register %s: %v %s", id, err, stderr)
		}
	}
	baseURL := startDeliveredACPDaemon(t, ctx, binary, directory, env)
	client := &http.Client{Timeout: 20 * time.Second}
	defer client.CloseIdleConnections()
	sessions := make(map[string]string)
	connections := make(map[string]net.Conn)
	readers := make(map[string]*bufio.Reader)
	pids := make(map[string]int)
	for _, id := range []string{"cancelled", "survivor"} {
		sessionID, connection, reader, pid := admitDeliveredRetirementPeer(t, ctx, client, baseURL, id, listener, deadline)
		sessions[id] = sessionID
		connections[id], readers[id], pids[id] = connection, reader, pid
	}
	if sessions["cancelled"] == sessions["survivor"] || pids["cancelled"] == pids["survivor"] {
		t.Fatal("overlapping attempts did not own distinct sessions and processes")
	}
	var control factoryapi.FactorySessionLifecycleControlResponse
	deliveredRetirementPost(t, ctx, client, baseURL+"/factory-sessions/"+sessions["cancelled"]+"/cancel", map[string]any{}, &control)
	if control.SessionId != sessions["cancelled"] || control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("selected cancel = %#v", control)
	}
	if _, err := fmt.Fprintf(connections["survivor"], "observe-exit %d\n", pids["cancelled"]); err != nil {
		t.Fatal(err)
	}
	line, err := readers["survivor"].ReadString('\n')
	if err != nil || line != fmt.Sprintf("survivor exited %d\n", pids["cancelled"]) {
		t.Fatalf("selected process exit while B stays active = %q, error=%v", line, err)
	}
	assertDeliveredSessionStatus(t, ctx, client, baseURL, sessions["survivor"], factoryapi.FactorySessionDurableLifecycleStatusRunning)
	readDeliveredSessionEvents(t, ctx, client, baseURL, sessions["cancelled"])
	assertDeliveredSessionStatus(t, ctx, client, baseURL, sessions["cancelled"], factoryapi.FactorySessionDurableLifecycleStatusCanceled)
	if _, err := io.WriteString(connections["survivor"], "release\n"); err != nil {
		t.Fatal(err)
	}
	events := readDeliveredSessionEvents(t, ctx, client, baseURL, sessions["survivor"])
	if strings.Contains(events, "delivered EOF primary result cancelled") {
		t.Fatal("B received A's result events")
	}
	assertDeliveredSessionStatus(t, ctx, client, baseURL, sessions["survivor"], factoryapi.FactorySessionDurableLifecycleStatusSucceeded)
	assertDeliveredSessionResult(t, ctx, client, baseURL, sessions["survivor"], "delivered EOF primary result survivor")
}

func deliveredRetirementTools(t *testing.T) (string, string, string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := filepath.Abs(filepath.Join("testdata", "delivered-peer.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	return prebuiltCLI(t), node, peer
}

func deliveredRetirementRequest(t *testing.T, ctx context.Context, client *http.Client, baseURL, id string) factoryapi.FactorySessionExecutionRequest {
	t.Helper()
	var factory factoryapi.Factory
	if err := json.Unmarshal(deliveredACPRead(t, ctx, client, baseURL+"/factory-sessions/~default/factory"), &factory); err != nil {
		t.Fatal(err)
	}
	return factoryapi.FactorySessionExecutionRequest{RequestId: id, Orchestrator: factory.Orchestrator,
		Source: factoryapi.FactorySessionExecutionSource{Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
				Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
				Inline: fmt.Sprintf(`return (async function () {
  const result = await agent.run({label: %q, prompt: "complete one turn", executorProvider: "ACP", modelProvider: %q, model: "fixture"});
  if (result.status !== "COMPLETED") { throw "ACP child failed"; }
  return result.output.text;
})();`, id, id),
			}}}}
}

func deliveredRetirementPost(t *testing.T, ctx context.Context, client *http.Client, url string, body, result any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || (response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted) {
		t.Fatalf("POST %s: status=%d error=%v body=%s", url, response.StatusCode, err, data)
	}
	if err := json.Unmarshal(data, result); err != nil {
		t.Fatalf("decode response %s: %v", data, err)
	}
}

func assertDeliveredSessionStatus(t *testing.T, ctx context.Context, client *http.Client, baseURL, id string, status factoryapi.FactorySessionDurableLifecycleStatus) {
	t.Helper()
	var session factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(deliveredACPRead(t, ctx, client, baseURL+"/factory-sessions/"+id), &session); err != nil || session.SessionId != id || session.Status != status {
		t.Fatalf("session = %#v, error=%v, want %s", session, err, status)
	}
}

func assertDeliveredSessionResult(t *testing.T, ctx context.Context, client *http.Client, baseURL, id, text string) {
	t.Helper()
	var result factoryapi.FactorySessionResult
	if err := json.Unmarshal(deliveredACPRead(t, ctx, client, baseURL+"/factory-sessions/"+id+"/results"), &result); err != nil || result.SessionId != id || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("session result = %#v, error=%v", result, err)
	}
	encoded, err := json.Marshal(result.PrimaryResult)
	if err != nil || !strings.Contains(string(encoded), text) || strings.Contains(string(encoded), "delivered EOF primary result cancelled") {
		t.Fatalf("session primary result = %s, error=%v", encoded, err)
	}
}

// Canonical terminal events drive completion, including already retained history.
// Identity is checked where present; startup frames need no invented session ID.
func readDeliveredSessionEvents(t *testing.T, ctx context.Context, client *http.Client, baseURL, id string) string {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/factory-sessions/"+id+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var events strings.Builder
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "data:"))
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatal(err)
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != id {
			t.Fatalf("event crossed session boundary: %s", data)
		}
		events.WriteString(data)
		if event.Type == factoryapi.FactoryEventTypeSessionCompleted {
			return events.String()
		}
	}
	t.Fatalf("no canonical terminal event: %v; events=%s", scanner.Err(), events.String())
	return ""
}

func admitDeliveredRetirementPeer(t *testing.T, ctx context.Context, client *http.Client, baseURL, id string, listener net.Listener, deadline time.Time) (string, net.Conn, *bufio.Reader, int) {
	t.Helper()
	request := deliveredRetirementRequest(t, ctx, client, baseURL, id)
	var admission factoryapi.FactorySessionExecutionResponse
	deliveredRetirementPost(t, ctx, client, baseURL+"/factory-sessions/async", request, &admission)
	if admission.SessionId == "" {
		t.Fatalf("missing hosted session identity: %#v", admission)
	}
	connection, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if err := connection.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	line, err := reader.ReadString('\n')
	var observedID string
	var pid int
	_, parseErr := fmt.Sscanf(line, "%s started %d\n", &observedID, &pid)
	if err != nil || parseErr != nil || observedID != id || pid <= 0 {
		t.Fatalf("real prompt readiness = %q, error=%v", line, err)
	}
	return admission.SessionId, connection, reader, pid
}

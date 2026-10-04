package acp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestPrebuiltACPDeliveredResultsSurvivePeerExit exercises the provider's actual
// stdio and process-wait boundary through a compiled CLI. The external peer
// flushes its response and exits immediately; no keepalive or timing padding
// separates the response from EOF. This package never builds the deliverable.
func TestPrebuiltACPDeliveredResultsSurvivePeerExit(t *testing.T) {
	binary := prebuiltCLI(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for the external ACP peer fixture")
	}
	peer, err := filepath.Abs(filepath.Join("testdata", "delivered-peer.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"initialize-version", "prompt-result", "custom-prompt-result", "prompt-disconnect", "prompt-secret-disconnect"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			directory := t.TempDir()
			env := builtcliacceptance.ProcessEnvForIsolatedHome(directory)
			provider := "opencode"
			if mode == "custom-prompt-result" {
				provider = "eof-peer"
			}
			launch := fmt.Sprintf("%q %q %s", node, peer, mode)
			_, diagnostic, err := invokeCLI(ctx, binary, directory, env,
				"workers", "acp", "add", "--name", provider, "--transport", "stdio", "--argument", launch)
			if err != nil {
				t.Fatalf("register external peer: %v\n%s", err, diagnostic)
			}
			args := []string{"run", "--named", "@you/subagent", "--worker-provider", provider,
				"--worker-model", "fixture", "--working-root", directory, "Return the fixture result"}
			if mode == "prompt-secret-disconnect" {
				args = []string{"run", "--factory", writeDeliveredSecretFactory(t, directory), "Return the fixture result"}
			}
			stdout, stderr, err := invokeCLI(ctx, binary, directory, env, args...)
			if mode == "initialize-version" {
				assertDeliveredUnsupportedVersion(t, stdout, stderr, err)
				return
			}
			if mode == "prompt-disconnect" || mode == "prompt-secret-disconnect" {
				assertDeliveredDisconnect(t, stdout, stderr, err)
				if mode == "prompt-secret-disconnect" {
					if strings.Contains(stdout+stderr, deliveredACPSecret) || !strings.Contains(stdout+stderr, "agent diagnostic token=<redacted>") {
						t.Fatalf("prompt disconnect lost safe configured diagnostic: stdout=%q stderr=%q", stdout, stderr)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("delivered Prompt failed after peer exit: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			if !strings.Contains(stdout, "delivered EOF primary result") {
				t.Fatalf("primary result was lost after peer exit: stdout=%q stderr=%q", stdout, stderr)
			}
		})
	}
	// The hosted prebuilt target selects this parent by exact name. Keep the
	// same-daemon witness in that selection, sharing the upstream artifact.
	t.Run("daemon-recovery", testDeliveredACPDaemonRecoversAfterDisconnect)
	t.Run("daemon-worker-disconnect-recovery", testDeliveredACPWorkerDisconnectRecovery)
}

const deliveredACPSecret = "delivered-acp-configured-secret-token"

// Direct Worker Sessions retain their public terminal classification after
// disconnect reconciliation. Both real attempts use the same daemon; configured
// stderr must not escape into either caller's terminal output.
func testDeliveredACPWorkerDisconnectRecovery(t *testing.T) {
	binary := prebuiltCLI(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := filepath.Abs(filepath.Join("testdata", "delivered-peer.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	directory := t.TempDir()
	env := builtcliacceptance.ProcessEnvForIsolatedHome(directory)
	journal := filepath.Join(directory, "attempts.txt")
	launch := fmt.Sprintf("%q %q disconnect-once %q", node, peer, journal)
	_, diagnostic, err := invokeCLI(ctx, binary, directory, env,
		"workers", "acp", "add", "--name", "opencode", "--transport", "stdio", "--argument", launch)
	if err != nil {
		t.Fatalf("register peer: %v %s", err, diagnostic)
	}
	baseURL := startDeliveredACPDaemon(t, ctx, binary, directory, env)
	client := &http.Client{Timeout: 20 * time.Second}
	defer client.CloseIdleConnections()
	for _, id := range []string{"redacted-disconnect", "fresh-recovery"} {
		request := map[string]any{
			"requestId": id, "workerSessionId": id,
			"execution": map[string]any{
				"factorySessionId": "~default", "workstationName": "__provider_invocation__", "workerType": "direct-worker",
				"workingDirectory": directory, "workstationType": "MODEL_WORKSTATION", "runnerId": "opencode", "executorProvider": "ACP", "modelProvider": "opencode", "model": "fixture",
				"userMessage": "complete one turn", "envVars": map[string]string{"ACP_TEST_API_TOKEN": deliveredACPSecret},
				"dispatch": map[string]any{"dispatchId": id, "workstationName": "__provider_invocation__", "workerType": "direct-worker"},
			},
		}
		payload, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		stdout, stderr, runErr := invokeCLI(ctx, binary, directory, env,
			"--remote", "--server", baseURL, "--json", "worker-sessions", "invoke", "--execution", string(payload), "--retry-max-attempts", "1")
		if strings.Contains(stdout+stderr, deliveredACPSecret) {
			t.Fatal("direct Worker Session leaked the configured secret")
		}
		if id == "redacted-disconnect" {
			var failure factoryapi.ErrorResponse
			if err := json.Unmarshal([]byte(stderr), &failure); err != nil {
				t.Fatalf("decode direct failure: %v stderr=%q", err, stderr)
			}
			// Direct supervision currently reports the reconciled unsupported
			// continuation class, just as the JavaScript dispatch witness does.
			if runErr == nil || stdout != "" || failure.Code != "WORKER_SESSION_FAILED" || failure.Message != "family=terminal type=permanent_bad_request" {
				t.Fatalf("direct disconnect: error=%v stdout=%q stderr=%q", runErr, stdout, stderr)
			}
			assertDeliveredLaunchJournal(t, journal, "disconnect\n")
			assertDeliveredWorkerTerminal(t, ctx, client, baseURL, id, "FAILED")
		} else {
			if runErr != nil || !strings.Contains(stdout, "delivered EOF primary result") {
				t.Fatalf("direct recovery: error=%v stdout=%q stderr=%q", runErr, stdout, stderr)
			}
			assertDeliveredLaunchJournal(t, journal, "disconnect\nsuccess\n")
			assertDeliveredWorkerTerminal(t, ctx, client, baseURL, id, "COMPLETED")
		}
	}
}

func assertDeliveredWorkerTerminal(t *testing.T, ctx context.Context, client *http.Client, baseURL, id, state string) {
	t.Helper()
	data := deliveredACPRead(t, ctx, client, baseURL+"/worker-sessions/"+id)
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(data, &observation); err != nil {
		t.Fatal(err)
	}
	if !observation.Direct || observation.WorkerSessionId != id || observation.AttemptId != id || string(observation.State) != state || observation.EndedAt == nil {
		t.Fatalf("direct terminal observation = %#v, want %s with request-owned identities and end time", observation, state)
	}
	if state == "FAILED" {
		if observation.Failure == nil || observation.Failure.Detail != "family=terminal type=permanent_bad_request" {
			t.Fatalf("direct failure classification = %#v", observation.Failure)
		}
	} else if observation.Failure != nil {
		t.Fatalf("recovered direct session retained failure = %#v", observation.Failure)
	}
}

func writeDeliveredSecretFactory(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(directory, "redaction.yaml")
	source := `name: delivered-redaction
workTypes:
  - name: task
    handlingBehavior: [DEFAULT]
    states:
      - {name: init, type: INITIAL}
      - {name: done, type: TERMINAL}
      - {name: failed, type: FAILED}
workers:
  - name: worker
    type: MODEL_WORKER
    modelProvider: opencode
    model: fixture
    body: Complete one turn.
workstations:
  - name: process
    type: MODEL_WORKSTATION
    worker: worker
    env:
      ACP_TEST_API_TOKEN: ` + deliveredACPSecret + `
    inputs: [{workType: task, state: init}]
    outputs: [{workType: task, state: done}]
    onFailure: [{workType: task, state: failed}]
    body: Complete one turn.
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// One delivered daemon owns both attempts. The real external peer disconnects
// on its first launch and succeeds on the next customer request; its launch
// journal also detects an unwanted retry of the failed request.
func testDeliveredACPDaemonRecoversAfterDisconnect(t *testing.T) {
	binary := prebuiltCLI(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := filepath.Abs(filepath.Join("testdata", "delivered-peer.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	directory := t.TempDir()
	env := builtcliacceptance.ProcessEnvForIsolatedHome(directory)
	journal := filepath.Join(directory, "attempts.txt")
	launch := fmt.Sprintf("%q %q disconnect-once %q", node, peer, journal)
	_, diagnostic, err := invokeCLI(ctx, binary, directory, env,
		"workers", "acp", "add", "--name", "opencode", "--transport", "stdio", "--argument", launch)
	if err != nil {
		t.Fatalf("register peer: %v %s", err, diagnostic)
	}
	baseURL := startDeliveredACPDaemon(t, ctx, binary, directory, env)
	client := &http.Client{Timeout: 20 * time.Second}
	defer client.CloseIdleConnections()
	first := invokeDeliveredACPWorkflow(t, ctx, client, baseURL, "disconnect")
	if first.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("disconnected session = %#v, want FAILED", first)
	}
	if first.Result != nil && first.Result.PrimaryResult != nil && len(*first.Result.PrimaryResult) != 0 {
		t.Fatalf("disconnected session returned primary content: %#v", first.Result)
	}
	assertDeliveredLaunchJournal(t, journal, "disconnect\n")
	var dispatches factoryapi.ListFactorySessionDispatchesResponse
	data := deliveredACPRead(t, ctx, client, baseURL+"/factory-sessions/"+first.SessionId+"/dispatches")
	if err := json.Unmarshal(data, &dispatches); err != nil {
		t.Fatal(err)
	}
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].FailureDetail == nil {
		t.Fatalf("failed dispatches = %#v, want one typed failure", dispatches)
	}
	// Preserve the documented detached-retry outcome: reconciliation currently
	// replaces the initial disconnect with unsupported continuation. The direct
	// delivered witness above separately pins the exact disconnect diagnostic.
	detail := dispatches.Dispatches[0].FailureDetail
	if detail.Reason != factoryapi.WorkFailureTypePermanentBadRequest || detail.Message != "provider session continuation is unsupported" {
		t.Fatalf("disconnect reconciliation = %#v", detail)
	}
	second := invokeDeliveredACPWorkflow(t, ctx, client, baseURL, "recovery")
	if second.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded || second.SessionId == first.SessionId {
		t.Fatalf("recovery session = %#v, want distinct SUCCEEDED session", second)
	}
	encoded, err := json.Marshal(second.Result)
	if err != nil || !strings.Contains(string(encoded), "delivered EOF primary result") {
		t.Fatalf("recovery primary result = %s, error=%v", encoded, err)
	}
	assertDeliveredLaunchJournal(t, journal, "disconnect\nsuccess\n")
}

func assertDeliveredLaunchJournal(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != expected {
		t.Fatalf("ACP launches = %q, error=%v, want %q", data, err, expected)
	}
}

func startDeliveredACPDaemon(t *testing.T, ctx context.Context, binary, directory string, env []string) string {
	t.Helper()
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	baseURL := "http://" + address
	factoryDirectory := filepath.Join(directory, "factory")
	if err := os.MkdirAll(factoryDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	definition := `{"name":"delivered-acp","orchestrator":{"kind":"JAVASCRIPT","javascript":{"inlineSource":{"encoding":"utf-8","inline":"return \"ready\";"}}}}`
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, "server", "--listen", address)
	command.Dir, command.Env = directory, env
	log, err := os.Create(filepath.Join(directory, "daemon.log"))
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second): //nolint:testsleep // Failure ceiling for joining the killed real OS process; completion comes from command.Wait, not elapsed time.
			t.Error("delivered daemon did not join after kill")
		}
		_ = log.Close()
	})
	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.Get(baseURL + "/factory-sessions/~default/factory")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return baseURL
			}
		}
		select {
		case err := <-done:
			done <- err
			data, _ := os.ReadFile(filepath.Join(directory, "daemon.log"))
			t.Fatalf("daemon exited before readiness: %v %s", err, data)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func invokeDeliveredACPWorkflow(t *testing.T, ctx context.Context, client *http.Client, baseURL, requestID string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	var factory factoryapi.Factory
	if err := json.Unmarshal(deliveredACPRead(t, ctx, client, baseURL+"/factory-sessions/~default/factory"), &factory); err != nil {
		t.Fatal(err)
	}
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline: `return (async function () {
  const result = await agent.run({label: "delivered-agent", prompt: "complete one turn", executorProvider: "ACP", modelProvider: "opencode", model: "fixture"});
  if (result.status !== "COMPLETED") { throw "ACP child failed"; }
  return result.output.text;
})();`,
				},
			},
		},
		Orchestrator: factory.Orchestrator,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/factory-sessions/sync", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := client.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("workflow response=%d body=%s error=%v", response.StatusCode, data, err)
	}
	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(data, &result); err != nil || result.SessionId == "" {
		t.Fatalf("workflow result=%s error=%v", data, err)
	}
	return result
}

func deliveredACPRead(t *testing.T, ctx context.Context, client *http.Client, url string) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("read %s: status=%d error=%v body=%s", url, response.StatusCode, err, data)
	}
	return data
}

// This pins the real EOF failure boundary. Configured redaction is asserted
// by its selected caller; live peer cancellation remains separate evidence.
func assertDeliveredDisconnect(t *testing.T, stdout, stderr string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("prompt disconnect succeeded: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, `ACP provider "opencode" disconnected before responding; retry the request`) {
		t.Fatalf("prompt disconnect lost its diagnostic: error=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if strings.Contains(stdout, "status: SUCCESS") || strings.Contains(stdout, "delivered EOF primary result") {
		t.Fatalf("prompt disconnect returned a success: stdout=%q stderr=%q", stdout, stderr)
	}
}

func prebuiltCLI(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("INFINITE_YOU_INTEGRATION_BINARY"))
	if path == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("INFINITE_YOU_INTEGRATION_BINARY is required; the test never builds the CLI")
		}
		t.Skip("set INFINITE_YOU_INTEGRATION_BINARY to a previously compiled CLI")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		t.Fatalf("invalid compiled CLI artifact %q: %v", path, err)
	}
	return path
}

func invokeCLI(ctx context.Context, binary, directory string, env []string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = directory, env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func assertDeliveredUnsupportedVersion(t *testing.T, stdout, stderr string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("unsupported initialize version succeeded: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "unsupported protocol version 999") {
		t.Fatalf("delivered unsupported-version failure was lost: error=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "disconnected before responding") || strings.Contains(stdout, "status: SUCCESS") || strings.Contains(stdout, "delivered EOF primary result") {
		t.Fatalf("unsupported version became disconnect or success: stdout=%q stderr=%q", stdout, stderr)
	}
}

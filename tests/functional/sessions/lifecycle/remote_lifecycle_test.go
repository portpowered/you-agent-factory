package lifecycle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIRemoteRunStartsDurableSessionOnSelectedServer proves the public
// root.BuildProcess/Process.Execute client path admits work on a real
// root-built server, survives submitting-client disconnect, and leaves the
// returned server-owned session inspectable. The same public server also
// proves request-id replay and conflict behavior without a fabricated HTTP
// responder.
func TestCLIRemoteRunStartsDurableSessionOnSelectedServer(t *testing.T) {
	homeDir := t.TempDir()
	namedFactoryDir := scaffoldRemotePlacementFactory(t, homeDir)
	server := startRemotePlacementServer(t, homeDir, namedFactoryDir)
	runRemoteClientDisconnect(t, homeDir, namedFactoryDir, server.URL())
	assertRemoteRequestIDBehavior(t, server.URL())
}

func scaffoldRemotePlacementFactory(t *testing.T, homeDir string) string {
	t.Helper()
	sourceDir := support.ScaffoldFactory(t, map[string]any{
		"name": "remote-placement",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name":     "prompt",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   "var spin = 0; while (true) { spin += 1; }",
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	namedFactoryDir := support.CreateNamedFactory(
		t,
		homeDir,
		sourceDir,
		"remote-placement",
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)
	workflowDir := filepath.Join(namedFactoryDir, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflow directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "remote-idempotency.js"), []byte("return 'idempotency complete';"), 0o600); err != nil {
		t.Fatalf("write idempotency workflow: %v", err)
	}
	return namedFactoryDir
}

func startRemotePlacementServer(t *testing.T, homeDir, namedFactoryDir string) *support.FunctionalAPIServer {
	t.Helper()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                namedFactoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Env:                       []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir},
		Edges: serviceedges.Edges{
			FactoryRuntimeWorkflowHome: func() (string, error) {
				return homeDir, nil
			},
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	return server
}

func runRemoteClientDisconnect(t *testing.T, homeDir, namedFactoryDir, serverURL string) {
	t.Helper()
	args := []string{
		"you", "--remote", "--server", serverURL, "--verbose", "run",
		"--named", "remote-placement", "--json", "--output", "response-stream", "--no-record", "same request",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.WorkingDirectory = namedFactoryDir
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	admissionOutput := newRemoteAdmissionOutput()
	inputs.Input.Stdout = admissionOutput
	inputs.Input.Stderr = admissionOutput
	clientProcess := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, clientProcess)
	clientDone := make(chan error, 1)
	go func() { clientDone <- clientProcess.Execute(inputs.Input) }()
	select {
	case <-admissionOutput.started:
	case <-time.After(10 * time.Second):
		cancel()
		select {
		case clientErr := <-clientDone:
			t.Fatalf("remote client did not observe durable admission: client error=%v\nadmission output:\n%s", clientErr, admissionOutput.String())
		case <-time.After(10 * time.Second):
			t.Fatalf("remote client did not observe durable admission\nadmission output:\n%s", admissionOutput.String())
		}
	}
	cancel()
	select {
	case clientErr := <-clientDone:
		if clientErr == nil {
			t.Fatal("remote client returned nil after submitting-client disconnect")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("remote client did not return after submitting connection cancellation")
	}
	sessionID := remoteSessionIDFromAdmissionOutput(t, admissionOutput.String())
	inspection := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
	durable, err := inspection.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session %s after client disconnect: %v", sessionID, err)
	}
	if durable.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want returned %q", durable.SessionId, sessionID)
	}
	if durable.Status != factoryapi.FactorySessionDurableLifecycleStatusQueued && durable.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("durable session status after client disconnect = %q, want QUEUED or RUNNING", durable.Status)
	}
	if err := terminateRemoteFunctionalSession(serverURL, sessionID); err != nil {
		t.Fatalf("terminate disconnected durable session %s: %v", sessionID, err)
	}
	if strings.Contains(admissionOutput.String(), "127.0.0.1:1") {
		t.Fatalf("remote output leaked a local fallback endpoint: %s", admissionOutput.String())
	}
}

func assertRemoteRequestIDBehavior(t *testing.T, serverURL string) {
	t.Helper()
	retryWorkflow := "remote-idempotency"
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: "functional-remote-retry-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &retryWorkflow,
		},
	}
	first := postRemoteFunctionalExecution(t, serverURL, request)
	replay := postRemoteFunctionalExecution(t, serverURL, request)
	if first.SessionId == "" || replay.SessionId != first.SessionId {
		t.Fatalf("request-id replay sessions = %q/%q, want one server-owned identity", first.SessionId, replay.SessionId)
	}
	conflictRequest := request
	conflictRequest.Source.WorkflowName = strPtr("remote-idempotency-conflict")
	conflictStatus, conflict := postRemoteFunctionalExecutionConflict(t, serverURL, conflictRequest)
	if conflictStatus != http.StatusConflict || conflict.Code != factoryapi.ErrorResponseCode("EXECUTION_REQUEST_ID_CONFLICT") {
		t.Fatalf("request-id conflict = status %d response %#v, want 409 EXECUTION_REQUEST_ID_CONFLICT", conflictStatus, conflict)
	}
	if err := terminateRemoteFunctionalSession(serverURL, first.SessionId); err != nil {
		t.Fatalf("terminate replayed durable session %s: %v", first.SessionId, err)
	}
}

type remoteAdmissionOutput struct {
	mu      sync.Mutex
	output  strings.Builder
	started chan struct{}
	once    sync.Once
}

func newRemoteAdmissionOutput() *remoteAdmissionOutput {
	return &remoteAdmissionOutput{started: make(chan struct{})}
}

func (writer *remoteAdmissionOutput) Write(p []byte) (int, error) {
	writer.mu.Lock()
	_, _ = writer.output.Write(p)
	started := strings.Contains(writer.output.String(), "remote durable start response") || strings.Contains(writer.output.String(), "session-started/")
	writer.mu.Unlock()
	if started {
		writer.once.Do(func() { close(writer.started) })
	}
	return len(p), nil
}

func (writer *remoteAdmissionOutput) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.String()
}

func remoteSessionIDFromAdmissionOutput(t *testing.T, output string) string {
	t.Helper()
	for _, marker := range []string{"sessionId=", `"sessionId":"`} {
		start := strings.Index(output, marker)
		if start < 0 {
			continue
		}
		value := output[start+len(marker):]
		if end := strings.IndexAny(value, " \r\n\",}"); end >= 0 {
			value = value[:end]
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("durable admission output did not contain a session id: %s", output)
	return ""
}

func postRemoteFunctionalExecution(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	status, response, body := postRemoteFunctionalExecutionRaw(t, serverURL, request)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		t.Fatalf("remote durable retry status = %d body = %s", status, body)
	}
	return response
}

func postRemoteFunctionalExecutionConflict(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	status, _, body := postRemoteFunctionalExecutionRaw(t, serverURL, request)
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode remote request-id conflict: %v body=%s", err, body)
	}
	return status, response
}

func postRemoteFunctionalExecutionRaw(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) (int, factoryapi.FactorySessionExecutionResponse, []byte) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal remote durable request: %v", err)
	}
	httpRequest, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, strings.TrimSuffix(serverURL, "/")+"/factory-sessions/async", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build remote durable request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("POST remote durable request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read remote durable response: %v", err)
	}
	var decoded factoryapi.FactorySessionExecutionResponse
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if err := json.Unmarshal(responseBody, &decoded); err != nil {
			t.Fatalf("decode remote durable response: %v body=%s", err, responseBody)
		}
	}
	return response.StatusCode, decoded, responseBody
}

func terminateRemoteFunctionalSession(serverURL, sessionID string) error {
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/terminate",
		bytes.NewReader([]byte(`{}`)),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode == http.StatusConflict && strings.Contains(string(body), `"outcome":"TERMINAL_SESSION"`) {
			return nil
		}
		return fmt.Errorf("status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func strPtr(value string) *string { return &value }

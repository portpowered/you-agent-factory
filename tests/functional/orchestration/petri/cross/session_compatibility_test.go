package cross

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	busyLoopWorkflowName = "busy-loop"
	sessionCompatTimeout = 10 * time.Second
)

// TestPetriAndJavaScriptSessionsShareLifecycleControls proves the same public
// pause and resume lifecycle controls are accepted for a live Petri Factory
// Session and a live JavaScript Factory Session, and that each session's public
// status reflects paused then running states without inspecting orchestrator
// internals.
func TestPetriAndJavaScriptSessionsShareLifecycleControls(t *testing.T) {
	dir := scaffoldSessionCompatibilityFactory(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	baseURL := strings.TrimSuffix(server.URL(), "/")

	petriSession := support.GetDefaultSession(t, baseURL)
	assertLiveSessionOrchestratorKind(
		t,
		petriSession.Id,
		petriSession.Runtime.OrchestratorKind,
		factoryapi.PETRI,
	)

	javaSessionID := startBusyLoopJavaScriptSession(t, baseURL)
	waitForDurableSessionStatus(
		t,
		baseURL,
		javaSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		sessionCompatTimeout,
	)
	javaSession := readDurableSession(t, baseURL, javaSessionID)
	assertDurableSessionOrchestratorKind(
		t,
		javaSession.SessionId,
		javaSession.OrchestratorKind,
		factoryapi.JAVASCRIPT,
	)

	if petriSession.Id == javaSessionID {
		t.Fatalf("session ids must differ: petri=%q javascript=%q", petriSession.Id, javaSessionID)
	}

	applyAcceptedLifecycleControl(
		t,
		baseURL,
		petriSession.Id,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	assertLiveSessionLifecycleControlStatus(
		t,
		baseURL,
		petriSession.Id,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)

	applyAcceptedLifecycleControl(
		t,
		baseURL,
		javaSessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	assertDurableSessionStatus(
		t,
		baseURL,
		javaSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)

	applyAcceptedLifecycleControl(
		t,
		baseURL,
		petriSession.Id,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	assertLiveSessionLifecycleControlStatus(
		t,
		baseURL,
		petriSession.Id,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)

	applyAcceptedLifecycleControl(
		t,
		baseURL,
		javaSessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	assertDurableSessionStatus(
		t,
		baseURL,
		javaSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)
}

func scaffoldSessionCompatibilityFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "session-compatibility",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})

	workflowDir := filepath.Join(dir, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	fixturePath := support.AgentFactoryPath(
		t,
		filepath.Join("tests", "fixtures", "javascript_runtime", busyLoopWorkflowName+".workflow.js"),
	)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read workflow fixture %s: %v", fixturePath, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, busyLoopWorkflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return dir
}

func startBusyLoopJavaScriptSession(t *testing.T, baseURL string) string {
	t.Helper()

	workflowName := busyLoopWorkflowName
	started := postJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		baseURL+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-session-compat-busy-loop",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowName,
			},
		},
		"start busy-loop JavaScript Factory Session",
	)
	if started.SessionId == "" {
		t.Fatalf("async JavaScript session id is empty: %#v", started)
	}
	return started.SessionId
}

func applyAcceptedLifecycleControl(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	pathSegment := "pause"
	if operation == factoryapi.FactorySessionLifecycleControlKindResume {
		pathSegment = "resume"
	}
	response := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
		factoryapi.FactorySessionLifecycleControlRequest{},
		"apply "+string(operation)+" to session "+sessionID,
	)
	if response.Operation != operation {
		t.Fatalf(
			"session %s lifecycle control operation = %q, want %q",
			sessionID,
			response.Operation,
			operation,
		)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf(
			"session %s lifecycle control outcome = %q, want ACCEPTED; response=%#v",
			sessionID,
			response.Outcome,
			response,
		)
	}
	if response.SessionId != sessionID {
		t.Fatalf("lifecycle control sessionId = %q, want %q", response.SessionId, sessionID)
	}
	return response
}

func assertLiveSessionOrchestratorKind(
	t *testing.T,
	sessionID string,
	got factoryapi.FactoryOrchestratorKind,
	want factoryapi.FactoryOrchestratorKind,
) {
	t.Helper()
	if got != want {
		t.Fatalf("live session %s orchestratorKind = %q, want %q", sessionID, got, want)
	}
}

func assertDurableSessionOrchestratorKind(
	t *testing.T,
	sessionID string,
	got factoryapi.FactoryOrchestratorKind,
	want factoryapi.FactoryOrchestratorKind,
) {
	t.Helper()
	if got != want {
		t.Fatalf("durable session %s orchestratorKind = %q, want %q", sessionID, got, want)
	}
}

func assertLiveSessionLifecycleControlStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	session := support.GetDefaultSession(t, baseURL)
	if session.Id != sessionID {
		t.Fatalf("default live session id = %q, want %q", session.Id, sessionID)
	}
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatalf("live session %s lifecycleControlStatus = nil, want %q", sessionID, want)
	}
	if *session.Runtime.LifecycleControlStatus != want {
		t.Fatalf(
			"live session %s lifecycleControlStatus = %q, want %q",
			sessionID,
			*session.Runtime.LifecycleControlStatus,
			want,
		)
	}
}

func assertDurableSessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	session := readDurableSession(t, baseURL, sessionID)
	if session.Status != want {
		t.Fatalf("durable session %s status = %q, want %q", sessionID, session.Status, want)
	}
}

func readDurableSession(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session %s: %v", sessionID, err)
	}
	if session.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want %q", session.SessionId, sessionID)
	}
	return session
}

func waitForDurableSessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableSession(t, baseURL, sessionID)
		if session.Status == want {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableSession(t, baseURL, sessionID)
	t.Fatalf("durable session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()

	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	response, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var out T
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}

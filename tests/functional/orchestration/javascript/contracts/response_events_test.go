package contracts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	cursorChildSessionID          = "cursor-js-child-session"
	childProgressWorkflowFileName = "child-progress.workflow.js"
	childProgressWorkflowSource   = `return (async function () {
  const child = await agent.run({
    prompt: "summarize workflows",
    label: "summarize-findings",
    modelProvider: "cursor",
    model: "cursor-test-model",
  });
  return { label: "child-progress", child: child };
})();`
)

// TestJavaScriptChildProgressPublishesCanonicalResponseEvents proves JavaScript
// child dispatches publish message and tool progress as canonical
// FactoryResponseEvent records on the public Factory Session response-event
// surface after a root-built process run.
func TestJavaScriptChildProgressPublishesCanonicalResponseEvents(t *testing.T) {
	dir := scaffoldChildProgressWorkflow(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: cursorChildProgressStream(cursorChildSessionID, "Child summary COMPLETE"),
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})

	started := startChildProgressWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 live child invocation", runner.CallCount())
	}
	responseEvents := support.GetFactoryResponseEventsAt(t, server.URL(), started.SessionId)
	assertJavaScriptChildProgressResponseEvents(t, responseEvents)
}

func scaffoldChildProgressWorkflow(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-child-progress"})
	if err := os.WriteFile(
		filepath.Join(dir, childProgressWorkflowFileName),
		[]byte(childProgressWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write child progress workflow: %v", err)
	}
	return dir
}

func startChildProgressWorkflow(t *testing.T, serverURL, dir string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	workflowPath := filepath.Join(dir, childProgressWorkflowFileName)
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-child-progress-response-events",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal child progress workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build child progress workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start child progress workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start child progress workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode child progress workflow response: %v", err)
	}
	return started
}

func assertJavaScriptChildProgressResponseEvents(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("response events are empty, want child message/tool progress")
	}

	var dispatchID string
	previousSequence := int64(0)
	sawMessage := false
	sawTool := false
	for index, event := range events {
		if event.Sequence <= previousSequence {
			t.Fatalf("response event[%d] sequence = %d follows %d", index, event.Sequence, previousSequence)
		}
		previousSequence = event.Sequence
		if event.DispatchId == nil || strings.TrimSpace(*event.DispatchId) == "" {
			t.Fatalf("response event[%d] = %#v, want dispatch correlation", index, event)
		}
		if dispatchID == "" {
			dispatchID = *event.DispatchId
		}
		if *event.DispatchId != dispatchID {
			t.Fatalf("response event[%d] dispatch = %q, want %q", index, *event.DispatchId, dispatchID)
		}
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			sawMessage = true
		case factoryapi.FactoryResponseEventKindTool:
			sawTool = true
		}
	}
	if !sawMessage {
		t.Fatalf("response events = %#v, want at least one MESSAGE progress event", events)
	}
	if !sawTool {
		t.Fatalf("response events = %#v, want at least one TOOL progress event", events)
	}
}

func cursorChildProgressStream(sessionID, result string) []byte {
	records := []string{
		`{"type":"system","subtype":"init","session_id":"` + sessionID + `"}`,
		`{"type":"assistant","timestamp_ms":1,"message":{"role":"assistant","content":[{"type":"text","text":"working"}]},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-1","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-1","tool_call":{"readToolCall":{"result":{"success":{}}}},"session_id":"` + sessionID + `"}`,
		string(cursorChildTerminalRecord(sessionID, result)),
	}
	return []byte(strings.Join(records, "\n"))
}

func cursorChildTerminalRecord(sessionID, result string) []byte {
	return []byte(
		`{"type":"result","subtype":"success","is_error":false,"result":` +
			mustJSONString(result) + `,"session_id":` + mustJSONString(sessionID) + `}`,
	)
}

func mustJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

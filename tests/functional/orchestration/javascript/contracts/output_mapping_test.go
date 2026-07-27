package contracts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	returnValueWorkflowFileName = "return-value.workflow.js"
	returnValueWorkflowSource   = `return "` + returnValuePrimaryResult + `";`
	returnValuePrimaryResult    = "js-output-mapping-primary-result"
)

var privateJavaScriptVMDiagnosticMarkers = []string{
	"goja",
	"goja.",
	"stack frame",
	"heap dump",
}

// TestJavaScriptReturnValueMapsToPrimaryInvocationResult proves a JavaScript
// Factory script return value maps onto the customer-visible primary invocation
// result on public Factory Session projection and Factory Event surfaces after
// a root-built process run.
func TestJavaScriptReturnValueMapsToPrimaryInvocationResult(t *testing.T) {
	t.Parallel()

	dir := scaffoldReturnValueMappingWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startReturnValueMappingWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for return-value echo workflow", runner.CallCount())
	}

	assertReturnValuePrimaryResult(t, started.Result, returnValuePrimaryResult)

	finalResult := readReturnValueFinalSessionResult(t, server.URL(), started.SessionId)
	assertReturnValuePrimaryResult(t, &finalResult, returnValuePrimaryResult)

	session := readReturnValueMappingSession(t, server.URL(), started.SessionId)
	if session.ResultSummary == nil ||
		session.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("session resultSummary = %#v, want FINAL durable projection", session.ResultSummary)
	}

	events := getFactoryEventsForSessionAt(t, server.URL(), started.SessionId)
	assertReturnValueMappingFactoryEvents(t, events, returnValuePrimaryResult)
	assertNoPrivateJavaScriptVMDiagnostics(
		t,
		marshalPrimaryResultForDiagnostics(t, started.Result),
		marshalFactoryEventsForDiagnostics(t, events),
	)
}

func scaffoldReturnValueMappingWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-output-mapping-return-value",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": returnValueWorkflowFileName,
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, returnValueWorkflowFileName),
		[]byte(returnValueWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write return value workflow: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "mock-workers.json"),
		[]byte(`{"mockWorkers":[]}`),
		0o600,
	); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return dir
}

func startReturnValueMappingWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, returnValueWorkflowFileName)
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-return-value-output-mapping",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal return value workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build return value workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start return value workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start return value workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode return value workflow response: %v", err)
	}
	return started
}

func readReturnValueFinalSessionResult(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionResult {
	t.Helper()

	return support.GetJSON[factoryapi.FactorySessionResult](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/results?mode=final",
	)
}

func readReturnValueMappingSession(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	return support.GetJSON[factoryapi.FactorySessionDurableReadModel](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
}

func assertReturnValuePrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	want string,
) {
	t.Helper()

	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL Factory Session result", result)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one Work content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result Work content part: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != want {
		t.Fatalf("primary result = %#v, want exact string %q", part.Json, want)
	}
}

func assertReturnValueMappingFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want string,
) {
	t.Helper()

	if len(events) == 0 {
		t.Fatal("factory events = empty, want at least one public Factory Event")
	}

	sawResultUpdated := false
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionResultUpdated:
			sawResultUpdated = true
			if !factoryEventReferencesReturnValue(t, event, want) {
				t.Fatalf("SESSION_RESULT_UPDATED event = %#v, want public result evidence for %q", event, want)
			}
		}
	}
	if !sawResultUpdated {
		t.Fatalf("factory events = %#v, want SESSION_RESULT_UPDATED with mapped return value", events)
	}
}

func factoryEventReferencesReturnValue(
	t *testing.T,
	event factoryapi.FactoryEvent,
	want string,
) bool {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal factory event: %v", err)
	}
	return strings.Contains(string(encoded), want)
}

func marshalPrimaryResultForDiagnostics(t *testing.T, result *factoryapi.FactorySessionResult) string {
	t.Helper()

	if result == nil || result.PrimaryResult == nil {
		return ""
	}
	encoded, err := json.Marshal(result.PrimaryResult)
	if err != nil {
		t.Fatalf("marshal primary result for diagnostics: %v", err)
	}
	return string(encoded)
}

func marshalFactoryEventsForDiagnostics(t *testing.T, events []factoryapi.FactoryEvent) string {
	t.Helper()

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events for diagnostics: %v", err)
	}
	return string(encoded)
}

func assertNoPrivateJavaScriptVMDiagnostics(t *testing.T, outputs ...string) {
	t.Helper()

	combined := strings.ToLower(strings.Join(outputs, "\n"))
	for _, marker := range privateJavaScriptVMDiagnosticMarkers {
		if strings.Contains(combined, strings.ToLower(marker)) {
			t.Fatalf("diagnostics exposed private VM detail %q in %q", marker, strings.Join(outputs, "\n---\n"))
		}
	}
}

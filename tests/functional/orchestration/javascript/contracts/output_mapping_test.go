package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	structuredArtifactWorkflowFileName = "structured-artifact.workflow.js"
	structuredArtifactWorkflowSource   = `const artifactRef = workflow.artifact({
  kind: "log",
  label: "` + structuredArtifactLabel + `",
  content: { message: "` + structuredArtifactContentValue + `" },
});
return { artifactRef: artifactRef };`
	structuredArtifactID            = "artifact-1"
	structuredArtifactLabel         = "js-output-mapping-artifact-label"
	structuredArtifactContentValue  = "js-output-mapping-artifact-content"

	unsupportedReturnWorkflowFileName = "unsupported-return.workflow.js"
	unsupportedReturnWorkflowSource   = `return function () {};`

	unsupportedReturnValidationCode       = "workflow.result.unsupportedType"
	unsupportedReturnValidationDiagnostic = "workflow result cannot include a function value"
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

// TestJavaScriptStructuredArtifactsMapToPublicResult proves structured artifacts
// produced by a JavaScript Factory invocation appear on the customer-visible public
// result, artifact list, and Factory Event surfaces after a root-built process run.
func TestJavaScriptStructuredArtifactsMapToPublicResult(t *testing.T) {
	t.Parallel()

	dir := scaffoldStructuredArtifactWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startStructuredArtifactWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for artifact workflow", runner.CallCount())
	}

	finalResult := readStructuredArtifactFinalSessionResult(t, server.URL(), started.SessionId)
	assertStructuredArtifactResultSurface(t, finalResult, started.SessionId)

	session := readStructuredArtifactSession(t, server.URL(), started.SessionId)
	assertStructuredArtifactSessionProjection(t, session)

	artifactList := readStructuredArtifactList(t, server.URL(), started.SessionId)
	artifactSummary := assertStructuredArtifactListSummary(t, artifactList, started.SessionId)

	artifactDetail := readStructuredArtifactDetail(t, server.URL(), started.SessionId, structuredArtifactID)
	assertStructuredArtifactDetailSurface(t, artifactDetail, artifactSummary)

	events := getFactoryEventsForSessionAt(t, server.URL(), started.SessionId)
	assertStructuredArtifactFactoryEvents(t, events)
	assertNoPrivateJavaScriptVMDiagnostics(
		t,
		marshalPrimaryResultForDiagnostics(t, started.Result),
		marshalArtifactListForDiagnostics(t, artifactList),
		marshalFactoryEventsForDiagnostics(t, events),
	)
}

// TestJavaScriptUnsupportedReturnValueFailsWithoutPrivateVMDetails proves an
// unsupported JavaScript return value yields a non-success customer-visible
// outcome with an actionable diagnostic and without private VM internals on
// public Factory Session projection and Factory Event surfaces.
func TestJavaScriptUnsupportedReturnValueFailsWithoutPrivateVMDetails(t *testing.T) {
	t.Parallel()

	dir := scaffoldUnsupportedReturnWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startUnsupportedReturnWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for unsupported return workflow", runner.CallCount())
	}

	assertUnsupportedReturnFailureResult(t, started.Result)

	session := readUnsupportedReturnSession(t, server.URL(), started.SessionId)
	assertUnsupportedReturnSessionFailure(t, session)

	events := getFactoryEventsForSessionAt(t, server.URL(), started.SessionId)
	assertUnsupportedReturnFactoryEvents(t, events)
	assertNoPrivateJavaScriptVMDiagnostics(
		t,
		marshalFailureDetailForDiagnostics(t, started.Result.FailureDetail),
		marshalFailureDetailForDiagnostics(t, session.FailureDetail),
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

func scaffoldStructuredArtifactWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-output-mapping-structured-artifact",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": structuredArtifactWorkflowFileName,
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, structuredArtifactWorkflowFileName),
		[]byte(structuredArtifactWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write structured artifact workflow: %v", err)
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

func scaffoldUnsupportedReturnWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-output-mapping-unsupported-return",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": unsupportedReturnWorkflowFileName,
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, unsupportedReturnWorkflowFileName),
		[]byte(unsupportedReturnWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write unsupported return workflow: %v", err)
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

func startStructuredArtifactWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, structuredArtifactWorkflowFileName)
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-structured-artifact-output-mapping",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal structured artifact workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build structured artifact workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start structured artifact workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start structured artifact workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode structured artifact workflow response: %v", err)
	}
	return started
}

func startUnsupportedReturnWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, unsupportedReturnWorkflowFileName)
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-unsupported-return-output-mapping",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal unsupported return workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build unsupported return workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start unsupported return workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start unsupported return workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode unsupported return workflow response: %v", err)
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

func readStructuredArtifactFinalSessionResult(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionResult {
	t.Helper()

	return support.GetJSON[factoryapi.FactorySessionResult](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/results?mode=final",
	)
}

func readStructuredArtifactSession(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	return support.GetJSON[factoryapi.FactorySessionDurableReadModel](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
}

func readStructuredArtifactList(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.ListFactorySessionArtifactsResponse {
	t.Helper()

	return support.GetJSON[factoryapi.ListFactorySessionArtifactsResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/artifacts",
	)
}

func readStructuredArtifactDetail(
	t *testing.T,
	serverURL, sessionID, artifactID string,
) factoryapi.FactorySessionArtifactDetail {
	t.Helper()

	return support.GetJSON[factoryapi.FactorySessionArtifactDetail](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/artifacts/"+artifactID,
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

func readUnsupportedReturnSession(
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

func assertStructuredArtifactResultSurface(
	t *testing.T,
	result factoryapi.FactorySessionResult,
	sessionID string,
) {
	t.Helper()

	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", result.ResultStatus)
	}
	if result.ArtifactIds == nil || len(*result.ArtifactIds) != 1 || (*result.ArtifactIds)[0] != structuredArtifactID {
		t.Fatalf("artifactIds = %#v, want [%q]", result.ArtifactIds, structuredArtifactID)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one Work content part with artifactRef", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result Work content part: %v", err)
	}
	payload, ok := part.Json.(map[string]any)
	if !ok {
		t.Fatalf("primary result = %#v, want JSON object with artifactRef", part.Json)
	}
	wantArtifactRef := fmt.Sprintf("you-artifact://sessions/%s/artifacts/%s", sessionID, structuredArtifactID)
	if got, _ := payload["artifactRef"].(string); got != wantArtifactRef {
		t.Fatalf("primary result artifactRef = %#v, want %q", payload["artifactRef"], wantArtifactRef)
	}
}

func assertStructuredArtifactSessionProjection(
	t *testing.T,
	session factoryapi.FactorySessionDurableReadModel,
) {
	t.Helper()

	if session.ResultSummary == nil ||
		session.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("session resultSummary = %#v, want FINAL durable projection", session.ResultSummary)
	}
	if session.ArtifactRefs == nil || len(*session.ArtifactRefs) != 1 ||
		(*session.ArtifactRefs)[0].Id != structuredArtifactID {
		t.Fatalf("artifactRefs = %#v, want one ref for %q", session.ArtifactRefs, structuredArtifactID)
	}
}

func assertStructuredArtifactListSummary(
	t *testing.T,
	artifactList factoryapi.ListFactorySessionArtifactsResponse,
	sessionID string,
) factoryapi.FactorySessionArtifactSummary {
	t.Helper()

	if len(artifactList.Artifacts) != 1 {
		t.Fatalf("artifact list = %#v, want exactly one artifact", artifactList.Artifacts)
	}
	artifact := artifactList.Artifacts[0]
	if artifact.Id != structuredArtifactID {
		t.Fatalf("artifact id = %q, want %q", artifact.Id, structuredArtifactID)
	}
	if artifact.Label == nil || *artifact.Label != structuredArtifactLabel {
		t.Fatalf("artifact label = %#v, want %q", artifact.Label, structuredArtifactLabel)
	}
	if artifact.Kind != factoryapi.FactoryArtifactKind("log") {
		t.Fatalf("artifact kind = %q, want log", artifact.Kind)
	}
	if artifact.ContentHash == nil || *artifact.ContentHash == "" {
		t.Fatalf("artifact contentHash = %#v, want non-empty structured payload hash", artifact.ContentHash)
	}
	if artifact.SizeBytes == nil || *artifact.SizeBytes <= 0 {
		t.Fatalf("artifact sizeBytes = %#v, want positive structured payload size", artifact.SizeBytes)
	}
	wantHref := "/factory-sessions/" + sessionID + "/artifacts/" + structuredArtifactID
	if artifact.RetrievalRef == nil || artifact.RetrievalRef.Href != wantHref {
		t.Fatalf("artifact retrievalRef = %#v, want href %q", artifact.RetrievalRef, wantHref)
	}
	return artifact
}

func assertStructuredArtifactDetailSurface(
	t *testing.T,
	detail factoryapi.FactorySessionArtifactDetail,
	summary factoryapi.FactorySessionArtifactSummary,
) {
	t.Helper()

	if detail.Id != summary.Id {
		t.Fatalf("artifact detail id = %q, want summary id %q", detail.Id, summary.Id)
	}
	if detail.Label == nil || *detail.Label != structuredArtifactLabel {
		t.Fatalf("artifact detail label = %#v, want %q", detail.Label, structuredArtifactLabel)
	}
	if detail.ContentHash == nil || summary.ContentHash == nil || *detail.ContentHash != *summary.ContentHash {
		t.Fatalf("artifact detail contentHash = %#v, want summary hash %v", detail.ContentHash, summary.ContentHash)
	}
}

func assertStructuredArtifactFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()

	sawResultUpdated := false
	sawSessionCompleted := false
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionResultUpdated:
			payload, err := event.Payload.AsSessionResultUpdatedEventPayload()
			if err != nil {
				t.Fatalf("decode SESSION_RESULT_UPDATED payload: %v", err)
			}
			if payload.ArtifactIds == nil || len(*payload.ArtifactIds) != 1 ||
				(*payload.ArtifactIds)[0] != structuredArtifactID {
				t.Fatalf("SESSION_RESULT_UPDATED artifactIds = %#v, want [%q]", payload.ArtifactIds, structuredArtifactID)
			}
			sawResultUpdated = true
		case factoryapi.FactoryEventTypeSessionCompleted:
			payload, err := event.Payload.AsSessionCompletedEventPayload()
			if err != nil {
				t.Fatalf("decode SESSION_COMPLETED payload: %v", err)
			}
			if payload.ArtifactIds == nil || len(*payload.ArtifactIds) != 1 ||
				(*payload.ArtifactIds)[0] != structuredArtifactID {
				t.Fatalf("SESSION_COMPLETED artifactIds = %#v, want [%q]", payload.ArtifactIds, structuredArtifactID)
			}
			sawSessionCompleted = true
		}
	}
	if !sawResultUpdated {
		t.Fatalf("factory events = %#v, want SESSION_RESULT_UPDATED with artifactIds", events)
	}
	if !sawSessionCompleted {
		t.Fatalf("factory events = %#v, want SESSION_COMPLETED with artifactIds", events)
	}
}

func assertUnsupportedReturnFailureResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
) {
	t.Helper()

	if result == nil {
		t.Fatal("result = nil, want failed Factory Session result")
	}
	if result.SessionStatus == nil ||
		*result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("result sessionStatus = %#v, want FAILED", result.SessionStatus)
	}
	if result.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("result status = %q, want UNAVAILABLE on unsupported return", result.ResultStatus)
	}
	if result.PrimaryResult != nil {
		t.Fatalf("primary result = %#v, want nil on unsupported return", result.PrimaryResult)
	}
	if result.FailureDetail != nil {
		assertUnsupportedReturnFailureDetail(t, result.FailureDetail)
	}
}

func assertUnsupportedReturnSessionFailure(
	t *testing.T,
	session factoryapi.FactorySessionDurableReadModel,
) {
	t.Helper()

	if session.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", session.Status)
	}
	assertUnsupportedReturnFailureDetail(t, session.FailureDetail)
}

func assertUnsupportedReturnFailureDetail(
	t *testing.T,
	failureDetail *factoryapi.FailureDetail,
) {
	t.Helper()

	if failureDetail == nil || strings.TrimSpace(failureDetail.Message) == "" {
		t.Fatalf("failureDetail = %#v, want actionable public failure record", failureDetail)
	}
	message := failureDetail.Message
	if !strings.Contains(message, unsupportedReturnValidationCode) {
		t.Fatalf("failure message = %#v, want validation code %q", message, unsupportedReturnValidationCode)
	}
	if !strings.Contains(message, unsupportedReturnValidationDiagnostic) {
		t.Fatalf(
			"failure message = %#v, want unsupported return diagnostic %q",
			message,
			unsupportedReturnValidationDiagnostic,
		)
	}
}

func assertUnsupportedReturnFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()

	if len(events) == 0 {
		t.Fatal("factory events = empty, want at least one public Factory Event")
	}

	sawResultUpdated := false
	sawSessionCompleted := false
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionResultUpdated:
			payload, err := event.Payload.AsSessionResultUpdatedEventPayload()
			if err != nil {
				t.Fatalf("decode SESSION_RESULT_UPDATED payload: %v", err)
			}
			if payload.ResultStatus != factoryapi.FactoryEventSessionResultStatusUnavailable {
				t.Fatalf("SESSION_RESULT_UPDATED resultStatus = %q, want UNAVAILABLE", payload.ResultStatus)
			}
			sawResultUpdated = true
		case factoryapi.FactoryEventTypeSessionCompleted:
			payload, err := event.Payload.AsSessionCompletedEventPayload()
			if err != nil {
				t.Fatalf("decode SESSION_COMPLETED payload: %v", err)
			}
			if payload.FinalStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
				t.Fatalf("SESSION_COMPLETED finalStatus = %q, want FAILED", payload.FinalStatus)
			}
			if payload.ResultStatus == nil ||
				*payload.ResultStatus != factoryapi.FactoryEventSessionResultStatusUnavailable {
				t.Fatalf("SESSION_COMPLETED resultStatus = %#v, want UNAVAILABLE", payload.ResultStatus)
			}
			sawSessionCompleted = true
		}
	}
	if !sawResultUpdated {
		t.Fatalf("factory events = %#v, want SESSION_RESULT_UPDATED with UNAVAILABLE result", events)
	}
	if !sawSessionCompleted {
		t.Fatalf("factory events = %#v, want SESSION_COMPLETED with FAILED final status", events)
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

func marshalFailureDetailForDiagnostics(t *testing.T, failureDetail *factoryapi.FailureDetail) string {
	t.Helper()

	if failureDetail == nil {
		return ""
	}
	encoded, err := json.Marshal(failureDetail)
	if err != nil {
		t.Fatalf("marshal failure detail for diagnostics: %v", err)
	}
	return string(encoded)
}

func marshalArtifactListForDiagnostics(
	t *testing.T,
	artifactList factoryapi.ListFactorySessionArtifactsResponse,
) string {
	t.Helper()

	encoded, err := json.Marshal(artifactList)
	if err != nil {
		t.Fatalf("marshal artifact list for diagnostics: %v", err)
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

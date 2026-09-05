package factory_events

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const fscp01CanonicalArtifactWorkflow = `return (async function () {
  const artifactRef = workflow.artifact({
    kind: "log",
    label: "fscp01-canonical-artifact",
    content: { message: "canonical reads survive response-event absence" },
  });
  return { artifactRef };
})();`

// TestFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents
// proves the public canonical Factory Event and artifact reads stand alone from
// the ephemeral Factory Response Event stream. The test intentionally never
// subscribes to response-events: all observations come from the canonical
// session-scoped reads after a completed durable execution.
func TestFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "fscp01-canonical-reads",
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startFSCP01CanonicalExecution(t, server.URL())
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("durable session status = %q, want SUCCEEDED", started.Status)
	}
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("durable session id is empty")
	}

	fullRead, cursorEvent := assertFSCP01CanonicalEventReads(t, server.URL(), started.SessionId)
	artifactCount := assertFSCP01CanonicalArtifactReads(t, server.URL(), started.SessionId, dir)
	unknownSessionRecovery := probeFSCP01CanonicalRecovery(t, support.SessionEventsURL(server.URL(), "fscp01-foreign-session"))
	if unknownSessionRecovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcomeUNKNOWNSESSION {
		t.Fatalf("unknown session recovery outcome = %q, want UNKNOWN_SESSION", unknownSessionRecovery.Outcome)
	}

	finalRead := support.GetFactoryEventsForSessionAt(t, server.URL(), started.SessionId)
	assertFactoryEventsSameRelativeOrder(t, fullRead, finalRead)
	t.Logf("FSCP-01 canonical evidence: session=%s events=%d cursor=%s artifacts=%d", started.SessionId, len(fullRead), cursorEvent.Id, artifactCount)
}

func assertFSCP01CanonicalEventReads(
	t *testing.T,
	baseURL string,
	sessionID string,
) ([]factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()
	fullRead := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	if len(fullRead) < 3 {
		t.Fatalf("canonical event count = %d, want at least session start/result/completed", len(fullRead))
	}
	assertFSCP01CanonicalEventIdentityAndTerminalBoundary(t, fullRead, sessionID)
	secondRead := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	assertFactoryEventsSameRelativeOrder(t, fullRead, secondRead)

	cursorIndex, cursorEvent := pickSessionScopedCursorEvent(t, fullRead, sessionID)
	wantAfter := append([]factoryapi.FactoryEvent(nil), fullRead[cursorIndex+1:]...)
	afterEventIDRead := support.GetFactoryEventsAfterForSessionAt(t, baseURL, sessionID, support.FactoryEventReadCursor{
		AfterEventID: cursorEvent.Id,
	})
	assertFactoryEventsCursorAfterResult(t, cursorEvent, wantAfter, afterEventIDRead)

	afterSequence := support.ReconnectSequenceForFactoryEvent(cursorEvent)
	afterSequenceRead := support.GetFactoryEventsAfterForSessionAt(t, baseURL, sessionID, support.FactoryEventReadCursor{
		AfterSequence: &afterSequence,
	})
	assertFactoryEventsCursorAfterResult(t, cursorEvent, wantAfter, afterSequenceRead)

	unknownCursor := readFSCP01CanonicalError(t, support.SessionEventsURLWithCursor(
		baseURL,
		sessionID,
		support.FactoryEventReadCursor{AfterEventID: "fscp01-unknown-canonical-event"},
	))
	if unknownCursor.Status != http.StatusBadRequest || unknownCursor.Response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("unknown canonical cursor error = %#v, want 400 BAD_REQUEST", unknownCursor)
	}
	if strings.Contains(unknownCursor.ContentType, "text/event-stream") || strings.Contains(unknownCursor.Body, "data: ") {
		t.Fatalf("unknown canonical cursor returned an event stream: %#v", unknownCursor)
	}
	return fullRead, cursorEvent
}

func assertFSCP01CanonicalArtifactReads(t *testing.T, baseURL, sessionID, dir string) int {
	t.Helper()
	artifactListEndpoint := fscp01SessionArtifactsEndpoint(baseURL, sessionID)
	artifactList := support.GetJSON[factoryapi.ListFactorySessionArtifactsResponse](t, artifactListEndpoint)
	if artifactList.SessionId != sessionID {
		t.Fatalf("artifact list sessionId = %q, want %q", artifactList.SessionId, sessionID)
	}
	if len(artifactList.Artifacts) == 0 {
		t.Fatal("artifact list is empty, want the completed workflow artifact")
	}
	firstListPayload, err := json.Marshal(artifactList)
	if err != nil {
		t.Fatalf("marshal first artifact list: %v", err)
	}
	secondArtifactList := support.GetJSON[factoryapi.ListFactorySessionArtifactsResponse](t, artifactListEndpoint)
	secondListPayload, err := json.Marshal(secondArtifactList)
	if err != nil {
		t.Fatalf("marshal second artifact list: %v", err)
	}
	if !bytes.Equal(firstListPayload, secondListPayload) {
		t.Fatalf("repeated artifact list changed: first=%s second=%s", firstListPayload, secondListPayload)
	}

	artifact := artifactList.Artifacts[0]
	if artifact.RetrievalRef == nil || strings.TrimSpace(artifact.RetrievalRef.Href) == "" {
		t.Fatalf("artifact summary retrievalRef = %#v, want a safe API reference", artifact.RetrievalRef)
	}
	assertFSCP01SafeArtifactHref(t, artifact.RetrievalRef.Href, dir)

	artifactEndpoint := artifactListEndpoint + "/" + artifact.Id
	detail := support.GetJSON[factoryapi.FactorySessionArtifactDetail](t, artifactEndpoint)
	if detail.SessionId != sessionID || detail.Id != artifact.Id {
		t.Fatalf("artifact detail identity = session:%q id:%q, want session:%q id:%q", detail.SessionId, detail.Id, sessionID, artifact.Id)
	}
	if detail.ContentHash == nil || strings.TrimSpace(*detail.ContentHash) == "" {
		t.Fatalf("artifact detail contentHash = %#v, want a stable content hash", detail.ContentHash)
	}
	if detail.Content == nil && detail.ContentRef == nil {
		t.Fatalf("artifact detail = %#v, want inline content or safe contentRef", detail)
	}
	if detail.ContentRef != nil {
		assertFSCP01SafeArtifactHref(t, detail.ContentRef.Href, dir)
	}

	repeatedDetail := support.GetJSON[factoryapi.FactorySessionArtifactDetail](t, artifactEndpoint)
	firstDetailPayload, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal first artifact detail: %v", err)
	}
	repeatedDetailPayload, err := json.Marshal(repeatedDetail)
	if err != nil {
		t.Fatalf("marshal repeated artifact detail: %v", err)
	}
	if !bytes.Equal(firstDetailPayload, repeatedDetailPayload) {
		t.Fatalf("repeated artifact detail changed: first=%s second=%s", firstDetailPayload, repeatedDetailPayload)
	}

	missingArtifact := readFSCP01CanonicalError(t, artifactListEndpoint+"/fscp01-missing-artifact")
	assertFSCP01NotFoundArtifact(t, missingArtifact, "missing artifact")
	corruptArtifact := readFSCP01CanonicalError(t, artifactListEndpoint+"/fscp01-corrupt-artifact-id")
	assertFSCP01NotFoundArtifact(t, corruptArtifact, "corrupt artifact id")
	foreignArtifact := readFSCP01CanonicalError(t, fscp01SessionArtifactsEndpoint(baseURL, "fscp01-foreign-session")+"/"+artifact.Id)
	assertFSCP01NotFoundArtifact(t, foreignArtifact, "foreign session artifact")
	return len(artifactList.Artifacts)
}

func startFSCP01CanonicalExecution(t *testing.T, serverURL string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "fscp01-canonical-reads-sync",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   fscp01CanonicalArtifactWorkflow,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal canonical execution request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build canonical execution request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode canonical execution response: %v", err)
	}
	return started
}

func assertFSCP01CanonicalEventIdentityAndTerminalBoundary(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
) {
	t.Helper()
	assertFactoryEventsAscendingOrder(t, events)
	seenIDs := make(map[string]struct{}, len(events))
	seenSequences := make(map[int]struct{}, len(events))
	terminalIndex := -1
	for index, event := range events {
		if event.Id == "" {
			t.Fatalf("canonical event[%d] has empty id", index)
		}
		if _, duplicate := seenIDs[event.Id]; duplicate {
			t.Fatalf("canonical event id %q was replayed", event.Id)
		}
		seenIDs[event.Id] = struct{}{}
		if _, duplicate := seenSequences[event.Context.Sequence]; duplicate {
			t.Fatalf("canonical sequence %d was replayed", event.Context.Sequence)
		}
		seenSequences[event.Context.Sequence] = struct{}{}
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("canonical event %q sessionId = %q, want %q", event.Id, *event.Context.SessionId, sessionID)
		}
		if event.Type == factoryapi.FactoryEventTypeSessionCompleted {
			if terminalIndex != -1 {
				t.Fatalf("canonical history has duplicate SESSION_COMPLETED at indexes %d and %d", terminalIndex, index)
			}
			terminalIndex = index
		}
	}
	if terminalIndex != len(events)-1 {
		t.Fatalf("canonical terminal boundary index = %d, want final event index %d", terminalIndex, len(events)-1)
	}
}

type fscp01CanonicalHTTPError struct {
	Status      int
	ContentType string
	Body        string
	Response    factoryapi.ErrorResponse
}

func readFSCP01CanonicalError(t *testing.T, endpoint string) fscp01CanonicalHTTPError {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build canonical negative read request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s error: %v", endpoint, err)
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		t.Fatalf("GET %s status = %d, want typed failure: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var typed factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatalf("decode typed failure from GET %s: %v: %s", endpoint, err, strings.TrimSpace(string(body)))
	}
	if strings.TrimSpace(string(typed.Code)) == "" || strings.TrimSpace(string(typed.Family)) == "" {
		t.Fatalf("GET %s returned incomplete typed failure: %#v", endpoint, typed)
	}
	return fscp01CanonicalHTTPError{
		Status:      response.StatusCode,
		ContentType: response.Header.Get("Content-Type"),
		Body:        string(body),
		Response:    typed,
	}
}

func assertFSCP01NotFoundArtifact(t *testing.T, got fscp01CanonicalHTTPError, label string) {
	t.Helper()
	if got.Status != http.StatusNotFound || got.Response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("%s error = %#v, want 404 NOT_FOUND", label, got)
	}
}

func probeFSCP01CanonicalRecovery(t *testing.T, endpoint string) factoryapi.FactorySessionEventStreamRecovery {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build canonical recovery probe: %v", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s recovery probe: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s recovery status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var recovery factoryapi.FactorySessionEventStreamRecovery
	if err := json.NewDecoder(response.Body).Decode(&recovery); err != nil {
		t.Fatalf("decode canonical recovery probe: %v", err)
	}
	return recovery
}

func fscp01SessionArtifactsEndpoint(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/artifacts"
}

func assertFSCP01SafeArtifactHref(t *testing.T, href, factoryDir string) {
	t.Helper()
	if strings.TrimSpace(href) == "" || strings.HasPrefix(strings.ToLower(href), "file:") {
		t.Fatalf("artifact href = %q, want non-empty API-relative reference", href)
	}
	if strings.Contains(href, factoryDir) || strings.Contains(href, "\\") {
		t.Fatalf("artifact href = %q exposes a host filesystem path", href)
	}
}

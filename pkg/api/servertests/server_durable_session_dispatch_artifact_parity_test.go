package apiserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestDispatchArtifactReads_PreserveSessionParityWithoutMutation(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-parity-001"))
	waitForRuntimeSessionTerminal(t, service, started.SessionId)

	beforeList := getFactorySessionList(t, server.URL, "persisted")
	beforeRead := getDurableFactorySession(t, server.URL, started.SessionId)
	beforeResult := getDurableFactorySessionResult(t, server.URL, started.SessionId, "")
	beforeEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionId, "")

	assertDispatchArtifactReadsDoNotCreateSessions(t, server.URL, started.SessionId)

	afterList := getFactorySessionList(t, server.URL, "persisted")
	if len(*afterList.DurableSessions) != len(*beforeList.DurableSessions) {
		t.Fatalf("persisted durable session count = %d, want %d after dispatch/artifact reads",
			len(*afterList.DurableSessions), len(*beforeList.DurableSessions))
	}

	afterRead := getDurableFactorySession(t, server.URL, started.SessionId)
	assertDurableSessionReadUnchanged(t, beforeRead, afterRead)

	afterResult := getDurableFactorySessionResult(t, server.URL, started.SessionId, "")
	assertDurableSessionResultUnchanged(t, beforeResult, afterResult)

	afterEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionId, "")
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("event count = %d, want %d after dispatch/artifact reads", len(afterEvents), len(beforeEvents))
	}
}

func assertDispatchArtifactReadsDoNotCreateSessions(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	paths := []string{
		"/factory-sessions/" + sessionID + "/dispatches",
		"/factory-sessions/" + sessionID + "/dispatches/dispatch-missing-001",
		"/factory-sessions/" + sessionID + "/artifacts",
		"/factory-sessions/" + sessionID + "/artifacts/artifact-missing-001",
	}
	for _, path := range paths {
		resp, err := http.Get(serverURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 200 or 404: %s", path, resp.StatusCode, readBody(t, resp))
		}
	}
}

func assertDurableSessionReadUnchanged(
	t *testing.T,
	before, after factoryapi.FactorySessionDurableReadModel,
) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal before session read: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("marshal after session read: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("session read changed: before=%s after=%s", beforeJSON, afterJSON)
	}
}

func assertDurableSessionResultUnchanged(
	t *testing.T,
	before, after factoryapi.FactorySessionResult,
) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal before result: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("marshal after result: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("result read changed: before=%s after=%s", beforeJSON, afterJSON)
	}
}

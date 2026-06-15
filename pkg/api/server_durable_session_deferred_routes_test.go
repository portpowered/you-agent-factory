package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestDeferredFactorySessionRoutes_RemainNotImplementedForRuntimeBackedSession(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-deferred-001"))
	waitForRuntimeSessionTerminal(t, service, started.SessionId)
	sessionPath := "/factory-sessions/" + started.SessionId

	deferredGET := []string{
		sessionPath + "/dispatches",
		sessionPath + "/dispatches/dispatch-001",
		sessionPath + "/artifacts",
		sessionPath + "/artifacts/artifact-001",
	}
	for _, path := range deferredGET {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Fatalf("GET %s status = %d, want 501: %s", path, resp.StatusCode, readBody(t, resp))
		}
	}

	deferredPOST := []string{
		sessionPath + "/approve",
		sessionPath + "/pause",
		sessionPath + "/resume",
		sessionPath + "/cancel",
		sessionPath + "/terminate",
		sessionPath + "/retry-dispatch",
	}
	for _, path := range deferredPOST {
		resp, err := http.Post(server.URL+path, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Fatalf("POST %s status = %d, want 501: %s", path, resp.StatusCode, readBody(t, resp))
		}
	}
}

package apiserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestDeferredFactorySessionRoutes_LifecycleControlsRemainNotImplemented(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-deferred-001"))
	waitForRuntimeSessionTerminal(t, service, started.SessionId)
	sessionPath := "/factory-sessions/" + started.SessionId

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

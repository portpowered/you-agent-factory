package apiserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestRealBackendFactorySessionRoutes_LifecycleControlsAreImplemented(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-lifecycle-slice-001"))
	sessionPath := "/factory-sessions/" + started.SessionId

	resp, err := http.Post(server.URL+sessionPath+"/cancel", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST %s/cancel: %v", sessionPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST %s/cancel status = %d, want 202: %s", sessionPath, resp.StatusCode, readBody(t, resp))
	}
	var control factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&control); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if control.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("operation = %q, want CANCEL", control.Operation)
	}
	if control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", control.Outcome)
	}
}

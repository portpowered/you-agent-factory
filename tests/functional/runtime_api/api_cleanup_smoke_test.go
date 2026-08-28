package runtime_api

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	server := startSharedFunctionalServer(t, dir, runtimeAPIScenario{
		providerRunner: support.NewStaticSuccessCommandRunner("Done. COMPLETE"),
		models:         []string{"gpt-5-codex"},
	})

	traceID := submitGeneratedWorkAt(t, server.workURL("/work"), factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "cleanup smoke"},
	})
	work := waitForGeneratedWorkAtEndpoint(t, server.workURL("/work"), traceID, "task:complete", 10*time.Second)
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	completed := work.Results[0]
	if support.StringPointerValue(completed.TraceId) != traceID {
		t.Fatalf("GET /work trace_id = %q, want %q", support.StringPointerValue(completed.TraceId), traceID)
	}
	if generatedWorkStateName(completed.State) != "complete" || generatedWorkStateType(completed.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want complete/TERMINAL", completed.State)
	}

	statusRead := getGeneratedJSON[factoryapi.StatusResponse](t, server.statusURL())
	if statusRead.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", statusRead.TotalTokens)
	}
	if statusRead.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", statusRead.Categories.Terminal)
	}
	assertCleanupSmokeCanonicalFactoryEvents(t, server, support.StringPointerValue(completed.WorkId))
	assertGeneratedEventsStreamHasCanonicalHistoryForServer(t, server)
	assertCleanupSmokeDashboardShell(t, server.URL())
}

func assertCleanupSmokeCanonicalFactoryEvents(t *testing.T, server *functionalAPIServer, workID string) {
	t.Helper()

	events := server.GetFactoryEvents(t)
	assertCleanupSmokeHasEventType(t, events, factoryapi.FactoryEventTypeWorkRequest)
	assertCleanupSmokeHasEventType(t, events, factoryapi.FactoryEventTypeDispatchRequest)
	assertCleanupSmokeHasEventType(t, events, factoryapi.FactoryEventTypeDispatchResponse)
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if support.DispatchObservationIncludesWork(dispatch, workID) && dispatch.Response != nil {
			return
		}
	}
	t.Fatalf("public Factory Events contain no completed dispatch for work %q", workID)
}

func assertCleanupSmokeHasEventType(t *testing.T, events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) {
	t.Helper()

	for _, event := range events {
		if event.Type == eventType {
			return
		}
	}
	t.Fatalf("GetFactoryEvents missing %s in canonical history", eventType)
}

func assertCleanupSmokeDashboardShell(t *testing.T, baseURL string) {
	t.Helper()

	resp, err := http.Get(baseURL + "/dashboard/ui")
	if err != nil {
		t.Fatalf("GET /dashboard/ui: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /dashboard/ui: %v", err)
	}
	shell := string(body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /dashboard/ui status = %d, want 200: %s", resp.StatusCode, shell)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("GET /dashboard/ui content type = %q, want html shell", resp.Header.Get("Content-Type"))
	}
	if strings.TrimSpace(shell) == "" {
		t.Fatal("GET /dashboard/ui returned an empty shell")
	}

	routeResp, err := http.Get(baseURL + "/dashboard/ui/work/" + url.PathEscape("work-from-cleanup-smoke"))
	if err != nil {
		t.Fatalf("GET /dashboard/ui/work/...: %v", err)
	}
	defer routeResp.Body.Close()
	routeBody, err := io.ReadAll(routeResp.Body)
	if err != nil {
		t.Fatalf("read dashboard client route: %v", err)
	}
	if routeResp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard client route status = %d, want 200", routeResp.StatusCode)
	}
	if string(routeBody) != shell {
		t.Fatal("dashboard client route should fall back to the embedded app shell")
	}
}

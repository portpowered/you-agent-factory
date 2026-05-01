package runtime_api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "cleanup smoke"},
	})
	work := waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	completed := work.Results[0]
	if completed.TraceId != traceID {
		t.Fatalf("GET /work trace_id = %q, want %q", completed.TraceId, traceID)
	}
	if completed.PlaceId != "task:complete" {
		t.Fatalf("GET /work place_id = %q, want task:complete", completed.PlaceId)
	}

	statusRead := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if statusRead.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", statusRead.TotalTokens)
	}
	if statusRead.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", statusRead.Categories.Terminal)
	}
	assertCleanupSmokeCanonicalFactoryEvents(t, server, completed.WorkId)
	assertGeneratedEventsStreamHasCanonicalHistory(t, server.URL())
	assertCleanupSmokeDashboardShell(t, server.URL())
}

func assertCleanupSmokeCanonicalFactoryEvents(t *testing.T, server *functionalAPIServer, workID string) {
	t.Helper()

	events, err := server.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertCleanupSmokeHasEventType(t, events, factoryapi.FactoryEventTypeWorkRequest)
	assertCleanupSmokeHasEventType(t, events, factoryapi.FactoryEventTypeDispatchRequest)
	assertCleanupSmokeHasEventType(t, events, factoryapi.FactoryEventTypeDispatchResponse)

	worldState, err := projections.ReconstructFactoryWorldState(events, cleanupSmokeMaxTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	worldView := projections.BuildFactoryWorldView(worldState)
	if worldView.Runtime.Session.CompletedCount != 1 {
		t.Fatalf("canonical world view completed count = %d, want 1", worldView.Runtime.Session.CompletedCount)
	}
	if got := worldView.Runtime.PlaceTokenCounts["task:complete"]; got != 1 {
		t.Fatalf("canonical world view task:complete count = %d, want 1", got)
	}
	if !cleanupSmokePlaceContainsWork(worldView.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:complete"], workID) {
		t.Fatalf("canonical world view task:complete occupancy = %#v, want work %q", worldView.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:complete"], workID)
	}
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

func cleanupSmokeMaxTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func cleanupSmokePlaceContainsWork(items []interfaces.FactoryWorldWorkItemRef, workID string) bool {
	for _, item := range items {
		if item.WorkID == workID {
			return true
		}
	}
	return false
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

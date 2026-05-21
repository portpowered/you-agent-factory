package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func TestDeprecatedFactoryApiRoutesAreNotRegistered(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	for _, path := range []string{"/dashboard", "/dashboard/stream", "/state", "/traces/trace-id", "/work/token-1/trace", "/workflows", "/workflows/wf-1"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestGetDashboardUI_ReturnsEmbeddedShell(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"<title>Infinite You Dashboard</title>", "Standalone live dashboard shell for Infinite You.", "Infinite%20You%20dashboard%20icon", "<div id=\"root\"></div>", "/dashboard/ui/assets/"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected embedded dashboard shell to contain %q, got body: %s", want, rec.Body.String())
		}
	}
	for _, retired := range []string{"Agent Factory Dashboard", "Standalone live dashboard shell for Agent Factory.", "Port OS Agent Factory"} {
		if strings.Contains(rec.Body.String(), retired) {
			t.Fatalf("expected embedded dashboard shell to retire %q, got body: %s", retired, rec.Body.String())
		}
	}
}

func TestGetDashboardUI_ServesEmbeddedAsset(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	shellReq := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	shellRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(shellRec, shellReq)

	assetReq := httptest.NewRequest(http.MethodGet, embeddedDashboardAssetPath(t, shellRec.Body.String()), nil)
	assetRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK || assetRec.Body.Len() == 0 {
		t.Fatalf("embedded asset response = status %d len %d", assetRec.Code, assetRec.Body.Len())
	}
}

func TestGetDashboardUI_FallbacksToIndexForClientRoutes(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui/workstations/live", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<div id=\"root\"></div>") {
		t.Fatalf("expected SPA fallback index shell, got status %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetEvents_ReplaysHistoryThenStreamsLiveEventsInOrder(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	historical := testHistoricalFactoryEvents(t, eventTime)
	liveEvents := make(chan factoryapi.FactoryEvent, 1)
	mf := &testutil.MockFactory{FactoryEventStream: &interfaces.FactoryEventStream{History: historical, Events: liveEvents}}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	reader, closeStream := openEventStreamReader(t, server.URL)
	defer closeStream()
	assertHistoricalEventsReplay(t, reader, historical)
	assertLiveEventReplay(t, reader, liveEvents, eventTime)
}

func testHistoricalFactoryEvents(t *testing.T, eventTime time.Time) []factoryapi.FactoryEvent {
	t.Helper()

	runStartedFactory := factoryapi.Factory{
		WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL}}}},
	}
	return []factoryapi.FactoryEvent{
		testFactoryEvent(t, factoryapi.FactoryEventTypeRunRequest, "factory-event/run-started", factoryapi.FactoryEventContext{Tick: 0, EventTime: eventTime}, factoryapi.RunRequestEventPayload{RecordedAt: eventTime, Factory: runStartedFactory}),
		testFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-event/initial-structure/0", factoryapi.FactoryEventContext{Tick: 0, EventTime: eventTime}, factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{Name: "factory"}}),
		testFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "factory-event/work-request/request-1", factoryapi.FactoryEventContext{Tick: 1, EventTime: time.Date(2026, 4, 8, 12, 0, 1, 0, time.UTC), RequestId: stringPointerForAPITest("request-1")}, factoryapi.WorkRequestEventPayload{Type: factoryapi.WorkRequestTypeFactoryRequestBatch}),
	}
}

func openEventStreamReader(t *testing.T, serverURL string) (*bufio.Reader, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	return bufio.NewReader(resp.Body), func() {
		cancel()
		_ = resp.Body.Close()
	}
}

func assertHistoricalEventsReplay(t *testing.T, reader *bufio.Reader, historical []factoryapi.FactoryEvent) {
	t.Helper()

	first := readSSEFactoryEvent(t, reader)
	second := readSSEFactoryEvent(t, reader)
	third := readSSEFactoryEvent(t, reader)
	if first.Id != historical[0].Id || second.Id != historical[1].Id || third.Id != historical[2].Id {
		t.Fatalf("historical event order = [%s %s %s], want [%s %s %s]", first.Id, second.Id, third.Id, historical[0].Id, historical[1].Id, historical[2].Id)
	}
	runStartedPayload, err := first.Payload.AsRunRequestEventPayload()
	if err != nil {
		t.Fatalf("decode run-started factory payload from SSE: %v", err)
	}
	if runStartedPayload.Factory.WorkTypes == nil || len(*runStartedPayload.Factory.WorkTypes) != 1 {
		t.Fatalf("run-started factory payload = %#v, want generated factory work types", runStartedPayload.Factory)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal streamed run-started event: %v", err)
	}
	if strings.Contains(string(firstJSON), "effectiveConfig") {
		t.Fatalf("streamed run-started event contains legacy effectiveConfig: %s", firstJSON)
	}
}

func assertLiveEventReplay(t *testing.T, reader *bufio.Reader, liveEvents chan factoryapi.FactoryEvent, eventTime time.Time) {
	t.Helper()

	live := testFactoryEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "factory-event/dispatch-created/dispatch-1", factoryapi.FactoryEventContext{
		Tick:       2,
		EventTime:  time.Date(2026, 4, 8, 12, 0, 2, 0, time.UTC),
		DispatchId: stringPointerForAPITest("dispatch-1"),
	}, factoryapi.DispatchRequestEventPayload{TransitionId: "review", Inputs: []factoryapi.DispatchConsumedWorkRef{}})
	liveEvents <- live

	fourth := readSSEFactoryEvent(t, reader)
	if fourth.Id != live.Id || fourth.Type != factoryapi.FactoryEventTypeDispatchRequest || fourth.Context.Tick != 2 {
		t.Fatalf("live event = %#v, want request event at tick 2", fourth)
	}
}

func TestGetEvents_ClientDisconnectCancelsSubscription(t *testing.T) {
	liveEvents := make(chan factoryapi.FactoryEvent)
	mf := &testutil.MockFactory{
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{
				testFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-event/initial-structure/0", factoryapi.FactoryEventContext{Tick: 0, EventTime: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)}, factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{Name: "factory"}}),
			},
			Events: liveEvents,
		},
	}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}

	_ = readSSEFactoryEvent(t, bufio.NewReader(resp.Body))
	cancel()
	_ = resp.Body.Close()

	streamCtx := mf.FactoryEventStreamCtx
	if streamCtx == nil {
		t.Fatal("expected subscription context")
	}
	select {
	case <-streamCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("expected subscription context cancellation after client disconnect")
	}
}

func TestDashboardSnapshotRoutes_RemovedFromRouter(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	for _, path := range []string{"/dashboard", "/dashboard/stream"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s status = %d, want route removed", path, rec.Code)
		}
	}
}

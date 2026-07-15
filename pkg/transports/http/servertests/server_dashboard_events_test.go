package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func newAPITestServer(f *testutil.MockFactory) *api.Server {
	logger, _ := zap.NewDevelopment()
	return api.NewServer(f, 8080, logger)
}

func readAPISSEFactoryEvent(t *testing.T, reader *bufio.Reader) factoryapi.FactoryEvent {
	t.Helper()

	var dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "event:") {
			t.Fatalf("factory event stream should use default SSE message event, got line %q", line)
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}
	if dataLine == "" {
		t.Fatal("expected SSE data payload")
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		t.Fatalf("decode SSE factory event: %v", err)
	}
	return event
}

func embeddedAPIDashboardAssetPath(t *testing.T, html string) string {
	t.Helper()

	pattern := regexp.MustCompile(`(?:src|href)="(/dashboard/ui/assets/[^"]+)"`)
	matches := pattern.FindStringSubmatch(html)
	if len(matches) != 2 {
		t.Fatalf("expected embedded dashboard asset path in html: %s", html)
	}
	return matches[1]
}

func testAPIFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
	t.Helper()

	var eventPayload factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		err = eventPayload.FromRunRequestEventPayload(typed)
	case factoryapi.InitialStructureRequestEventPayload:
		err = eventPayload.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = eventPayload.FromWorkRequestEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	default:
		t.Fatalf("unsupported test factory event payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode test factory event payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
		Id:            id,
		Context:       context,
		Payload:       eventPayload,
	}
}

func stringPointerForAPIServerTest(value string) *string {
	return &value
}

func TestDeprecatedFactoryApiRoutesAreNotRegistered(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{})
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
	srv := newAPITestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"<title>You Agent Factory Dashboard</title>", "Standalone live dashboard shell for You Agent Factory.", "You%20Agent%20Factory%20dashboard%20icon", "<div id=\"root\"></div>", "/dashboard/ui/assets/"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected embedded dashboard shell to contain %q, got body: %s", want, rec.Body.String())
		}
	}
	for _, retired := range []string{"Infinite You Dashboard", "Standalone live dashboard shell for Infinite You.", "Infinite%20You%20dashboard%20icon"} {
		if strings.Contains(rec.Body.String(), retired) {
			t.Fatalf("expected embedded dashboard shell to retire %q, got body: %s", retired, rec.Body.String())
		}
	}
}

func TestGetDashboardUI_ServesEmbeddedAsset(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{})
	shellReq := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	shellRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(shellRec, shellReq)

	assetReq := httptest.NewRequest(http.MethodGet, embeddedAPIDashboardAssetPath(t, shellRec.Body.String()), nil)
	assetRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK || assetRec.Body.Len() == 0 {
		t.Fatalf("embedded asset response = status %d len %d", assetRec.Code, assetRec.Body.Len())
	}
}

func TestGetDashboardUI_FallbacksToIndexForClientRoutes(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui/workstations/live", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<div id=\"root\"></div>") {
		t.Fatalf("expected SPA fallback index shell, got status %d body: %s", rec.Code, rec.Body.String())
	}
}

// TestCompatibilityGetEvents_* exercises compatibility-only process-global GET /events behavior.
// Dashboard, Factory Session, and replay smokes should use session-scoped routes instead.
func TestCompatibilityGetEvents_ReplaysHistoryThenStreamsLiveEventsInOrder(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	historical := testHistoricalFactoryEvents(t, eventTime)
	liveEvents := make(chan interfaces.FactoryEvent, 1)
	mf := &testutil.MockFactory{FactoryEventStream: &interfaces.FactoryEventStream{History: testutil.FactoryEvents(t, historical), Events: liveEvents}}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(api.NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	reader, closeStream := openCompatibilityEventStreamReader(t, server.URL)
	defer closeStream()
	assertHistoricalEventsReplay(t, reader, historical)
	assertLiveEventReplay(t, reader, liveEvents, eventTime)
}

func TestSessionScopedLiveGetEvents_JSONRecoveryProbeReturnsCursorStaleForLiveSession(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	sessionID := "session-alpha"
	historical := testHistoricalFactoryEvents(t, eventTime)
	mf := &testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			sessionID: {
				FactoryEvents: historical,
			},
		},
	}

	server := httptest.NewServer(newAPITestServer(mf).Handler())
	defer server.Close()

	eventPath := "/factory-sessions/" + sessionID + "/events?after_event_id=missing-event-id"
	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+eventPath,
		nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET recovery probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var recovery factoryapi.FactorySessionEventStreamRecovery
	if err := json.NewDecoder(resp.Body).Decode(&recovery); err != nil {
		t.Fatalf("decode recovery response: %v", err)
	}
	if recovery.FactorySessionId != sessionID {
		t.Fatalf("factorySessionId = %q, want %q", recovery.FactorySessionId, sessionID)
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcome("CURSOR_STALE") {
		t.Fatalf("outcome = %q, want CURSOR_STALE", recovery.Outcome)
	}
	if !recovery.Retry.OmitAfterEventId || !recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want both omit flags true", recovery.Retry)
	}
	assertSessionScopedFactoryEventsPath(t, eventPath, sessionID)
}

func assertSessionScopedFactoryEventsPath(t *testing.T, path, sessionID string) {
	t.Helper()
	wantPrefix := "/factory-sessions/" + sessionID + "/events"
	if !strings.HasPrefix(path, wantPrefix) {
		t.Fatalf("event stream path = %q, want session-scoped prefix %q", path, wantPrefix)
	}
	if path == "/events" || strings.HasPrefix(path, "/events?") {
		t.Fatalf("event stream path = %q, must not use compatibility-only GET /events", path)
	}
}

func TestSessionScopedLiveGetEvents_JSONRecoveryProbeReturnsUnknownSessionForMissingLiveSession(t *testing.T) {
	server := httptest.NewServer(newAPITestServer(&testutil.MockFactory{}).Handler())
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+"/factory-sessions/session-missing/events?after_event_id=missing-event-id",
		nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET recovery probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var recovery factoryapi.FactorySessionEventStreamRecovery
	if err := json.NewDecoder(resp.Body).Decode(&recovery); err != nil {
		t.Fatalf("decode recovery response: %v", err)
	}
	if recovery.FactorySessionId != "session-missing" {
		t.Fatalf("factorySessionId = %q, want session-missing", recovery.FactorySessionId)
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcome("UNKNOWN_SESSION") {
		t.Fatalf("outcome = %q, want UNKNOWN_SESSION", recovery.Outcome)
	}
	if recovery.Retry.OmitAfterEventId || recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want omit flags false", recovery.Retry)
	}
}

func TestSessionScopedLiveGetEvents_ValidReconnectCursorStillStreamsSSEForLiveSession(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	sessionID := "session-alpha"
	historical := testHistoricalFactoryEvents(t, eventTime)
	mf := &testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			sessionID: {
				FactoryEvents: historical,
			},
		},
	}

	server := httptest.NewServer(newAPITestServer(mf).Handler())
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+"/factory-sessions/"+sessionID+"/events?after_event_id="+historical[1].Id,
		nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(resp.Body)
	event := readAPISSEFactoryEvent(t, reader)
	if event.Id != historical[2].Id {
		t.Fatalf("event id = %q, want %q", event.Id, historical[2].Id)
	}
}

func testHistoricalFactoryEvents(t *testing.T, eventTime time.Time) []factoryapi.FactoryEvent {
	t.Helper()

	runStartedFactory := factoryapi.Factory{
		WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL}}}},
	}
	return []factoryapi.FactoryEvent{
		testAPIFactoryEvent(t, factoryapi.FactoryEventTypeRunRequest, "factory-event/run-started", factoryapi.FactoryEventContext{Tick: 0, EventTime: eventTime}, factoryapi.RunRequestEventPayload{RecordedAt: eventTime, Factory: runStartedFactory}),
		testAPIFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-event/initial-structure/0", factoryapi.FactoryEventContext{Tick: 0, EventTime: eventTime}, factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{Name: "factory"}}),
		testAPIFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "factory-event/work-request/request-1", factoryapi.FactoryEventContext{Tick: 1, EventTime: time.Date(2026, 4, 8, 12, 0, 1, 0, time.UTC), RequestId: stringPointerForAPIServerTest("request-1")}, factoryapi.WorkRequestEventPayload{Type: factoryapi.WorkRequestTypeFactoryRequestBatch}),
	}
}

// openCompatibilityEventStreamReader opens process-global GET /events for retained compatibility coverage.
func openCompatibilityEventStreamReader(t *testing.T, serverURL string) (*bufio.Reader, func()) {
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
		t.Fatalf("compatibility GET /events status = %d, body = %s", resp.StatusCode, string(body))
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

	first := readAPISSEFactoryEvent(t, reader)
	second := readAPISSEFactoryEvent(t, reader)
	third := readAPISSEFactoryEvent(t, reader)
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

func assertLiveEventReplay(t *testing.T, reader *bufio.Reader, liveEvents chan interfaces.FactoryEvent, eventTime time.Time) {
	t.Helper()

	live := testAPIFactoryEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "factory-event/dispatch-created/dispatch-1", factoryapi.FactoryEventContext{
		Tick:       2,
		EventTime:  time.Date(2026, 4, 8, 12, 0, 2, 0, time.UTC),
		DispatchId: stringPointerForAPIServerTest("dispatch-1"),
	}, factoryapi.DispatchRequestEventPayload{TransitionId: "review", Inputs: []factoryapi.DispatchConsumedWorkRef{}})
	liveEvents <- testutil.FactoryEvent(t, live)

	fourth := readAPISSEFactoryEvent(t, reader)
	if fourth.Id != live.Id || fourth.Type != factoryapi.FactoryEventTypeDispatchRequest || fourth.Context.Tick != 2 {
		t.Fatalf("live event = %#v, want request event at tick 2", fourth)
	}
}

func TestCompatibilityGetEvents_ReconnectAfterEventIDSkipsAcknowledgedHistory(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	historical := testHistoricalFactoryEvents(t, eventTime)
	liveEvents := make(chan interfaces.FactoryEvent, 1)
	mf := &testutil.MockFactory{FactoryEventStream: &interfaces.FactoryEventStream{History: testutil.FactoryEvents(t, historical), Events: liveEvents}}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(api.NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/events?after_event_id="+historical[1].Id, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	replayed := readAPISSEFactoryEvent(t, reader)
	if replayed.Id != historical[2].Id {
		t.Fatalf("reconnect replay = %q, want only events after %q", replayed.Id, historical[1].Id)
	}
}

func TestCompatibilityGetEvents_ClientDisconnectCancelsSubscription(t *testing.T) {
	liveEvents := make(chan interfaces.FactoryEvent)
	mf := &testutil.MockFactory{
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []interfaces.FactoryEvent{
				testutil.FactoryEvent(t, testAPIFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-event/initial-structure/0", factoryapi.FactoryEventContext{Tick: 0, EventTime: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)}, factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{Name: "factory"}})),
			},
			Events: liveEvents,
		},
	}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(api.NewServer(mf, 8080, logger).Handler())
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

	_ = readAPISSEFactoryEvent(t, bufio.NewReader(resp.Body))
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

func TestCompatibilityGetEvents_ReconnectAfterSequenceSkipsAcknowledgedHistory(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	historical := testHistoricalFactoryEventsWithSequence(t, eventTime)
	liveEvents := make(chan interfaces.FactoryEvent, 1)
	mf := &testutil.MockFactory{FactoryEventStream: &interfaces.FactoryEventStream{History: testutil.FactoryEvents(t, historical), Events: liveEvents}}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(api.NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/events?after_sequence=0", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("compatibility GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("compatibility GET /events status = %d, body = %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	replayed := readAPISSEFactoryEvent(t, reader)
	if replayed.Id != historical[1].Id {
		t.Fatalf("compatibility reconnect replay = %q, want only events after sequence 0 (%q)", replayed.Id, historical[1].Id)
	}
}

func TestCompatibilityGetEvents_InvalidReconnectCursorReturnsBadRequest(t *testing.T) {
	historical := testHistoricalFactoryEvents(t, time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC))
	mf := &testutil.MockFactory{FactoryEventStream: &interfaces.FactoryEventStream{History: testutil.FactoryEvents(t, historical)}}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(api.NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/events?after_event_id=missing-event-id", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("compatibility GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("compatibility GET /events status = %d, want 400: %s", resp.StatusCode, readBody(t, resp))
	}

	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode compatibility error response: %v", err)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("compatibility error code = %q, want BAD_REQUEST", errResp.Code)
	}
}

func testHistoricalFactoryEventsWithSequence(t *testing.T, eventTime time.Time) []factoryapi.FactoryEvent {
	t.Helper()

	historical := testHistoricalFactoryEvents(t, eventTime)
	for index := range historical {
		sequence := index
		historical[index].Context.Sequence = sequence
	}
	return historical
}

func TestDashboardSnapshotRoutes_RemovedFromRouter(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{})
	for _, path := range []string{"/dashboard", "/dashboard/stream"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s status = %d, want route removed", path, rec.Code)
		}
	}
}

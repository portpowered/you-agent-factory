package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestGetFactorySessionEvents_RuntimeBackedReturnsCanonicalEvents(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-events-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	events := getDurableFactorySessionEvents(t, server.URL, syncResult.SessionId, "")
	if len(events) < 3 {
		t.Fatalf("events = %d, want start/result-updated/completed", len(events))
	}
	assertRuntimeBackedCanonicalEvent(t, events[0], "SESSION_STARTED", "session-started/"+syncResult.SessionId, syncResult.SessionId)
	assertRuntimeBackedCanonicalEvent(t, events[1], "SESSION_RESULT_UPDATED", "", syncResult.SessionId)
	assertRuntimeBackedCanonicalEvent(t, events[2], "SESSION_COMPLETED", "session-completed/"+syncResult.SessionId, syncResult.SessionId)
}

func TestGetFactorySessionEvents_RuntimeBackedReconnectCursorReturnsLaterEvents(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-events-reconnect-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	all := getDurableFactorySessionEvents(t, server.URL, syncResult.SessionId, "")
	if len(all) < 3 {
		t.Fatalf("events = %d, want at least 3", len(all))
	}
	afterStart := getDurableFactorySessionEvents(
		t,
		server.URL,
		syncResult.SessionId,
		"after_event_id=session-started/"+syncResult.SessionId,
	)
	if len(afterStart) != len(all)-1 {
		t.Fatalf("after start events = %d, want %d", len(afterStart), len(all)-1)
	}
	if afterStart[0].Type != factoryapi.FactoryEventTypeSessionResultUpdated {
		t.Fatalf("first reconnect event type = %q, want SESSION_RESULT_UPDATED", afterStart[0].Type)
	}
}

func TestGetFactorySessionEvents_RuntimeBackedUnknownCursorReturnsBadRequest(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-events-missing-cursor-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	resp, err := http.Get(server.URL + "/factory-sessions/" + syncResult.SessionId + "/events?after_event_id=missing-event-id")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, readBody(t, resp))
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.BADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
}

func TestGetFactorySessionEvents_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-events-001/events")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestGetFactorySessionEvents_RuntimeBackedAPIShapingMatchesServiceProjection(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-events-projection-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	serviceEvents, err := service.ReadEvents(context.Background(), syncResult.SessionId, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("service ReadEvents: %v", err)
	}
	want := factorysession.EventReadResponseToAPI(serviceEvents)
	got := getDurableFactorySessionEvents(t, server.URL, syncResult.SessionId, "")
	assertFactoryEventsJSONEqual(t, want, got)
}

func TestGetFactorySessionEvents_RuntimeBackedReplayMatchesReadAndResultAPIs(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-events-replay-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	apiEvents := getDurableFactorySessionEvents(t, server.URL, syncResult.SessionId, "")
	rawEvents := make([]json.RawMessage, 0, len(apiEvents))
	for _, event := range apiEvents {
		encoded, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatalf("marshal api event: %v", marshalErr)
		}
		rawEvents = append(rawEvents, encoded)
	}
	replayedSession, replayedResult, err := factorysessionexecution.ReplaySessionProjection(rawEvents)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}

	apiRead := getDurableFactorySession(t, server.URL, syncResult.SessionId)
	if string(apiRead.Status) != string(replayedSession.Status) {
		t.Fatalf("replayed status = %q, want API read status %q", replayedSession.Status, apiRead.Status)
	}
	if apiRead.ResultSummary == nil || string(apiRead.ResultSummary.ResultStatus) != string(replayedResult.ResultStatus) {
		t.Fatalf("replayed result status = %q, want API result summary %#v", replayedResult.ResultStatus, apiRead.ResultSummary)
	}

	apiResult := getDurableFactorySessionResult(t, server.URL, syncResult.SessionId, "")
	if string(apiResult.ResultStatus) != string(replayedResult.ResultStatus) {
		t.Fatalf("replayed result status = %q, want API result status %q", replayedResult.ResultStatus, apiResult.ResultStatus)
	}
}

func TestGetFactorySessionEvents_LivePetriSessionRemainsCompatible(t *testing.T) {
	closed := make(chan factoryapi.FactoryEvent)
	close(closed)
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"session-beta": {
				FactoryEventStream: &interfaces.FactoryEventStream{
					History: []factoryapi.FactoryEvent{{Id: "event-1", Type: factoryapi.FactoryEventTypeWorkRequest}},
					Events:  closed,
				},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/events", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	streamed := readSSEFactoryEvent(t, bufio.NewReader(rec.Body))
	if streamed.Id != "event-1" {
		t.Fatalf("streamed event id = %q, want event-1", streamed.Id)
	}
}

func getDurableFactorySessionEvents(t *testing.T, serverURL, sessionID, query string) []factoryapi.FactoryEvent {
	t.Helper()
	url := serverURL + "/factory-sessions/" + sessionID + "/events"
	if query != "" {
		url += "?" + query
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get events status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	return readAllSSEFactoryEvents(t, bufio.NewReader(resp.Body))
}

func readAllSSEFactoryEvents(t *testing.T, reader *bufio.Reader) []factoryapi.FactoryEvent {
	t.Helper()
	events := make([]factoryapi.FactoryEvent, 0)
	for {
		event, ok, err := tryReadSSEFactoryEvent(reader)
		if err != nil {
			t.Fatalf("read SSE factory event: %v", err)
		}
		if !ok {
			return events
		}
		events = append(events, event)
	}
}

func tryReadSSEFactoryEvent(reader *bufio.Reader) (factoryapi.FactoryEvent, bool, error) {
	var dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if len(dataLine) == 0 {
				return factoryapi.FactoryEvent{}, false, nil
			}
			return factoryapi.FactoryEvent{}, false, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}
	if dataLine == "" {
		return factoryapi.FactoryEvent{}, false, nil
	}
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return factoryapi.FactoryEvent{}, false, err
	}
	return event, true, nil
}

func assertRuntimeBackedCanonicalEvent(t *testing.T, event factoryapi.FactoryEvent, wantType, wantID, wantSessionID string) {
	t.Helper()
	if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
		t.Fatalf("schemaVersion = %q, want agent-factory.event.v1", event.SchemaVersion)
	}
	if wantType != "" && string(event.Type) != wantType {
		t.Fatalf("type = %q, want %q", event.Type, wantType)
	}
	if wantID != "" && event.Id != wantID {
		t.Fatalf("id = %q, want %q", event.Id, wantID)
	}
	if event.Context.SessionId == nil || *event.Context.SessionId != wantSessionID {
		t.Fatalf("context.sessionId = %#v, want %q", event.Context.SessionId, wantSessionID)
	}
	if event.Context.Sequence < 0 {
		t.Fatalf("context.sequence = %d, want non-negative", event.Context.Sequence)
	}
}

func assertFactoryEventsJSONEqual(t *testing.T, want, got []factoryapi.FactoryEvent) {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want events: %v", err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got events: %v", err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("API events JSON diverged from EventReadResponseToAPI projection:\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
}

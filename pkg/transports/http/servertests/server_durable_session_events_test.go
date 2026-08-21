package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

type liveFactoryEventWorkAPI struct {
	apisurface.WorkAPI
	stream *interfaces.FactoryEventStream
}

func (api liveFactoryEventWorkAPI) SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	return api.stream, nil
}

func (api liveFactoryEventWorkAPI) ProbeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) error {
	return nil
}

func TestGetFactorySessionEvents_RuntimeBackedReturnsCanonicalEvents(t *testing.T) {
	const sessionID = "dur-sess-api-events-001"
	service := apiExecutionScript{
		startSync: apiSyncStartCallback(sessionID),
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return apiTerminalEvents(sessionID), nil
		},
	}
	srv := newDurableAPITestServer(service)
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
	const sessionID = "dur-sess-api-events-reconnect-001"
	service := apiExecutionScript{
		startSync: apiSyncStartCallback(sessionID),
		readEvents: func(_ context.Context, _ string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			result := apiTerminalEvents(sessionID)
			if request.AfterEventID == "" {
				return result, nil
			}
			if request.AfterEventID != "session-started/"+sessionID {
				t.Fatalf("afterEventId = %q, want session start cursor", request.AfterEventID)
			}
			result.Events = result.Events[1:]
			return result, nil
		},
	}
	srv := newDurableAPITestServer(service)
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
	const sessionID = "dur-sess-api-events-cursor-001"
	service := apiExecutionScript{
		startSync: apiSyncStartCallback(sessionID),
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return factorysessionexecution.EventReadResult{}, factorysessionexecution.ErrReconnectCursorNotFound
		},
	}
	srv := newDurableAPITestServer(service)
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
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
}

func TestGetFactorySessionEvents_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return factorysessionexecution.EventReadResult{}, factorysessionexecution.ErrDurableSessionNotFound
		},
	}
	srv := newDurableAPITestServer(service)
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
	const sessionID = "dur-sess-api-events-projection-001"
	serviceEvents := apiTerminalEvents(sessionID)
	service := apiExecutionScript{
		startSync: apiSyncStartCallback(sessionID),
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return serviceEvents, nil
		},
	}
	srv := newDurableAPITestServer(service)
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

	want := factorysession.EventReadResponseToAPI(serviceEvents)
	got := getDurableFactorySessionEvents(t, server.URL, syncResult.SessionId, "")
	assertFactoryEventsJSONEqual(t, want, got)
}

func TestGetFactorySessionEvents_RuntimeBackedReplayMatchesReadAndResultAPIs(t *testing.T) {
	const sessionID = "dur-sess-api-events-read-result-001"
	service := apiExecutionScript{
		startSync: apiSyncStartCallback(sessionID),
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return apiTerminalEvents(sessionID), nil
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			return terminalAPIReadResult(sessionID), nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return finalAPIResult(sessionID), nil
		},
	}
	srv := newDurableAPITestServer(service)
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
	if len(apiEvents) != 3 || apiEvents[len(apiEvents)-1].Type != factoryapi.FactoryEventTypeSessionCompleted {
		t.Fatalf("events = %#v, want terminal canonical event stream", apiEvents)
	}

	apiRead := getDurableFactorySession(t, server.URL, syncResult.SessionId)
	if apiRead.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("read status = %q, want SUCCEEDED", apiRead.Status)
	}
	if apiRead.ResultSummary == nil || apiRead.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("API result summary = %#v, want FINAL", apiRead.ResultSummary)
	}

	apiResult := getDurableFactorySessionResult(t, server.URL, syncResult.SessionId, "")
	if apiResult.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("API result status = %q, want FINAL", apiResult.ResultStatus)
	}
}

func TestGetFactorySessionEvents_LivePetriSessionRemainsCompatible(t *testing.T) {
	closed := make(chan interfaces.FactoryEvent)
	close(closed)
	event, err := interfaces.NewFactoryEvent(factoryapi.FactoryEvent{Id: "event-1", Type: factoryapi.FactoryEventTypeWorkRequest})
	if err != nil {
		t.Fatalf("convert live FactoryEvent fixture: %v", err)
	}
	srv := newWorkAPITestServer(liveFactoryEventWorkAPI{stream: &interfaces.FactoryEventStream{
		History: []interfaces.FactoryEvent{event},
		Events:  closed,
	}})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/events", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	streamed := readAPISSEFactoryEvent(t, bufio.NewReader(rec.Body))
	if streamed.Id != "event-1" {
		t.Fatalf("streamed event id = %q, want event-1", streamed.Id)
	}
}

func TestGetFactorySessionEvents_LiveAssociationProjectsStrictPublicPayload(t *testing.T) {
	closed := make(chan interfaces.FactoryEvent)
	close(closed)
	canonical := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			DispatchID: strPtr("dispatch-actual-7"),
			EventTime:  time.Date(2026, 8, 4, 16, 30, 0, 0, time.UTC),
			Sequence:   17,
			Tick:       8,
		},
		Id:            "factory-event/dispatch-worker-session-association/dispatch-actual-7",
		Payload:       json.RawMessage(`{"workerSessionId":"worker-session-actual-11","model":"gpt-5.6-luna","reasoningEffort":"high"}`),
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}
	srv := newWorkAPITestServer(liveFactoryEventWorkAPI{stream: &interfaces.FactoryEventStream{
		History: []interfaces.FactoryEvent{canonical},
		Events:  closed,
	}})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/factory-sessions/session-beta/events")
	if err != nil {
		t.Fatalf("GET Factory Events route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET Factory Events route status = %d, want 200: %s", response.StatusCode, readBody(t, response))
	}

	events := readAllSSEFactoryEvents(t, bufio.NewReader(response.Body))
	if len(events) != 1 {
		t.Fatalf("served event count = %d, want 1", len(events))
	}
	served := events[0]
	if served.Id != canonical.Id || served.Type != factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation {
		t.Fatalf("served association identity = %#v, want id %q and association type", served, canonical.Id)
	}
	if served.Context.DispatchId == nil || *served.Context.DispatchId != "dispatch-actual-7" {
		t.Fatalf("served context.dispatchId = %#v, want dispatch-actual-7", served.Context.DispatchId)
	}

	publicPayload, err := json.Marshal(served.Payload)
	if err != nil {
		t.Fatalf("marshal served association payload: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(publicPayload, &fields); err != nil {
		t.Fatalf("decode served association payload: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("served association payload fields = %v, want only workerSessionId", fields)
	}
	var workerSessionID string
	if err := json.Unmarshal(fields["workerSessionId"], &workerSessionID); err != nil {
		t.Fatalf("decode served workerSessionId: %v", err)
	}
	if workerSessionID != "worker-session-actual-11" {
		t.Fatalf("served workerSessionId = %q, want worker-session-actual-11", workerSessionID)
	}
	if string(canonical.Payload) != `{"workerSessionId":"worker-session-actual-11","model":"gpt-5.6-luna","reasoningEffort":"high"}` {
		t.Fatalf("canonical association payload changed to %s", canonical.Payload)
	}

	assertServedFactoryEventMatchesBundledSchema(t, served)
}

func assertServedFactoryEventMatchesBundledSchema(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load bundled OpenAPI contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate bundled OpenAPI contract: %v", err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal served Factory Event: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode served Factory Event: %v", err)
	}
	if err := doc.Components.Schemas["FactoryEvent"].Value.VisitJSON(document); err != nil {
		t.Fatalf("served Factory Event does not validate against bundled schema: %v", err)
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

func getDurableArtifactDetail(t *testing.T, serverURL, sessionID, artifactID string) factoryapi.FactorySessionArtifactDetail {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/artifacts/" + artifactID)
	if err != nil {
		t.Fatalf("GET artifact detail: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("artifact detail status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.FactorySessionArtifactDetail
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact detail: %v", err)
	}
	return response
}

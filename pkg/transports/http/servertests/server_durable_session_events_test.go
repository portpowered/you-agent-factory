package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestGetFactorySessionEvents_RuntimeBackedReturnsCanonicalEvents(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	closed := make(chan interfaces.FactoryEvent)
	close(closed)
	srv := newAPITestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"session-beta": {
				FactoryEventStream: &interfaces.FactoryEventStream{
					History: []interfaces.FactoryEvent{testutil.FactoryEvent(t, factoryapi.FactoryEvent{Id: "event-1", Type: factoryapi.FactoryEventTypeWorkRequest})},
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
	streamed := readAPISSEFactoryEvent(t, bufio.NewReader(rec.Body))
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

func newAPILiveProviderRuntimeService(t *testing.T) factorysessionexecution.Service {
	t.Helper()
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	return newAPIJavaScriptExecutionService(t, projectRoot, factorysessionexecution.ChildExecutorModeLive,
		factorysessionexecution.SmokeLiveChildProvider())
}

func newAPILiveProviderBlockingRuntimeService(t *testing.T) (
	*factorysessionexecution.JavaScriptRuntimeService,
	*apiLiveProviderBlockingFixtureProvider,
) {
	t.Helper()
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	provider := &apiLiveProviderBlockingFixtureProvider{}
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeLive, provider)
	return service, provider
}

type apiLiveProviderBlockingFixtureProvider struct {
	mu           sync.Mutex
	inferStarted chan struct{}
	release      chan struct{}
}

func (p *apiLiveProviderBlockingFixtureProvider) Infer(ctx context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	if p.inferStarted == nil {
		p.inferStarted = make(chan struct{})
	}
	if p.release == nil {
		p.release = make(chan struct{})
	}
	started := p.inferStarted
	release := p.release
	p.mu.Unlock()

	close(started)
	select {
	case <-ctx.Done():
		return workerexecution.InferenceResponse{}, ctx.Err()
	case <-release:
		return workerexecution.InferenceResponse{
			Content: `{"text":"live:agent-run-fake-child:summarize-findings:summarize workflows:workflows"}`,
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}
}

func (p *apiLiveProviderBlockingFixtureProvider) waitForInferStart(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	if p.inferStarted == nil {
		p.inferStarted = make(chan struct{})
	}
	started := p.inferStarted
	p.mu.Unlock()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider Infer did not start before timeout")
	}
}

func (p *apiLiveProviderBlockingFixtureProvider) releaseInfer() {
	p.mu.Lock()
	if p.release == nil {
		p.release = make(chan struct{})
	}
	release := p.release
	p.mu.Unlock()
	close(release)
}

func waitForAPIDispatchStatus(
	t *testing.T,
	serverURL, sessionID, dispatchID string,
	want factoryapi.FactoryDispatchStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dispatchList := getDurableDispatchList(t, serverURL, sessionID)
		for _, dispatch := range dispatchList.Dispatches {
			if dispatch.Id == dispatchID && dispatch.Status == want {
				return dispatch
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("dispatch %q did not reach status %q before timeout", dispatchID, want)
	return factoryapi.FactorySessionDispatchSummary{}
}

func assertAPILiveProviderProviderSessionRef(
	t *testing.T,
	refs *[]factoryapi.LoadableProviderSessionRef,
) {
	t.Helper()
	if refs == nil || len(*refs) != 1 {
		t.Fatalf("providerSessionRefs = %#v, want one ref", refs)
	}
	ref := (*refs)[0]
	if ref.Id != "live-provider-session-1" {
		t.Fatalf("providerSessionRef id = %q, want live-provider-session-1", ref.Id)
	}
	if ref.Provider != "mock" {
		t.Fatalf("providerSessionRef provider = %q, want mock", ref.Provider)
	}
	if ref.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("providerSessionRef kind = %q, want session_id", ref.Kind)
	}
}

func assertAPILiveProviderSessionArtifactRef(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	sessionRead := getDurableFactorySession(t, serverURL, sessionID)
	if sessionRead.ArtifactRefs == nil || len(*sessionRead.ArtifactRefs) != 1 ||
		(*sessionRead.ArtifactRefs)[0].Id != "child-artifact-1" {
		t.Fatalf("session artifactRefs = %#v, want child-artifact-1", sessionRead.ArtifactRefs)
	}
}

func assertAPILiveProviderArtifactListDetail(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	artifactList := getDurableArtifactList(t, serverURL, sessionID)
	if len(artifactList.Artifacts) != 1 {
		t.Fatalf("artifact list = %#v, want one artifact", artifactList.Artifacts)
	}
	artifactSummary := artifactList.Artifacts[0]
	if artifactSummary.Id != "child-artifact-1" {
		t.Fatalf("artifact id = %q, want child-artifact-1", artifactSummary.Id)
	}
	if artifactSummary.Kind != factoryapi.FactoryArtifactKindCHILDRESULT {
		t.Fatalf("artifact kind = %q, want CHILD_RESULT", artifactSummary.Kind)
	}
	if artifactSummary.DispatchId == nil || *artifactSummary.DispatchId != "dispatch-1" {
		t.Fatalf("artifact dispatchId = %#v, want dispatch-1", artifactSummary.DispatchId)
	}
	wantHref := "/factory-sessions/" + sessionID + "/artifacts/child-artifact-1"
	if artifactSummary.RetrievalRef == nil || artifactSummary.RetrievalRef.Href != wantHref {
		t.Fatalf("artifact retrievalRef = %#v, want %q", artifactSummary.RetrievalRef, wantHref)
	}

	artifactDetail := getDurableArtifactDetail(t, serverURL, sessionID, "child-artifact-1")
	if artifactDetail.DispatchId == nil || *artifactDetail.DispatchId != "dispatch-1" {
		t.Fatalf("artifact detail dispatchId = %#v, want dispatch-1", artifactDetail.DispatchId)
	}
	if artifactDetail.Kind != factoryapi.FactoryArtifactKindCHILDRESULT {
		t.Fatalf("artifact detail kind = %q, want CHILD_RESULT", artifactDetail.Kind)
	}
	if artifactDetail.ContentRef == nil || artifactDetail.ContentRef.Href != wantHref {
		t.Fatalf("artifact detail contentRef = %#v, want %q", artifactDetail.ContentRef, wantHref)
	}
}

func assertAPILiveProviderDispatchArtifactCrossRefs(
	t *testing.T,
	dispatchSummary factoryapi.FactorySessionDispatchSummary,
	dispatchDetail factoryapi.FactoryDispatch,
) {
	t.Helper()
	if dispatchSummary.OutputArtifactIds == nil || len(*dispatchSummary.OutputArtifactIds) != 1 ||
		(*dispatchSummary.OutputArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("dispatch outputArtifactIds = %#v, want [child-artifact-1]", dispatchSummary.OutputArtifactIds)
	}
	if dispatchDetail.ArtifactIds == nil || len(*dispatchDetail.ArtifactIds) != 1 ||
		(*dispatchDetail.ArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("dispatch detail artifactIds = %#v, want [child-artifact-1]", dispatchDetail.ArtifactIds)
	}
}

func assertAPILiveProviderArtifactLineage(
	t *testing.T,
	serverURL, sessionID string,
	dispatchSummary factoryapi.FactorySessionDispatchSummary,
	dispatchDetail factoryapi.FactoryDispatch,
) {
	t.Helper()
	assertAPILiveProviderSessionArtifactRef(t, serverURL, sessionID)
	assertAPILiveProviderArtifactListDetail(t, serverURL, sessionID)
	assertAPILiveProviderDispatchArtifactCrossRefs(t, dispatchSummary, dispatchDetail)
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

func assertAPIDispatchQueuedLifecycleEvent(t *testing.T, events []factoryapi.FactoryEvent, dispatchID string) {
	t.Helper()
	queued := findAPIFactoryEventByType(events, "DISPATCH_QUEUED", dispatchID)
	if queued == nil {
		t.Fatalf("events = %#v, want DISPATCH_QUEUED for %q", events, dispatchID)
	}
	if queued.Context.DispatchId == nil || *queued.Context.DispatchId != dispatchID {
		t.Fatalf("DISPATCH_QUEUED dispatchId = %#v, want %q", queued.Context.DispatchId, dispatchID)
	}
	payload, err := json.Marshal(queued.Payload)
	if err != nil {
		t.Fatalf("marshal DISPATCH_QUEUED payload: %v", err)
	}
	var queuedBody struct {
		DispatchKind string `json:"dispatchKind"`
	}
	if err := json.Unmarshal(payload, &queuedBody); err != nil {
		t.Fatalf("unmarshal DISPATCH_QUEUED payload: %v", err)
	}
	if queuedBody.DispatchKind != string(factoryapi.FactoryDispatchKindJAVASCRIPTAGENT) {
		t.Fatalf("DISPATCH_QUEUED dispatchKind = %q, want JAVASCRIPT_AGENT", queuedBody.DispatchKind)
	}
}

func assertAPIDispatchReconciledLifecycleEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	dispatchID string,
	summary factoryapi.FactorySessionDispatchSummary,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()
	reconciled := findAPIFactoryEventByType(events, "DISPATCH_RECONCILED", dispatchID)
	if reconciled == nil {
		t.Fatalf("events = %#v, want DISPATCH_RECONCILED for %q", events, dispatchID)
	}
	if reconciled.Context.DispatchId == nil || *reconciled.Context.DispatchId != dispatchID {
		t.Fatalf("DISPATCH_RECONCILED dispatchId = %#v, want %q", reconciled.Context.DispatchId, dispatchID)
	}
	reconciledPayload, err := json.Marshal(reconciled.Payload)
	if err != nil {
		t.Fatalf("marshal DISPATCH_RECONCILED payload: %v", err)
	}
	var reconciledBody struct {
		ReconciledStatus     factoryapi.FactoryDispatchStatus        `json:"reconciledStatus"`
		ReconciliationSource factoryapi.DispatchReconciliationSource `json:"reconciliationSource"`
		ArtifactIds          *[]string                               `json:"artifactIds"`
		FailureDetail        *factoryapi.FailureDetail               `json:"failureDetail"`
	}
	if err := json.Unmarshal(reconciledPayload, &reconciledBody); err != nil {
		t.Fatalf("unmarshal DISPATCH_RECONCILED payload: %v", err)
	}
	if reconciledBody.ReconciledStatus != detail.Status {
		t.Fatalf("DISPATCH_RECONCILED reconciledStatus = %q, want %q", reconciledBody.ReconciledStatus, detail.Status)
	}
	if detail.Status == factoryapi.FactoryDispatchStatusCOMPLETED {
		if reconciledBody.ReconciliationSource != factoryapi.PROVIDERSESSION {
			t.Fatalf("reconciliationSource = %q, want PROVIDER_SESSION", reconciledBody.ReconciliationSource)
		}
		if summary.OutputArtifactIds == nil || reconciledBody.ArtifactIds == nil ||
			len(*summary.OutputArtifactIds) != len(*reconciledBody.ArtifactIds) {
			t.Fatalf("artifactIds = %#v, want %#v", reconciledBody.ArtifactIds, summary.OutputArtifactIds)
		}
	}
	if detail.Status == factoryapi.FactoryDispatchStatusFAILED {
		assertAPILiveProviderDispatchFailureDetail(t, reconciledBody.FailureDetail)
		assertAPILiveProviderDispatchFailureDetail(t, detail.FailureDetail)
	}
}

func assertAPILiveProviderDispatchLifecycleEventsAlignWithReads(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	dispatchID string,
	summary factoryapi.FactorySessionDispatchSummary,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()
	assertAPIDispatchQueuedLifecycleEvent(t, events, dispatchID)
	if !isTerminalFactoryDispatchStatus(detail.Status) {
		return
	}
	assertAPIDispatchReconciledLifecycleEvent(t, events, dispatchID, summary, detail)
}

func findAPIFactoryEventByType(
	events []factoryapi.FactoryEvent,
	eventType, dispatchID string,
) *factoryapi.FactoryEvent {
	for index := range events {
		event := &events[index]
		if string(event.Type) != eventType {
			continue
		}
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		return event
	}
	return nil
}

func isTerminalFactoryDispatchStatus(status factoryapi.FactoryDispatchStatus) bool {
	switch status {
	case factoryapi.FactoryDispatchStatusCOMPLETED,
		factoryapi.FactoryDispatchStatusFAILED,
		factoryapi.FactoryDispatchStatusINTERRUPTED:
		return true
	default:
		return false
	}
}

func assertAPILiveProviderDispatchFailureDetail(
	t *testing.T,
	failure *factoryapi.FailureDetail,
) {
	t.Helper()
	if failure == nil {
		t.Fatalf("failureDetail = %#v, want typed provider failure", failure)
	}
	if failure.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
		t.Fatalf("failure reason = %q, want %q", failure.Reason, workerexecution.WorkFailureTypePermanentBadRequest)
	}
	if failure.Message != "Provider rejected the request as invalid." {
		t.Fatalf("failure message = %#v, want sanitized provider failure", failure.Message)
	}
}

func assertAPIDispatchStatusTransitions(
	t *testing.T,
	got *[]factoryapi.FactoryDispatchStatus,
	want []factoryapi.FactoryDispatchStatus,
) {
	t.Helper()
	if got == nil {
		t.Fatalf("statusTransitions = nil, want %#v", want)
	}
	if len(*got) != len(want) {
		t.Fatalf("statusTransitions = %#v, want %#v", *got, want)
	}
	for index, status := range *got {
		if status != want[index] {
			t.Fatalf("statusTransitions[%d] = %q, want %q", index, status, want[index])
		}
	}
}

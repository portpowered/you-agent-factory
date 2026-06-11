package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestListFactorySessions_ScopedPersistedReturnsFixtureSeeds(t *testing.T) {
	doc := loadOpenAPIContractForServerTests(t)
	srv := newDurableSessionAPITestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=persisted", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions?scope=persisted status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ListFactorySessionsResponse](t, rec)
	assertResponseValidatesOpenAPISchema(t, doc, "ListFactorySessionsResponse", response)
	if response.Scope == nil || *response.Scope != factoryapi.FactorySessionListScopePersisted {
		t.Fatalf("scope = %#v, want persisted", response.Scope)
	}
	if response.DurableSessions == nil || len(*response.DurableSessions) == 0 {
		t.Fatal("durableSessions missing persisted fixture rows")
	}
	if !containsDurableSummaryID(*response.DurableSessions, "dur-sess-petri-success-001") {
		t.Fatalf("durableSessions = %#v, want terminal petri success row", *response.DurableSessions)
	}
	if len(response.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want no live workspace rows", response.Sessions)
	}
}

func TestListFactorySessions_ScopedLiveReturnsStartedRunningSession(t *testing.T) {
	doc := loadOpenAPIContractForServerTests(t)
	scenario := durableStartScenarioByID(t, "javascript-running-n-dispatch")
	body, err := json.Marshal(scenario.ExecutionRequest)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newDurableSessionAPITestServer(t)
	startReq := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202: %s", startRec.Code, startRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=live", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions?scope=live status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ListFactorySessionsResponse](t, rec)
	assertResponseValidatesOpenAPISchema(t, doc, "ListFactorySessionsResponse", response)
	if response.DurableSessions != nil && len(*response.DurableSessions) > 0 {
		t.Fatalf("durableSessions = %#v, want none for live scope", response.DurableSessions)
	}
	if !containsLiveSummaryID(response.Sessions, "dur-sess-js-run-n-001") {
		t.Fatalf("sessions = %#v, want running durable row in live scope", response.Sessions)
	}
}

func TestListFactorySessions_ScopedAllDedupesTerminalFromLiveRows(t *testing.T) {
	scenario := durableStartScenarioByID(t, "petri-succeeded-one-dispatch")
	body, err := json.Marshal(scenario.ExecutionRequest)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newDurableSessionAPITestServer(t)
	startReq := httptest.NewRequest(http.MethodPost, "/factory-sessions/sync", bytes.NewReader(body))
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200: %s", startRec.Code, startRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=all", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions?scope=all status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ListFactorySessionsResponse](t, rec)
	if containsLiveSummaryID(response.Sessions, "dur-sess-petri-success-001") {
		t.Fatalf("live sessions still contain deduped terminal id: %#v", response.Sessions)
	}
	if response.DurableSessions == nil || !containsDurableSummaryID(*response.DurableSessions, "dur-sess-petri-success-001") {
		t.Fatalf("durableSessions = %#v, want terminal row in all scope", response.DurableSessions)
	}
}

func TestListFactorySessions_KeepsLiveWorkspaceRowsSeparateFromDurableRows(t *testing.T) {
	scenario := durableStartScenarioByID(t, "javascript-running-n-dispatch")
	body, err := json.Marshal(scenario.ExecutionRequest)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	mf := &testutil.MockFactory{
		FactorySessions: factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{Id: "live-workspace-alpha", Project: "alpha"},
			},
		},
	}
	srv := newDurableSessionAPITestServerWithFactory(t, mf)
	startReq := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202: %s", startRec.Code, startRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=all", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions?scope=all status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ListFactorySessionsResponse](t, rec)
	if !containsLiveSummaryID(response.Sessions, "live-workspace-alpha") {
		t.Fatalf("sessions = %#v, want live workspace row", response.Sessions)
	}
	if !containsLiveSummaryID(response.Sessions, "dur-sess-js-run-n-001") {
		t.Fatalf("sessions = %#v, want running durable row", response.Sessions)
	}
	if response.DurableSessions == nil {
		t.Fatal("durableSessions missing for all scope")
	}
}

func TestGetFactorySession_ReturnsDurableReadModelForMockScenario(t *testing.T) {
	doc := loadOpenAPIContractForServerTests(t)
	scenario := durableStartScenarioByID(t, "javascript-running-n-dispatch")
	body, err := json.Marshal(scenario.ExecutionRequest)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newDurableSessionAPITestServer(t)
	startReq := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202: %s", startRec.Code, startRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-js-run-n-001", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/{session_id} status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.FactorySessionDurableReadModel](t, rec)
	assertResponseValidatesOpenAPISchema(t, doc, "FactorySessionDurableReadModel", response)
	if response.SessionId != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %q", response.SessionId)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
	if response.Phase == nil || *response.Phase != "verify" {
		t.Fatalf("phase = %#v, want verify", response.Phase)
	}
	if response.Progress == nil || response.Progress.InFlightDispatches == nil || *response.Progress.InFlightDispatches != 1 {
		t.Fatalf("progress = %#v, want one in-flight dispatch", response.Progress)
	}
	if response.Links == nil || response.Links.Session == nil {
		t.Fatalf("links = %#v, want inspection links", response.Links)
	}
}

func TestGetFactorySession_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newDurableSessionAPITestServerWithFactory(t, &testutil.MockFactory{
		GetFactorySessionErr: apisurface.ErrFactorySessionNotFound,
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-missing-999", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if response.Code != factoryapi.NOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", response.Code)
	}
}

func TestGetFactorySession_FallsBackToLiveWorkspaceSession(t *testing.T) {
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:      "session-beta",
			Project: "beta",
		},
		GetFactorySessionErr: nil,
	})
	// Live fallback requires durable read seam unset; use server without durable bindings.
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.FactorySession](t, rec)
	if response.Id != "session-beta" {
		t.Fatalf("id = %q, want session-beta", response.Id)
	}
}

func containsLiveSummaryID(sessions []factoryapi.FactorySessionSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.Id == sessionID {
			return true
		}
	}
	return false
}

func containsDurableSummaryID(sessions []factoryapi.FactorySessionDurableSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.SessionId == sessionID {
			return true
		}
	}
	return false
}

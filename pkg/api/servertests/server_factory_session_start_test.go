package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestStartDurableFactorySessionAsync_ReturnsAcceptedMockScenario(t *testing.T) {
	doc := loadOpenAPIContractForServerTests(t)
	scenario := durableStartScenarioByID(t, "javascript-running-n-dispatch")
	body, err := json.Marshal(scenario.ExecutionRequest)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newDurableSessionAPITestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /factory-sessions/async status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.FactorySessionExecutionResponse](t, rec)
	assertResponseValidatesOpenAPISchema(t, doc, "FactorySessionExecutionResponse", response)
	if response.SessionId != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-run-n-001", response.SessionId)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
	if response.Links == nil || response.Links.Session == nil || *response.Links.Session != "/factory-sessions/dur-sess-js-run-n-001" {
		t.Fatalf("links = %#v, want session inspection link", response.Links)
	}
}

func TestStartDurableFactorySessionSync_ReturnsCompletedAndTimedOutMockScenarios(t *testing.T) {
	doc := loadOpenAPIContractForServerTests(t)
	cases := []struct {
		scenarioID string
		wantOutcome factoryapi.FactorySessionSyncExecutionOutcome
	}{
		{"petri-succeeded-one-dispatch", factoryapi.FactorySessionSyncExecutionOutcomeCompleted},
		{"javascript-sync-timed-out", factoryapi.FactorySessionSyncExecutionOutcomeTimedOut},
	}
	for _, tc := range cases {
		t.Run(tc.scenarioID, func(t *testing.T) {
			scenario := durableStartScenarioByID(t, tc.scenarioID)
			body, err := json.Marshal(scenario.ExecutionRequest)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}

			srv := newDurableSessionAPITestServer(t)
			req := httptest.NewRequest(http.MethodPost, "/factory-sessions/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("POST /factory-sessions/sync status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			response := decodeJSONResponse[factoryapi.FactorySessionSyncExecutionResponse](t, rec)
			assertResponseValidatesOpenAPISchema(t, doc, "FactorySessionSyncExecutionResponse", response)
			if response.SyncOutcome != tc.wantOutcome {
				t.Fatalf("syncOutcome = %q, want %q", response.SyncOutcome, tc.wantOutcome)
			}
			if response.Links == nil || response.Links.Session == nil {
				t.Fatalf("links = %#v, want session inspection link", response.Links)
			}
			if tc.wantOutcome == factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
				if response.Links.Results == nil {
					t.Fatalf("links = %#v, want result inspection link for completed sync", response.Links)
				}
				if response.Result == nil || response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
					t.Fatalf("result = %#v, want FINAL terminal result", response.Result)
				}
			}
			if tc.wantOutcome == factoryapi.FactorySessionSyncExecutionOutcomeTimedOut {
				if response.TimedOut == nil || !*response.TimedOut {
					t.Fatal("timedOut = false, want true")
				}
			}
		})
	}
}

func TestStartDurableFactorySessionAsync_RejectsMissingRequestID(t *testing.T) {
	body, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: stringPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newDurableSessionAPITestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if response.Code != factoryapi.BADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", response.Code)
	}
}

func TestStartDurableFactorySessionAsync_IdempotentReplayAndConflict(t *testing.T) {
	doc := loadOpenAPIContractForServerTests(t)
	catalog := loadDurableStartFixtureCatalog(t)
	body, err := json.Marshal(catalog.IdempotentReplay.ExecutionRequest)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newDurableSessionAPITestServer(t)

	firstReq := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("first start status = %d, want 202: %s", firstRec.Code, firstRec.Body.String())
	}
	first := decodeJSONResponse[factoryapi.FactorySessionExecutionResponse](t, firstRec)
	assertResponseValidatesOpenAPISchema(t, doc, "FactorySessionExecutionResponse", first)

	replayReq := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	replayReq.Header.Set("Content-Type", "application/json")
	replayRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusAccepted {
		t.Fatalf("replay start status = %d, want 202: %s", replayRec.Code, replayRec.Body.String())
	}
	replay := decodeJSONResponse[factoryapi.FactorySessionExecutionResponse](t, replayRec)
	if replay.SessionId != first.SessionId {
		t.Fatalf("replay sessionId = %q, want %q", replay.SessionId, first.SessionId)
	}

	conflictRequest := decodeDurableExecutionRequest(t, catalog.IdempotentReplay.ExecutionRequest)
	conflictRequest.Args = &map[string]any{"task": "different"}
	conflictBody, err := json.Marshal(conflictRequest)
	if err != nil {
		t.Fatalf("marshal conflict request: %v", err)
	}
	conflictReq := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(conflictBody))
	conflictReq.Header.Set("Content-Type", "application/json")
	conflictRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(conflictRec, conflictReq)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409: %s", conflictRec.Code, conflictRec.Body.String())
	}
	conflict := decodeJSONResponse[factoryapi.ErrorResponse](t, conflictRec)
	if conflict.Code != factoryapi.EXECUTIONREQUESTIDCONFLICT {
		t.Fatalf("code = %q, want EXECUTION_REQUEST_ID_CONFLICT", conflict.Code)
	}
}

func TestStartDurableFactorySessionAsync_ReturnsNotImplementedWithoutExecutionSeam(t *testing.T) {
	body, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: stringPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501: %s", rec.Code, rec.Body.String())
	}
}

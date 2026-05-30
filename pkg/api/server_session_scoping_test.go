package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestSessionScopedAPI_ReadsAndMutationsTargetOnlyRequestedSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	defaultFactoryID := "root-runtime"
	betaFactoryID := "beta-runtime"
	defaultSession := newSessionScopedMockFactory(now, &defaultFactoryID, apisurface.DefaultCurrentFactoryName, "tok-default-1", "default-work-1", "factory-event/work-request/default-history")
	betaSession := newSessionScopedMockFactory(now, &betaFactoryID, "beta", "tok-beta-1", "beta-work-1", "factory-event/work-request/beta-history")
	srv := newTestServer(&testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: apisurface.DefaultCurrentFactoryName, Id: &defaultFactoryID},
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":     defaultSession,
			"session-beta": betaSession,
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	assertScopedSessionSubmit(t, server.URL, betaSession, defaultSession)
	assertScopedSessionList(t, server.URL, betaSession, defaultSession)
	assertScopedSessionWorkRead(t, server.URL)
	assertScopedSessionStatus(t, server.URL)
	assertScopedCurrentFactory(t, server.URL, "beta")
	assertScopedSessionEvents(t, server.URL, "factory-event/work-request/beta-history")
}

func newSessionScopedMockFactory(
	now time.Time,
	factoryID *string,
	factoryName string,
	tokenID string,
	workID string,
	historyEventID string,
) *testutil.MockFactory {
	return &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				tokenID: listWorkToken(tokenID, workID, "task:init", "task", now),
			},
		},
		Net: sessionScopedStateNet(),
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{{Id: historyEventID, Type: factoryapi.FactoryEventTypeWorkRequest}},
			Events:  make(chan factoryapi.FactoryEvent),
		},
		CurrentFactory: &factoryapi.Factory{Name: factoryName, Id: factoryID},
	}
}

func sessionScopedStateNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
			"task:done": {ID: "task:done", TypeID: "task", State: "done"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "done", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}

func assertScopedSessionSubmit(t *testing.T, serverURL string, betaSession *testutil.MockFactory, defaultSession *testutil.MockFactory) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodPost, serverURL+"/factory-sessions/session-beta/work", bytes.NewBufferString(`{"name":"scoped-submit","workTypeName":"task","traceId":"trace-scoped-submit","payload":{"title":"scoped"}}`), "application/json", http.StatusCreated)
	defer response.Body.Close()

	if len(betaSession.WorkRequests) != 1 {
		t.Fatalf("beta submitted work requests = %d, want 1", len(betaSession.WorkRequests))
	}
	if len(defaultSession.WorkRequests) != 0 {
		t.Fatalf("default submitted work requests = %d, want 0", len(defaultSession.WorkRequests))
	}
}

func assertScopedSessionList(t *testing.T, serverURL string, betaSession *testutil.MockFactory, defaultSession *testutil.MockFactory) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/work", nil, "", http.StatusOK)
	defer response.Body.Close()

	var listBody factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode scoped list response: %v", err)
	}
	if len(listBody.Results) != 1 || stringValue(listBody.Results[0].WorkId) != "beta-work-1" {
		t.Fatalf("scoped list results = %#v, want beta-work-1", listBody.Results)
	}
	if betaSession.EngineStateSnapshotCalls == 0 {
		t.Fatal("expected scoped GET /work to read the targeted session snapshot")
	}
	if defaultSession.EngineStateSnapshotCalls != 0 {
		t.Fatalf("default session snapshot calls = %d, want 0 after scoped list", defaultSession.EngineStateSnapshotCalls)
	}
}

func assertScopedSessionWorkRead(t *testing.T, serverURL string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/work/tok-beta-1", nil, "", http.StatusOK)
	defer response.Body.Close()
}

func assertScopedSessionStatus(t *testing.T, serverURL string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/status", nil, "", http.StatusOK)
	defer response.Body.Close()
}

func assertScopedCurrentFactory(t *testing.T, serverURL string, wantName string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/factory", nil, "", http.StatusOK)
	defer response.Body.Close()

	var currentBody factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&currentBody); err != nil {
		t.Fatalf("decode scoped current factory response: %v", err)
	}
	if currentBody.Name != wantName {
		t.Fatalf("scoped current factory name = %q, want %s", currentBody.Name, wantName)
	}
}

func assertScopedSessionEvents(t *testing.T, serverURL string, wantEventID string) {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/factory-sessions/session-beta/events", nil)
	if err != nil {
		t.Fatalf("new scoped /events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET /factory-sessions/session-beta/events status = %d, want 200: %s", response.StatusCode, string(body))
	}

	streamed := readSSEFactoryEvent(t, bufio.NewReader(response.Body))
	if streamed.Id != wantEventID {
		t.Fatalf("scoped streamed event id = %q, want %s", streamed.Id, wantEventID)
	}
}

func requireHTTPSuccess(
	t *testing.T,
	method string,
	url string,
	body io.Reader,
	contentType string,
	wantStatus int,
) *http.Response {
	t.Helper()

	var (
		response *http.Response
		err      error
	)
	switch method {
	case http.MethodGet:
		response, err = http.Get(url)
	case http.MethodPost:
		response, err = http.Post(url, contentType, body)
	default:
		request, requestErr := http.NewRequestWithContext(context.Background(), method, url, body)
		if requestErr != nil {
			t.Fatalf("%s %s request: %v", method, url, requestErr)
		}
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		response, err = http.DefaultClient.Do(request)
	}
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	if response.StatusCode != wantStatus {
		bodyBytes, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("%s %s status = %d, want %d: %s", method, url, response.StatusCode, wantStatus, string(bodyBytes))
	}
	return response
}

func TestSessionScopedAPI_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestFactorySessionsAPI_ListFactorySessions(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		FactorySessions: factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{
					FactoryDir: "/workspace/root",
					FolderPath: "/workspace/root",
					Id:         "~default",
					IsDefault:  true,
					Project:    "root",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindDefault,
					},
				},
				{
					FactoryDir: "/workspace/root/beta",
					FolderPath: "/workspace/root",
					Id:         "session-beta",
					IsDefault:  false,
					Project:    "beta",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindNamed,
						Name: stringPointerForAPITest("beta"),
					},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode list factory sessions response: %v", err)
	}
	if len(response.Sessions) != 2 || response.Sessions[1].Id != "session-beta" {
		t.Fatalf("factory sessions = %#v, want default and beta sessions", response.Sessions)
	}
}

func TestFactorySessionsAPI_OpenFactorySession(t *testing.T) {
	mf := &testutil.MockFactory{
		OpenFactorySessionResult: factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Id:         "session-beta",
				IsDefault:  false,
				Project:    "beta",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindNamed,
					Name: stringPointerForAPITest("beta"),
				},
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/fleet","target":{"kind":"named","name":"beta"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(mf.OpenedFactorySessions) != 1 {
		t.Fatalf("opened factory sessions = %d, want 1", len(mf.OpenedFactorySessions))
	}
	if mf.OpenedFactorySessions[0].FolderPath != "/workspace/fleet" {
		t.Fatalf("opened session folder = %q, want /workspace/fleet", mf.OpenedFactorySessions[0].FolderPath)
	}
	if mf.OpenedFactorySessions[0].Target == nil ||
		mf.OpenedFactorySessions[0].Target.Kind != factoryapi.FactorySessionTargetRefKindNamed ||
		mf.OpenedFactorySessions[0].Target.Name == nil ||
		*mf.OpenedFactorySessions[0].Target.Name != "beta" {
		t.Fatalf("opened session target = %#v, want named beta", mf.OpenedFactorySessions[0].Target)
	}
	var response factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode open factory session response: %v", err)
	}
	if response.Session == nil || response.Session.Id != "session-beta" {
		t.Fatalf("open factory session response = %#v, want session-beta", response)
	}
}

func TestFactorySessionsAPI_OpenFactorySession_ValidationTargets(t *testing.T) {
	mf := &testutil.MockFactory{
		OpenFactorySessionErr: apiTestSessionValidationError{
			message: "folder validation failed",
			targets: []factoryapi.FactoryValidationTarget{
				factoryvalidation.FactorySessionFieldTarget("missing", "folderPath", "folder validation failed"),
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/missing","validateOnly":true}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /factory-sessions validation status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode open factory session error response: %v", err)
	}
	if response.Code != factoryapi.BADREQUEST {
		t.Fatalf("open factory session error code = %q, want BAD_REQUEST", response.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("open factory session error targets = %#v, want one target", response.Targets)
	}
	target := (*response.Targets)[0]
	if target.Code != "factory.session.field.missing" ||
		target.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory ||
		target.Subject.Id != "folderPath" ||
		target.Subject.Location != factoryapi.FactoryValidationSubjectLocationReference {
		t.Fatalf("open factory session error target = %#v, want structured folder validation target", target)
	}
}

func TestFactorySessionsAPI_CloseFactorySession(t *testing.T) {
	mf := &testutil.MockFactory{}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /factory-sessions/session-beta status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(mf.ClosedFactorySessions) != 1 || mf.ClosedFactorySessions[0] != "session-beta" {
		t.Fatalf("closed factory sessions = %#v, want [session-beta]", mf.ClosedFactorySessions)
	}
}

type apiTestSessionValidationError struct {
	message string
	targets []factoryapi.FactoryValidationTarget
}

func (e apiTestSessionValidationError) Error() string {
	return e.message
}

func (e apiTestSessionValidationError) ErrorTargets() []factoryapi.FactoryValidationTarget {
	return e.targets
}

func TestFactorySessionsAPI_CloseFactorySession_NotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CloseFactorySessionErr: apisurface.ErrFactorySessionNotFound,
	})

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestGetCurrentFactoryBySessionId_ReturnsSessionDefinitionAndVersion(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				CurrentFactory: &factoryapi.Factory{Name: "alpha"},
				FactoryVersion: defaultVersion,
			},
			"session-2": {
				CurrentFactory: &factoryapi.Factory{Name: "beta"},
				FactoryVersion: sessionVersion,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-2/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET factory status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.Factory
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode factory response: %v", err)
	}
	if response.Name != "beta" || response.Version == nil || *response.Version != sessionVersion {
		t.Fatalf("factory response = %#v, want beta/%#v", response, sessionVersion)
	}
}

func TestSaveCurrentFactoryBySessionId_SubmitsToTargetedSessionOnly(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	defaultFactory := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: "alpha"},
		FactoryVersion: defaultVersion,
	}
	sessionFactory := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: "beta"},
		FactoryVersion: sessionVersion,
	}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":  defaultFactory,
			"session-2": sessionFactory,
		},
	})

	body := `{"name":"beta","version":{"physical":"1970-01-01T00:00:00.000000002Z","logical":2},"workTypes":[],"workstations":[],"workers":[]}`
	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/session-2/factory", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT factory status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(defaultFactory.SavedCurrentFactories) != 0 {
		t.Fatalf("default session save count = %d, want 0", len(defaultFactory.SavedCurrentFactories))
	}
	if len(sessionFactory.SavedCurrentFactories) != 1 {
		t.Fatalf("session save count = %d, want 1", len(sessionFactory.SavedCurrentFactories))
	}
	saved := sessionFactory.SavedCurrentFactories[0]
	if saved.Name != "beta" {
		t.Fatalf("saved factory = %#v, want beta definition", saved)
	}
}

func TestCurrentFactoryBySessionId_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
	})

	getReq := httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session/factory", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	assertJSONError(t, getRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")

	putReq := httptest.NewRequest(http.MethodPut, "/factory-sessions/missing-session/factory", bytes.NewBufferString(`{"name":"beta"}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	assertJSONError(t, putRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

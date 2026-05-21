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
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// backendsizecheck:ignore-function this API integration test keeps the full cross-session assertion path in one place until the harness is split.
func TestSessionScopedAPI_ReadsAndMutationsTargetOnlyRequestedSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	defaultFactoryID := "root-runtime"
	betaFactoryID := "beta-runtime"
	defaultSession := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-default-1": listWorkToken("tok-default-1", "default-work-1", "task:init", "task", now),
			},
		},
		Net: &state.Net{
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
		},
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{{Id: "factory-event/work-request/default-history", Type: factoryapi.FactoryEventTypeWorkRequest}},
			Events:  make(chan factoryapi.FactoryEvent),
		},
		CurrentNamedFactory: &factoryapi.Factory{Name: apisurface.DefaultCurrentFactoryName, Id: &defaultFactoryID},
	}
	betaSession := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-beta-1": listWorkToken("tok-beta-1", "beta-work-1", "task:init", "task", now),
			},
		},
		Net: &state.Net{
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
		},
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{{Id: "factory-event/work-request/beta-history", Type: factoryapi.FactoryEventTypeWorkRequest}},
			Events:  make(chan factoryapi.FactoryEvent),
		},
		CurrentNamedFactory: &factoryapi.Factory{Name: "beta", Id: &betaFactoryID},
	}
	srv := newTestServer(&testutil.MockFactory{
		CurrentNamedFactory: &factoryapi.Factory{Name: apisurface.DefaultCurrentFactoryName, Id: &defaultFactoryID},
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":     defaultSession,
			"session-beta": betaSession,
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	submitResp, err := http.Post(server.URL+"/factories/session-beta/work", "application/json", bytes.NewBufferString(`{"name":"scoped-submit","workTypeName":"task","traceId":"trace-scoped-submit","payload":{"title":"scoped"}}`))
	if err != nil {
		t.Fatalf("POST /factories/session-beta/work: %v", err)
	}
	defer submitResp.Body.Close()
	if submitResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(submitResp.Body)
		t.Fatalf("POST /factories/session-beta/work status = %d, want 201: %s", submitResp.StatusCode, string(body))
	}
	if len(betaSession.WorkRequests) != 1 {
		t.Fatalf("beta submitted work requests = %d, want 1", len(betaSession.WorkRequests))
	}
	if len(defaultSession.WorkRequests) != 0 {
		t.Fatalf("default submitted work requests = %d, want 0", len(defaultSession.WorkRequests))
	}

	listResp, err := http.Get(server.URL + "/factories/session-beta/work")
	if err != nil {
		t.Fatalf("GET /factories/session-beta/work: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(listResp.Body)
		t.Fatalf("GET /factories/session-beta/work status = %d, want 200: %s", listResp.StatusCode, string(body))
	}
	var listBody factoryapi.ListWorkResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
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

	workResp, err := http.Get(server.URL + "/factories/session-beta/work/tok-beta-1")
	if err != nil {
		t.Fatalf("GET /factories/session-beta/work/tok-beta-1: %v", err)
	}
	defer workResp.Body.Close()
	if workResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(workResp.Body)
		t.Fatalf("GET /factories/session-beta/work/tok-beta-1 status = %d, want 200: %s", workResp.StatusCode, string(body))
	}

	statusResp, err := http.Get(server.URL + "/factories/session-beta/status")
	if err != nil {
		t.Fatalf("GET /factories/session-beta/status: %v", err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(statusResp.Body)
		t.Fatalf("GET /factories/session-beta/status status = %d, want 200: %s", statusResp.StatusCode, string(body))
	}

	currentResp, err := http.Get(server.URL + "/factories/session-beta/factory/~current")
	if err != nil {
		t.Fatalf("GET /factories/session-beta/factory/~current: %v", err)
	}
	defer currentResp.Body.Close()
	if currentResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(currentResp.Body)
		t.Fatalf("GET /factories/session-beta/factory/~current status = %d, want 200: %s", currentResp.StatusCode, string(body))
	}
	var currentBody factoryapi.Factory
	if err := json.NewDecoder(currentResp.Body).Decode(&currentBody); err != nil {
		t.Fatalf("decode scoped current factory response: %v", err)
	}
	if currentBody.Name != "beta" {
		t.Fatalf("scoped current factory name = %q, want beta", currentBody.Name)
	}

	eventsReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/factories/session-beta/events", nil)
	if err != nil {
		t.Fatalf("new scoped /events request: %v", err)
	}
	eventsResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET /factories/session-beta/events: %v", err)
	}
	defer eventsResp.Body.Close()
	if eventsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(eventsResp.Body)
		t.Fatalf("GET /factories/session-beta/events status = %d, want 200: %s", eventsResp.StatusCode, string(body))
	}
	streamed := readSSEFactoryEvent(t, bufio.NewReader(eventsResp.Body))
	if streamed.Id != "factory-event/work-request/beta-history" {
		t.Fatalf("scoped streamed event id = %q, want beta history", streamed.Id)
	}
}

func TestSessionScopedAPI_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factories/missing-session/status", nil)
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

func TestFactorySessionsAPI_CloseFactorySession_NotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CloseFactorySessionErr: apisurface.ErrFactorySessionNotFound,
	})

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestGetEditableCurrentFactoryDefinitionByFactoryId_ReturnsSessionDefinitionAndVersion(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				CurrentNamedFactory:    &factoryapi.Factory{Name: "alpha"},
				EditableFactoryVersion: defaultVersion,
			},
			"session-2": {
				CurrentNamedFactory:    &factoryapi.Factory{Name: "beta"},
				EditableFactoryVersion: sessionVersion,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factories/session-2/factory/~current/editable-definition", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET editable-definition status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.EditableFactoryDefinition
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode editable-definition response: %v", err)
	}
	if response.FactoryDefinition.Name != "beta" || response.Version != sessionVersion {
		t.Fatalf("editable-definition response = %#v, want beta/%#v", response, sessionVersion)
	}
}

func TestSaveEditableCurrentFactoryDefinitionByFactoryId_SubmitsToTargetedSessionOnly(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	defaultFactory := &testutil.MockFactory{
		CurrentNamedFactory:    &factoryapi.Factory{Name: "alpha"},
		EditableFactoryVersion: defaultVersion,
	}
	sessionFactory := &testutil.MockFactory{
		CurrentNamedFactory:    &factoryapi.Factory{Name: "beta"},
		EditableFactoryVersion: sessionVersion,
	}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":  defaultFactory,
			"session-2": sessionFactory,
		},
	})

	body := `{"baseVersion":{"physical":"1970-01-01T00:00:00.000000002Z","logical":2},"factoryDefinition":{"name":"beta","workTypes":[],"workstations":[],"workers":[]}}`
	req := httptest.NewRequest(http.MethodPut, "/factories/session-2/factory/~current/editable-definition", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT editable-definition status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(defaultFactory.SavedEditableFactories) != 0 {
		t.Fatalf("default session save count = %d, want 0", len(defaultFactory.SavedEditableFactories))
	}
	if len(sessionFactory.SavedEditableFactories) != 1 {
		t.Fatalf("session save count = %d, want 1", len(sessionFactory.SavedEditableFactories))
	}
	saved := sessionFactory.SavedEditableFactories[0].FactoryDefinition
	if saved.Name != "beta" {
		t.Fatalf("saved factory = %#v, want beta definition", saved)
	}
}

func TestEditableCurrentFactoryDefinitionByFactoryId_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
	})

	getReq := httptest.NewRequest(http.MethodGet, "/factories/missing-session/factory/~current/editable-definition", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	assertJSONError(t, getRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")

	putReq := httptest.NewRequest(http.MethodPut, "/factories/missing-session/factory/~current/editable-definition", bytes.NewBufferString(`{"factoryDefinition":{"name":"beta"}}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	assertJSONError(t, putRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

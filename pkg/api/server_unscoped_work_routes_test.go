package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestUnscopedWorkRoutes_RemovedFromRouter(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
	})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "GET /work", method: http.MethodGet, path: "/work"},
		{name: "POST /work", method: http.MethodPost, path: "/work", body: `{"name":"retired","workTypeName":"task","payload":{"title":"x"}}`},
		{name: "POST /work/staged-files", method: http.MethodPost, path: "/work/staged-files", body: `{"itemType":"document","fileName":"x.txt","mediaType":"text/plain","contentBase64":"eA=="}`},
		{name: "PUT /work-requests/{request_id}", method: http.MethodPut, path: "/work-requests/req-retired", body: `{"works":[]}`},
		{name: "GET /work/{id}", method: http.MethodGet, path: "/work/work-retired"},
		{name: "POST /work/{id}/move", method: http.MethodPost, path: "/work/work-retired/move", body: `{"stateName":"complete"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = bytes.NewBufferString(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s status = %d, want route removed: %s", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSessionScopedWorkRoutes_AcceptValidRequests(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	submitRec := submitWorkRequest(t, srv, `{"name":"scoped-submit","workTypeName":"task","traceId":"trace-scoped","payload":{"title":"scoped"}}`)
	if submitRec.Code != http.StatusCreated {
		t.Fatalf("POST /factory-sessions/~default/work status = %d, want 201: %s", submitRec.Code, submitRec.Body.String())
	}

	listRec := httptest.NewRequest(http.MethodGet, defaultSessionWorkAPIPrefix+"/work", nil)
	listResp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listResp, listRec)
	if listResp.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/~default/work status = %d, want 200: %s", listResp.Code, listResp.Body.String())
	}

	upsertRec := upsertWorkRequest(t, srv, "/work-requests/batch-req-1", `{"requestId":"batch-req-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"batch-work","workTypeName":"task","payload":{"title":"batch"}}]}`)
	if upsertRec.Code != http.StatusOK && upsertRec.Code != http.StatusCreated {
		t.Fatalf("PUT /factory-sessions/~default/work-requests/{id} status = %d, want success: %s", upsertRec.Code, upsertRec.Body.String())
	}
}

func TestSessionScopedWorkRoutes_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			factorysessions.DefaultSessionID: {Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}},
		},
	})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "GET work list", method: http.MethodGet, path: "/factory-sessions/missing-session/work"},
		{name: "POST submit", method: http.MethodPost, path: "/factory-sessions/missing-session/work", body: `{"name":"x","workTypeName":"task","payload":{"title":"x"}}`},
		{name: "POST staged-files", method: http.MethodPost, path: "/factory-sessions/missing-session/work/staged-files", body: `{"itemType":"document","fileName":"x.txt","mediaType":"text/plain","contentBase64":"eA=="}`},
		{name: "PUT work request", method: http.MethodPut, path: "/factory-sessions/missing-session/work-requests/req-1", body: `{"requestId":"req-1","type":"FACTORY_REQUEST_BATCH","works":[]}`},
		{name: "GET work by id", method: http.MethodGet, path: "/factory-sessions/missing-session/work/work-1"},
		{name: "POST move", method: http.MethodPost, path: "/factory-sessions/missing-session/work/work-1/move", body: `{"stateName":"complete"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = bytes.NewBufferString(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
		})
	}
}

package apiserver_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	work "github.com/portpowered/infinite-you/pkg/services/work"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type moveWorkAPI struct {
	moveAndRead func(context.Context, string, string, string, string) (work.ReadModel, error)
}

func (moveWorkAPI) ListWork(context.Context, string, work.ListOptions) (work.ListResult, error) {
	panic("unexpected WorkReadAPI.ListWork call")
}

func (moveWorkAPI) GetWork(context.Context, string, string) (work.ReadModel, error) {
	panic("unexpected WorkReadAPI.GetWork call")
}

func (api moveWorkAPI) MoveWorkAndRead(ctx context.Context, sessionID, workID, stateName, requestID string) (work.ReadModel, error) {
	if api.moveAndRead == nil {
		panic("unexpected WorkReadAPI.MoveWorkAndRead call")
	}
	return api.moveAndRead(ctx, sessionID, workID, stateName, requestID)
}

func TestMoveWork_SucceedsAndReturnsUpdatedWork(t *testing.T) {
	workAPI := successfulMoveWorkAPI(t, "~default", "work-move-1", "complete", "")
	srv := newMoveWorkTestServer(workAPI)

	rec := postMoveWork(t, srv, "work-move-1", `{"stateName":"complete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /work/work-move-1/move status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	work := decodeJSONResponse[factoryapi.Work](t, rec)
	if moveWorkIDString(work.WorkId) != "work-move-1" || work.State == nil || work.State.Name != "complete" {
		t.Fatalf("work = %#v, want work-move-1 at complete", work)
	}
}

func TestMoveWork_AcceptsWhileFactoryPaused(t *testing.T) {
	workAPI := successfulMoveWorkAPI(t, "~default", "work-move-paused", "complete", "")
	srv := newMoveWorkTestServer(workAPI)

	rec := postMoveWork(t, srv, "work-move-paused", `{"stateName":"complete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /work/work-move-paused/move status = %d, want 200 while paused: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns404ForMissingWork(t *testing.T) {
	workAPI := moveWorkAPI{moveAndRead: func(_ context.Context, sessionID, workID, stateName, requestID string) (work.ReadModel, error) {
		assertMoveWorkRequest(t, sessionID, workID, stateName, requestID, "~default", "missing-work", "complete", "")
		return work.ReadModel{}, factory.ErrMoveWorkNotFound
	}}
	srv := newMoveWorkTestServer(workAPI)

	rec := postMoveWork(t, srv, "missing-work", `{"stateName":"complete"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns400ForInvalidState(t *testing.T) {
	workAPI := moveWorkAPI{moveAndRead: func(_ context.Context, sessionID, workID, stateName, requestID string) (work.ReadModel, error) {
		assertMoveWorkRequest(t, sessionID, workID, stateName, requestID, "~default", "work-move-invalid", "nowhere", "")
		return work.ReadModel{}, factory.ErrMoveWorkInvalidState
	}}
	srv := newMoveWorkTestServer(workAPI)

	rec := postMoveWork(t, srv, "work-move-invalid", `{"stateName":"nowhere"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns409ForDuplicateRequestId(t *testing.T) {
	moveCalls := 0
	workAPI := moveWorkAPI{
		moveAndRead: func(_ context.Context, sessionID, workID, stateName, requestID string) (work.ReadModel, error) {
			assertMoveWorkRequest(t, sessionID, workID, stateName, requestID, "~default", "work-move-dup", "complete", "move-req-1")
			moveCalls++
			if moveCalls > 1 {
				return work.ReadModel{}, work.ErrMoveWorkRequestAlreadyApplied
			}
			return detachedMovedWork("work-move-dup", "complete"), nil
		},
	}
	srv := newMoveWorkTestServer(workAPI)

	body := `{"stateName":"complete","requestId":"move-req-1"}`
	rec := postMoveWork(t, srv, "work-move-dup", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("first move status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = postMoveWork(t, srv, "work-move-dup", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second move status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if resp.Code != factoryapi.ErrorResponseCodeMOVEWORKREQUESTALREADYAPPLIED {
		t.Fatalf("error code = %q, want %q", resp.Code, factoryapi.ErrorResponseCodeMOVEWORKREQUESTALREADYAPPLIED)
	}
}

func TestMoveWorkBySessionId_Returns404ForMissingSession(t *testing.T) {
	workAPI := moveWorkAPI{moveAndRead: func(_ context.Context, sessionID, workID, stateName, requestID string) (work.ReadModel, error) {
		assertMoveWorkRequest(t, sessionID, workID, stateName, requestID, "missing-session", "work-1", "complete", "")
		return work.ReadModel{}, apisurface.ErrFactorySessionNotFound
	}}
	srv := newMoveWorkTestServer(workAPI)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/missing-session/work/work-1/move", bytes.NewBufferString(`{"stateName":"complete"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWorkBySessionId_SucceedsForScopedSession(t *testing.T) {
	workAPI := successfulMoveWorkAPI(t, "beta", "work-beta-move", "complete", "")
	srv := newMoveWorkTestServer(workAPI)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/beta/work/work-beta-move/move", bytes.NewBufferString(`{"stateName":"complete"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	work := decodeJSONResponse[factoryapi.Work](t, rec)
	if work.State == nil || work.State.Name != "complete" {
		t.Fatalf("work = %#v, want complete state", work)
	}
}

func postMoveWork(t *testing.T, srv *api.Server, workID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/work/"+workID+"/move", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func moveWorkIDString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func successfulMoveWorkAPI(t *testing.T, wantSessionID, workID, stateName, requestID string) moveWorkAPI {
	t.Helper()
	return moveWorkAPI{
		moveAndRead: func(_ context.Context, sessionID, gotWorkID, gotStateName, gotRequestID string) (work.ReadModel, error) {
			assertMoveWorkRequest(t, sessionID, gotWorkID, gotStateName, gotRequestID, wantSessionID, workID, stateName, requestID)
			return detachedMovedWork(workID, stateName), nil
		},
	}
}

func newMoveWorkTestServer(role moveWorkAPI) *api.Server {
	return api.NewServer(
		nil, nil, nil, nil, role, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
}

func detachedMovedWork(workID, stateName string) work.ReadModel {
	return work.ReadModel{
		CursorID: "detached-" + workID,
		WorkID:   workID,
		State:    &work.State{Name: stateName, Type: work.StateTypeTerminal},
	}
}

func assertMoveWorkRequest(t *testing.T, sessionID, workID, stateName, requestID, wantSessionID, wantWorkID, wantStateName, wantRequestID string) {
	t.Helper()
	if sessionID != wantSessionID || workID != wantWorkID || stateName != wantStateName || requestID != wantRequestID {
		t.Fatalf(
			"move request = session %q work %q state %q request %q, want session %q work %q state %q request %q",
			sessionID, workID, stateName, requestID,
			wantSessionID, wantWorkID, wantStateName, wantRequestID,
		)
	}
}

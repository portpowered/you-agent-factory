package apiserver_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	factoryruntime "github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func TestMoveWork_SucceedsAndReturnsUpdatedWork(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mf := moveWorkMockFactory(now, "work-move-1", "task", "init")
	srv := newAPITestServer(mf)

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
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mf := moveWorkMockFactory(now, "work-move-paused", "task", "init")
	mf.State = interfaces.FactoryStatePaused
	srv := newAPITestServer(mf)

	rec := postMoveWork(t, srv, "work-move-paused", `{"stateName":"complete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /work/work-move-paused/move status = %d, want 200 while paused: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns404ForMissingWork(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net:     moveWorkTestNet(),
	}
	srv := newAPITestServer(mf)

	rec := postMoveWork(t, srv, "missing-work", `{"stateName":"complete"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns400ForInvalidState(t *testing.T) {
	now := time.Now().UTC()
	mf := moveWorkMockFactory(now, "work-move-invalid", "task", "init")
	srv := newAPITestServer(mf)

	rec := postMoveWork(t, srv, "work-move-invalid", `{"stateName":"nowhere"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns409ForDuplicateRequestId(t *testing.T) {
	now := time.Now().UTC()
	mf := moveWorkMockFactory(now, "work-move-dup", "task", "init")
	srv := newAPITestServer(mf)

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
	if resp.Code != factoryapi.MOVEWORKREQUESTALREADYAPPLIED {
		t.Fatalf("error code = %q, want %q", resp.Code, factoryapi.MOVEWORKREQUESTALREADYAPPLIED)
	}
}

func TestMoveWorkBySessionId_Returns404ForMissingSession(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net:     moveWorkTestNet(),
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
				Net:     moveWorkTestNet(),
			},
		},
	}
	srv := newAPITestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/missing-session/work/work-1/move", bytes.NewBufferString(`{"stateName":"complete"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWorkBySessionId_SucceedsForScopedSession(t *testing.T) {
	now := time.Now().UTC()
	beta := moveWorkMockFactory(now, "work-beta-move", "task", "init")
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net:     moveWorkTestNet(),
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
				Net:     moveWorkTestNet(),
			},
			"beta": beta,
		},
	}
	srv := newAPITestServer(mf)

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

func TestMoveWork_IntegrationWithRuntimeFactoryWhilePaused(t *testing.T) {
	f, err := factoryruntime.New(
		factory.WithNet(moveWorkTestNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := f.SubmitWorkRequest(ctx, factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
		WorkID:     "work-runtime-paused",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-paused",
	}})); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	tickable, ok := f.(factoryruntime.TickableFactory)
	if !ok {
		t.Fatal("expected tickable factory")
	}
	if err := tickable.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	srv := api.NewServer(newRuntimeMoveAPISurface(f), 8080, logger)
	rec := postMoveWork(t, srv, "work-runtime-paused", `{"stateName":"complete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

type runtimeMoveAPISurface struct {
	*testutil.MockFactory
	runtime factory.Factory
}

func newRuntimeMoveAPISurface(f factory.Factory) apisurface.APISurface {
	return &runtimeMoveAPISurface{
		MockFactory: &testutil.MockFactory{},
		runtime:     f,
	}
}

func (r *runtimeMoveAPISurface) MoveWork(ctx context.Context, workID, stateName string, source interfaces.WorkStateChangeSource, requestID string) (interfaces.OperatorMoveResult, error) {
	return r.runtime.MoveWork(ctx, workID, stateName, source, requestID)
}

func (r *runtimeMoveAPISurface) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return r.runtime.GetEngineStateSnapshot(ctx)
}

func postMoveWork(t *testing.T, srv *api.Server, workID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/work/"+workID+"/move", bytes.NewBufferString(body))
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

func moveWorkMockFactory(now time.Time, workID, workTypeID, stateName string) *testutil.MockFactory {
	net := moveWorkTestNet()
	placeID := state.PlaceID(workTypeID, stateName)
	return &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-1": {
					ID:      "tok-1",
					PlaceID: placeID,
					Color: interfaces.TokenColor{
						WorkID:     workID,
						WorkTypeID: workTypeID,
					},
					CreatedAt: now,
					EnteredAt: now,
				},
			},
		},
		Net: net,
	}
}

func moveWorkTestNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, place := range wt.GeneratePlaces() {
		places[place.ID] = place
	}
	return &state.Net{
		Places:    places,
		WorkTypes: map[string]*state.WorkType{"task": wt},
	}
}

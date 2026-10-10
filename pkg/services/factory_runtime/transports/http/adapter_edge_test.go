package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimehttpcontrol "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/control"
	runtimehttpdispatch "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/dispatch"
	runtimehttpobservation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/observation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestControlResume_MapsTypedLifecycleFailures(t *testing.T) {
	t.Parallel()

	fake := &runtimeRootFake{
		resume: func(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
			return factoryruntime.ResumeResult{}, factoryruntime.ErrInvalidLifecycleTransition
		},
	}
	adapter := runtimehttpcontrol.NewHandler(fake)

	rec := httptest.NewRecorder()
	adapter.ControlResume(rec, httptest.NewRequest(http.MethodPost, "/control/resume", nil))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "factory runtime invalid lifecycle transition")
}

func TestAcceptDispatchResult_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	adapter := runtimehttpdispatch.NewHandler(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.AcceptDispatchResult(rec, httptest.NewRequest(http.MethodPost, "/runtime/dispatch/accept-result", strings.NewReader("{")))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestMoveWorkBySessionId_RejectsInvalidJSONBody(t *testing.T) {
	t.Parallel()

	adapter := runtimehttpcontrol.NewHandler(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.MoveWorkBySessionId(
		rec,
		httptest.NewRequest(http.MethodPost, "/sessions/session-1/work/work-1/move", strings.NewReader("{")),
		"session-1",
		factoryapi.WorkOrTokenID("work-1"),
	)
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

// The operation handlers retain nil/zero admission outcomes after retiring the facade.
func TestOperationHandlers_UnconfiguredOperationsFailClosed(t *testing.T) {
	t.Parallel()
	control := &runtimehttpcontrol.Handler{}
	dispatch := &runtimehttpdispatch.Handler{}
	observation := &runtimehttpobservation.Handler{}
	for _, test := range []struct {
		name    string
		invoke  func(*httptest.ResponseRecorder)
		message string
	}{
		{"pause", func(w *httptest.ResponseRecorder) {
			control.ControlPause(w, httptest.NewRequest(http.MethodPost, "/control/pause", nil))
		}, "factory runtime service is required"},
		{"resume", func(w *httptest.ResponseRecorder) {
			control.ControlResume(w, httptest.NewRequest(http.MethodPost, "/control/resume", nil))
		}, "factory runtime service is required"},
		{"terminate", func(w *httptest.ResponseRecorder) {
			control.ControlTerminate(w, httptest.NewRequest(http.MethodPost, "/control/terminate", nil))
		}, "factory runtime service is required"},
		{"move work", func(w *httptest.ResponseRecorder) {
			control.MoveWorkBySessionId(w, httptest.NewRequest(http.MethodPost, "/work/move", strings.NewReader(`{"stateName":"complete"}`)), "session-1", "work-1")
		}, "factory runtime service is required"},
		{"plan dispatch", func(w *httptest.ResponseRecorder) {
			dispatch.PlanDispatch(w, httptest.NewRequest(http.MethodPost, "/dispatch/plan", strings.NewReader(`{"dispatchId":"dispatch-1"}`)))
		}, "factory runtime service is required"},
		{"dispatch result", func(w *httptest.ResponseRecorder) {
			dispatch.AcceptDispatchResult(w, httptest.NewRequest(http.MethodPost, "/dispatch/result", strings.NewReader(`{"dispatchId":"dispatch-1"}`)))
		}, "factory runtime service is required"},
		{"status", func(w *httptest.ResponseRecorder) {
			observation.GetStatus(w, httptest.NewRequest(http.MethodGet, "/status", nil))
		}, "failed to observe factory runtime status"},
		{"session status", func(w *httptest.ResponseRecorder) {
			observation.GetStatusBySessionId(w, httptest.NewRequest(http.MethodGet, "/session/status", nil), "session-1")
		}, "failed to observe factory runtime status"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			test.invoke(recorder)
			assertErrorResponse(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", test.message)
		})
	}
}

func TestOperationHandlers_NilResumeFailsClosed(t *testing.T) {
	t.Parallel()
	var handler *runtimehttpcontrol.Handler
	recorder := httptest.NewRecorder()
	handler.InvokeResume(recorder, context.Background())
	assertErrorResponse(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", "factory runtime service is required")
}

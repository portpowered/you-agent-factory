package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAdapter_NilReceiverHelpersFailClosed(t *testing.T) {
	t.Parallel()

	var adapter *Adapter
	if adapter.Root() != nil {
		t.Fatal("Root() on nil adapter must return nil")
	}

	root, err := adapter.runtimeRoot()
	if err == nil || root != nil {
		t.Fatalf("runtimeRoot() = (%v, %v), want (nil, error)", root, err)
	}
}

func TestAdapter_RuntimeRootFailsWhenRootUnset(t *testing.T) {
	t.Parallel()

	adapter := &Adapter{}
	rec := httptest.NewRecorder()
	adapter.invokeControlResume(rec, context.Background())
	assertErrorResponse(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR", "factory runtime service is required")
}

func TestAdapter_CompatibilityHelpersPreserveHTTPOutcomes(t *testing.T) {
	t.Parallel()

	adapter := &Adapter{}

	jsonRecorder := httptest.NewRecorder()
	adapter.writeJSON(jsonRecorder, http.StatusCreated, map[string]string{"message": "created"})
	if jsonRecorder.Code != http.StatusCreated || jsonRecorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("writeJSON response = %d %q, want 201 application/json", jsonRecorder.Code, jsonRecorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(jsonRecorder.Body.String(), `"message":"created"`) {
		t.Fatalf("writeJSON body = %q, want encoded message", jsonRecorder.Body.String())
	}

	errorRecorder := httptest.NewRecorder()
	adapter.writeError(errorRecorder, http.StatusBadRequest, "bad request", "BAD_REQUEST")
	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(errorRecorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode writeError response: %v", err)
	}
	if errorRecorder.Code != http.StatusBadRequest || response.Message != "bad request" || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("writeError response = %d %#v, want bad request response", errorRecorder.Code, response)
	}
}

func TestAdapter_CompatibilityContextHelpersPreserveTerminalOutcomes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !requestContextEnded(ctx) || requestContextEnded(context.Background()) {
		t.Fatal("requestContextEnded must recognize only canceled or expired contexts")
	}
	if !isRequestContextEnded(context.Canceled) || !isRequestContextEnded(context.DeadlineExceeded) || isRequestContextEnded(errors.New("boom")) {
		t.Fatal("isRequestContextEnded must recognize only request terminal errors")
	}

	adapter := &Adapter{}
	canceledRecorder := httptest.NewRecorder()
	if !adapter.writeRuntimeRequestContextOutcome(canceledRecorder, context.Canceled) {
		t.Fatal("canceled request outcome must be handled")
	}
	if canceledRecorder.Body.Len() != 0 {
		t.Fatalf("canceled response body = %q, want empty", canceledRecorder.Body.String())
	}

	deadlineRecorder := httptest.NewRecorder()
	if !adapter.writeRuntimeRequestContextOutcome(deadlineRecorder, context.DeadlineExceeded) {
		t.Fatal("deadline request outcome must be handled")
	}
	if deadlineRecorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("deadline response status = %d, want 504", deadlineRecorder.Code)
	}

	guardRecorder := httptest.NewRecorder()
	if !adapter.guardRuntimeRequestContext(guardRecorder, httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(ctx)) {
		t.Fatal("guardRuntimeRequestContext must handle an already-canceled request")
	}
}

func TestAdapter_CompatibilityRootErrorHelpersPreserveMappings(t *testing.T) {
	t.Parallel()

	adapter := &Adapter{}
	mappedRecorder := httptest.NewRecorder()
	if !adapter.writeRootError(mappedRecorder, runtimeHTTPOperationControl, factoryruntime.ErrInvalidLifecycleTransition) {
		t.Fatal("writeRootError must handle a mapped Runtime failure")
	}
	if mappedRecorder.Code != http.StatusBadRequest || !strings.Contains(mappedRecorder.Body.String(), "factory runtime invalid lifecycle transition") {
		t.Fatalf("mapped root error response = %d %q, want 400 typed failure", mappedRecorder.Code, mappedRecorder.Body.String())
	}

	unmappedRecorder := httptest.NewRecorder()
	if adapter.writeRootError(unmappedRecorder, runtimeHTTPOperationControl, errors.New("boom")) {
		t.Fatal("writeRootError must not handle an unmapped failure")
	}
}

func TestAdapter_NilReceiverControlResumeFailsClosed(t *testing.T) {
	t.Parallel()

	var adapter *Adapter
	recorder := httptest.NewRecorder()
	adapter.invokeControlResume(recorder, context.Background())
	assertErrorResponse(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", "factory runtime service is required")
}

func TestAdapter_ZeroValueObservationOperationsFailClosed(t *testing.T) {
	t.Parallel()
	assertZeroValueOperationsFailClosed(t, []zeroValueOperation{
		{
			name:        "status",
			wantMessage: "failed to observe factory runtime status",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.GetStatus(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
			},
		},
		{
			name:        "session status",
			wantMessage: "failed to observe factory runtime status",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.GetStatusBySessionId(
					recorder,
					httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/status", nil),
					"session-1",
				)
			},
		},
	})
}

func TestAdapter_ZeroValueControlOperationsFailClosed(t *testing.T) {
	t.Parallel()
	assertZeroValueOperationsFailClosed(t, []zeroValueOperation{
		{
			name: "pause",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.ControlPause(recorder, httptest.NewRequest(http.MethodPost, "/control/pause", nil))
			},
		},
		{
			name: "resume",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.ControlResume(recorder, httptest.NewRequest(http.MethodPost, "/control/resume", nil))
			},
		},
		{
			name: "terminate",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.ControlTerminate(recorder, httptest.NewRequest(http.MethodPost, "/control/terminate", nil))
			},
		},
		{
			name: "move work",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.MoveWorkBySessionId(
					recorder,
					httptest.NewRequest(
						http.MethodPost,
						"/factory-sessions/session-1/work/work-1/move",
						strings.NewReader(`{"stateName":"complete"}`),
					),
					"session-1",
					factoryapi.WorkOrTokenID("work-1"),
				)
			},
		},
	})
}

func TestAdapter_ZeroValueDispatchOperationsFailClosed(t *testing.T) {
	t.Parallel()
	assertZeroValueOperationsFailClosed(t, []zeroValueOperation{
		{
			name: "plan dispatch",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.PlanDispatch(
					recorder,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/dispatch/plan",
						strings.NewReader(`{"dispatchId":"dispatch-1"}`),
					),
				)
			},
		},
		{
			name: "accept dispatch result",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.AcceptDispatchResult(
					recorder,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/dispatch/accept-result",
						strings.NewReader(`{"dispatchId":"dispatch-1"}`),
					),
				)
			},
		},
	})
}

func TestAdapter_ZeroValueCheckpointOperationsFailClosed(t *testing.T) {
	t.Parallel()
	assertZeroValueOperationsFailClosed(t, []zeroValueOperation{
		{
			name: "capture checkpoint",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.CaptureCheckpoint(
					recorder,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/capture",
						strings.NewReader(`{"checkpointId":"checkpoint-1"}`),
					),
				)
			},
		},
		{
			name: "load checkpoint",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.LoadCheckpoint(
					recorder,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/load",
						strings.NewReader(`{"checkpointId":"checkpoint-1"}`),
					),
				)
			},
		},
		{
			name: "restore checkpoint",
			invoke: func(adapter *Adapter, recorder *httptest.ResponseRecorder) {
				adapter.RestoreCheckpoint(
					recorder,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/restore",
						strings.NewReader(`{"checkpoint":{"checkpointId":"checkpoint-1"}}`),
					),
				)
			},
		},
	})
}

type zeroValueOperation struct {
	name        string
	wantMessage string
	invoke      func(*Adapter, *httptest.ResponseRecorder)
}

func assertZeroValueOperationsFailClosed(t *testing.T, tests []zeroValueOperation) {
	t.Helper()
	adapter := &Adapter{}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			tc.invoke(adapter, recorder)
			wantMessage := tc.wantMessage
			if wantMessage == "" {
				wantMessage = "factory runtime service is required"
			}
			assertErrorResponse(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", wantMessage)
		})
	}
}

func TestControlResume_MapsTypedLifecycleFailures(t *testing.T) {
	t.Parallel()

	fake := &runtimeRootFake{
		resume: func(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
			return factoryruntime.ResumeResult{}, factoryruntime.ErrInvalidLifecycleTransition
		},
	}
	adapter := NewAdapter(fake)

	rec := httptest.NewRecorder()
	adapter.ControlResume(rec, httptest.NewRequest(http.MethodPost, "/control/resume", nil))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "factory runtime invalid lifecycle transition")
}

func TestAcceptDispatchResult_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.AcceptDispatchResult(rec, httptest.NewRequest(http.MethodPost, "/runtime/dispatch/accept-result", strings.NewReader("{")))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestLoadCheckpoint_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.LoadCheckpoint(rec, httptest.NewRequest(http.MethodPost, "/runtime/checkpoint/load", strings.NewReader("{")))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestRestoreCheckpoint_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.RestoreCheckpoint(rec, httptest.NewRequest(http.MethodPost, "/runtime/checkpoint/restore", strings.NewReader("{")))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestMoveWorkBySessionId_RejectsInvalidJSONBody(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.MoveWorkBySessionId(
		rec,
		httptest.NewRequest(http.MethodPost, "/sessions/session-1/work/work-1/move", strings.NewReader("{")),
		"session-1",
		factoryapi.WorkOrTokenID("work-1"),
	)
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestBindSessionObserver_NoOpsOnNilAdapter(t *testing.T) {
	t.Parallel()

	var adapter *Adapter
	adapter.BindSessionObserver(&sessionObserverFake{})
}

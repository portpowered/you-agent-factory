package http

import (
	"context"
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

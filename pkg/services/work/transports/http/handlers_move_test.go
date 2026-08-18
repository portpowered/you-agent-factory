package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestMoveWorkBySessionId_EncodesFakeRootPostMoveReadModel(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(
			_ context.Context,
			sessionID string,
			workID string,
			stateName string,
			requestID string,
		) (work.ReadModel, error) {
			invoked = true
			if sessionID != "session-1" || workID != "work-1" || stateName != "complete" || requestID != "move-req-1" {
				t.Fatalf(
					"MoveWorkAndRead(%q, %q, %q, %q), want session-1/work-1/complete/move-req-1",
					sessionID,
					workID,
					stateName,
					requestID,
				)
			}
			return work.ReadModel{
				CursorID:     "tok-1",
				WorkID:       "work-1",
				Name:         "Review PRD",
				WorkTypeName: "prd",
				State:        &work.State{Name: "complete", Type: work.StateTypeTerminal},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/work-1/move",
			strings.NewReader(`{"stateName":"complete","requestId":"move-req-1"}`),
		),
		"session-1",
		"work-1",
	)

	if !invoked {
		t.Fatal("MoveWorkBySessionId must invoke the injected Work root")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.Work
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if response.Name != "Review PRD" || response.State == nil || response.State.Name != "complete" {
		t.Fatalf("response = %#v, want encoded post-move work read model", response)
	}
}

func TestMoveWorkBySessionId_RejectsInvalidDecodeBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			invoked = true
			return work.ReadModel{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/work-1/move",
			strings.NewReader(`{"stateName":"complete","extraField":true}`),
		),
		"session-1",
		"work-1",
	)

	if invoked {
		t.Fatal("invalid move request must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_RejectsMissingStateNameBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			invoked = true
			return work.ReadModel{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/work-1/move",
			strings.NewReader(`{"stateName":"   "}`),
		),
		"session-1",
		"work-1",
	)

	if invoked {
		t.Fatal("missing stateName must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "stateName is required") {
		t.Fatalf("response = %d %s, want stateName required", recorder.Code, recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_RejectsEmptySessionBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			invoked = true
			return work.ReadModel{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/%20/work/work-1/move",
			strings.NewReader(`{"stateName":"complete"}`),
		),
		"   ",
		"work-1",
	)

	if invoked {
		t.Fatal("empty session id must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want bad request", recorder.Code, recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_RejectsEmptyWorkIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			invoked = true
			return work.ReadModel{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/%20/move",
			strings.NewReader(`{"stateName":"complete"}`),
		),
		"session-1",
		"   ",
	)

	if invoked {
		t.Fatal("empty work id must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_MapsWorkNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, work.ErrWorkNotFound
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/missing/move",
			strings.NewReader(`{"stateName":"complete"}`),
		),
		"session-1",
		"missing",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("response = %d %s, want work not found", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "work not found") {
		t.Fatalf("response = %s, want work not found message", recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_MapsRuntimeMoveWorkNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, work.ErrMoveWorkNotFound
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/missing/move",
			strings.NewReader(`{"stateName":"complete"}`),
		),
		"session-1",
		"missing",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "work not found") {
		t.Fatalf("response = %d %s, want runtime move work not found", recorder.Code, recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_MapsAlreadyAppliedMove(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, work.ErrMoveWorkRequestAlreadyApplied
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/work-1/move",
			strings.NewReader(`{"stateName":"complete","requestId":"dup-move"}`),
		),
		"session-1",
		"work-1",
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d %s, want conflict", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"MOVE_WORK_REQUEST_ALREADY_APPLIED"`) {
		t.Fatalf("response = %s, want already-applied move code", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response = %s, must not collapse already-applied move into internal error", recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_MapsSessionNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, fmt.Errorf("%w: session-1", apisurface.ErrFactorySessionNotFound)
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/work-1/move",
			strings.NewReader(`{"stateName":"complete"}`),
		),
		"session-1",
		"work-1",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "factory session not found") {
		t.Fatalf("response = %d %s, want factory session not found", recorder.Code, recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_MapsMoveValidationFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		err     error
		message string
	}{
		{name: "invalid state", err: work.ErrMoveWorkInvalidState, message: "invalid target state for work type"},
		{name: "in flight dispatch", err: work.ErrMoveWorkInFlightDispatch, message: "work is in an active dispatch"},
		{name: "engine terminated", err: work.ErrMoveWorkEngineTerminated, message: "engine has terminated"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			adapter := NewAdapter(&rootFake{
				moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
					return work.ReadModel{}, testCase.err
				},
			})
			recorder := httptest.NewRecorder()

			adapter.MoveWorkBySessionId(
				recorder,
				httptest.NewRequest(
					http.MethodPost,
					"/factory-sessions/session-1/work/work-1/move",
					strings.NewReader(`{"stateName":"complete"}`),
				),
				"session-1",
				"work-1",
			)

			body := recorder.Body.String()
			if recorder.Code != http.StatusBadRequest || !strings.Contains(body, testCase.message) {
				t.Fatalf("response = %d %s, want move validation bad request", recorder.Code, body)
			}
			if strings.Contains(body, `"code":"INTERNAL_ERROR"`) {
				t.Fatalf("response = %s, must not collapse move validation into internal error", body)
			}
		})
	}
}

func TestMoveWorkBySessionId_MapsUnmappedRootFailureToInternalError(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		moveWorkAndRead: func(context.Context, string, string, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, fmt.Errorf("unexpected move failure")
		},
	})
	recorder := httptest.NewRecorder()

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/work-1/move",
			strings.NewReader(`{"stateName":"complete"}`),
		),
		"session-1",
		"work-1",
	)

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response = %d %s, want internal error", recorder.Code, recorder.Body.String())
	}
}

func TestMoveWorkBySessionId_DefaultFakeRootReturnsWorkNotFound(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	adapter := NewAdapter(&rootFake{})

	adapter.MoveWorkBySessionId(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/session-1/work/work-1/move",
			strings.NewReader(`{"stateName":"complete"}`),
		),
		"session-1",
		"work-1",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "work not found") {
		t.Fatalf("response = %d %s, want default fake-root work not found", recorder.Code, recorder.Body.String())
	}
}

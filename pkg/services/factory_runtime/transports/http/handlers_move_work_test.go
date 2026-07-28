package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestMoveWorkBySessionId_ForwardsDecodedFieldsToRoot(t *testing.T) {
	t.Parallel()

	var got factoryruntime.MoveWorkRequest
	fake := &runtimeRootFake{
		moveWork: func(_ context.Context, req factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
			got = req
			return factoryruntime.MoveWorkResult{
				WorkID:     req.WorkID,
				WorkTypeID: "task",
				FromState:  "processing",
				ToState:    req.StateName,
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	body := strings.NewReader(`{"stateName":"complete","requestId":"move-req-1"}`)
	rec := httptest.NewRecorder()
	adapter.MoveWorkBySessionId(
		rec,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-alpha/work/work-1/move", body),
		"session-alpha",
		"work-1",
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.WorkID != "work-1" || got.StateName != "complete" || got.RequestID != "move-req-1" {
		t.Fatalf("move request = %#v, want work-1 complete move-req-1", got)
	}
	if got.Source != factoryruntime.WorkMoveSourceAPI {
		t.Fatalf("source = %q, want api", got.Source)
	}

	var response factoryapi.Work
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.WorkId == nil || *response.WorkId != "work-1" {
		t.Fatalf("workId = %#v, want work-1", response.WorkId)
	}
	if response.State == nil || response.State.Name != "complete" {
		t.Fatalf("state = %#v, want complete", response.State)
	}
}

func TestMoveWorkBySessionId_MapsTypedMoveFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		programmed error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "not found",
			programmed: factoryruntime.ErrMoveWorkNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "work not found",
		},
		{
			name:       "invalid state",
			programmed: factoryruntime.ErrMoveWorkInvalidState,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "invalid target state for work type",
		},
		{
			name:       "request conflict",
			programmed: factoryruntime.ErrMoveWorkRequestConflict,
			wantStatus: http.StatusConflict,
			wantCode:   "MOVE_WORK_REQUEST_ALREADY_APPLIED",
			wantMsg:    "Operator move request was already applied.",
		},
		{
			name:       "not running",
			programmed: factoryruntime.ErrNotRunning,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "SERVICE_UNAVAILABLE",
			wantMsg:    "factory runtime is not running",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &runtimeRootFake{
				moveWork: func(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
					return factoryruntime.MoveWorkResult{}, tc.programmed
				},
			}
			adapter := NewAdapter(fake)
			rec := httptest.NewRecorder()
			adapter.MoveWorkBySessionId(
				rec,
				httptest.NewRequest(http.MethodPost, "/factory-sessions/session-alpha/work/work-1/move", strings.NewReader(`{"stateName":"complete"}`)),
				"session-alpha",
				"work-1",
			)
			assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
		})
	}
}

func TestMoveWorkBySessionId_RequiresStateName(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.MoveWorkBySessionId(
		rec,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-alpha/work/work-1/move", strings.NewReader(`{"stateName":"  "}`)),
		"session-alpha",
		"work-1",
	)
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "stateName is required")
}

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestGetStatus_MapsRootObservationToStatusResponse(t *testing.T) {
	t.Parallel()

	var gotScope factoryruntime.ObservationScope
	fake := &runtimeRootFake{
		observe: func(_ context.Context, req factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			gotScope = req.Scope
			return factoryruntime.ObserveResult{Observation: factoryruntime.Observation{
				Status: factoryruntime.ObservationStatusActive,
				Progress: factoryruntime.ObservationProgress{
					TotalWorkCount: 4,
					WorkCategories: factoryruntime.ObservationWorkCategories{
						Initial:    1,
						Processing: 2,
						Terminal:   1,
					},
				},
				Health: factoryruntime.ObservationHealth{FactoryState: "RUNNING"},
			}}, nil
		},
	}
	adapter := NewAdapter(fake)

	rec := httptest.NewRecorder()
	adapter.GetStatus(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if gotScope != factoryruntime.ObservationScopeFull {
		t.Fatalf("Observe scope = %q, want FULL", gotScope)
	}
	var response factoryapi.StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.FactoryState != "RUNNING" || response.RuntimeStatus != "ACTIVE" || response.TotalTokens != 4 ||
		response.Categories.Initial != 1 || response.Categories.Processing != 2 || response.Categories.Terminal != 1 {
		t.Fatalf("response = %#v, want mapped observation status", response)
	}
}

func TestGetStatusBySessionId_ForwardsSessionScopedObservation(t *testing.T) {
	t.Parallel()

	var gotSessionID string
	var gotScope factoryruntime.ObservationScope
	sessions := &sessionObserverFake{
		observe: func(_ context.Context, sessionID string, req factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			gotSessionID = sessionID
			gotScope = req.Scope
			return factoryruntime.ObserveResult{Observation: factoryruntime.Observation{
				Status:   factoryruntime.ObservationStatusActive,
				Progress: factoryruntime.ObservationProgress{TotalWorkCount: 2},
				Health:   factoryruntime.ObservationHealth{FactoryState: "SCOPED"},
			}}, nil
		},
	}
	adapter := NewAdapter(&runtimeRootFake{})
	adapter.BindSessionObserver(sessions)

	rec := httptest.NewRecorder()
	adapter.GetStatusBySessionId(rec, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/status", nil), "session-beta")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if gotSessionID != "session-beta" {
		t.Fatalf("sessionID = %q, want session-beta", gotSessionID)
	}
	if gotScope != factoryruntime.ObservationScopeFull {
		t.Fatalf("Observe scope = %q, want FULL", gotScope)
	}
	var response factoryapi.StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.FactoryState != "SCOPED" || response.TotalTokens != 2 {
		t.Fatalf("response = %#v, want scoped observation projection", response)
	}
}

func TestGetStatusBySessionId_RequiresSessionObserver(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.GetStatusBySessionId(rec, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/status", nil), "session-beta")

	assertErrorResponse(t, rec, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "factory status is unavailable")
}

func TestGetStatus_MapsTypedObservationFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		programmed error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "not running",
			programmed: factoryruntime.ErrNotRunning,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "SERVICE_UNAVAILABLE",
			wantMsg:    "factory runtime is not running",
		},
		{
			name:       "not found",
			programmed: factoryruntime.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "factory runtime target not found",
		},
		{
			name:       "invalid scope",
			programmed: factoryruntime.ErrInvalidObservationScope,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "invalid observation scope",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &runtimeRootFake{
				observe: func(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
					return factoryruntime.ObserveResult{}, tc.programmed
				},
			}
			adapter := NewAdapter(fake)
			rec := httptest.NewRecorder()
			adapter.GetStatus(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
			assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
		})
	}
}

func TestGetStatusBySessionId_MapsSessionNotFound(t *testing.T) {
	t.Parallel()

	sessions := &sessionObserverFake{
		observe: func(context.Context, string, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			return factoryruntime.ObserveResult{}, factorysessions.ErrSessionNotFound
		},
	}
	adapter := NewAdapter(&runtimeRootFake{})
	adapter.BindSessionObserver(sessions)

	rec := httptest.NewRecorder()
	adapter.GetStatusBySessionId(rec, httptest.NewRequest(http.MethodGet, "/factory-sessions/missing/status", nil), "missing")
	assertErrorResponse(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode, wantMsg string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(response.Code) != wantCode {
		t.Fatalf("code = %q, want %q", response.Code, wantCode)
	}
	if response.Message != wantMsg {
		t.Fatalf("message = %q, want %q", response.Message, wantMsg)
	}
}

type sessionObserverFake struct {
	observe func(context.Context, string, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error)
}

func (fake *sessionObserverFake) ObserveForSession(
	ctx context.Context,
	sessionID string,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	if fake.observe != nil {
		return fake.observe(ctx, sessionID, req)
	}
	return factoryruntime.ObserveResult{}, errors.New("unexpected ObserveForSession call")
}

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestControlPause_ForwardsToRootAndEncodesOutcome(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &runtimeRootFake{
		pause: func(_ context.Context, _ factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			invoked = true
			return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
		},
	}
	adapter := NewAdapter(fake)

	rec := httptest.NewRecorder()
	adapter.ControlPause(rec, httptest.NewRequest(http.MethodPost, "/control/pause", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !invoked {
		t.Fatal("ControlPause did not reach injected Runtime root")
	}
	var response runtimeControlHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func TestControlResume_ForwardsToRootAndEncodesNoOpOutcome(t *testing.T) {
	t.Parallel()

	fake := &runtimeRootFake{
		resume: func(_ context.Context, _ factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
			return factoryruntime.ResumeResult{Outcome: factoryruntime.ControlOutcomeNoOp}, nil
		},
	}
	adapter := NewAdapter(fake)

	rec := httptest.NewRecorder()
	adapter.ControlResume(rec, httptest.NewRequest(http.MethodPost, "/control/resume", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response runtimeControlHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.ControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}
}

func TestControlTerminate_ForwardsDecodedReasonToRoot(t *testing.T) {
	t.Parallel()

	var gotReason string
	fake := &runtimeRootFake{
		terminate: func(_ context.Context, req factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
			gotReason = req.Reason
			return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
		},
	}
	adapter := NewAdapter(fake)

	body := strings.NewReader(`{"reason":"operator stop"}`)
	rec := httptest.NewRecorder()
	adapter.ControlTerminate(rec, httptest.NewRequest(http.MethodPost, "/control/terminate", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if gotReason != "operator stop" {
		t.Fatalf("reason = %q, want operator stop", gotReason)
	}
}

func TestControlPause_MapsTypedLifecycleFailures(t *testing.T) {
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
			name:       "invalid transition",
			programmed: factoryruntime.ErrInvalidLifecycleTransition,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "factory runtime invalid lifecycle transition",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &runtimeRootFake{
				pause: func(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
					return factoryruntime.PauseResult{}, tc.programmed
				},
			}
			adapter := NewAdapter(fake)
			rec := httptest.NewRecorder()
			adapter.ControlPause(rec, httptest.NewRequest(http.MethodPost, "/control/pause", nil))
			assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
		})
	}
}

func TestControlTerminate_MapsAlreadyStopped(t *testing.T) {
	t.Parallel()

	fake := &runtimeRootFake{
		terminate: func(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
			return factoryruntime.TerminateResult{}, factoryruntime.ErrAlreadyStopped
		},
	}
	adapter := NewAdapter(fake)

	rec := httptest.NewRecorder()
	adapter.ControlTerminate(rec, httptest.NewRequest(http.MethodPost, "/control/terminate", bytes.NewReader(nil)))
	assertErrorResponse(t, rec, http.StatusConflict, "CONFLICT", "factory runtime is already stopped")
}

func TestControlTerminate_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.ControlTerminate(rec, httptest.NewRequest(http.MethodPost, "/control/terminate", strings.NewReader("{")))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

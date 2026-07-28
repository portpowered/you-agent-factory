package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestPlanDispatch_ForwardsDecodedFieldsToRoot(t *testing.T) {
	t.Parallel()

	var got factoryruntime.PlanDispatchRequest
	fake := &runtimeRootFake{
		planDispatch: func(_ context.Context, req factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
			got = req
			return factoryruntime.PlanDispatchResult{
				Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
				DispatchID:      req.DispatchID,
				CorrelationID: req.CorrelationID,
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	body := strings.NewReader(`{
		"dispatchId":"dispatch-1",
		"correlationId":"corr-1",
		"workIds":["work-1"," work-2 "],
		"workstationName":"ws-alpha",
		"workerType":"agent",
		"replayKey":"replay-1"
	}`)
	rec := httptest.NewRecorder()
	adapter.PlanDispatch(rec, httptest.NewRequest(http.MethodPost, "/runtime/dispatch/plan", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.DispatchID != "dispatch-1" || got.CorrelationID != "corr-1" ||
		got.WorkstationName != "ws-alpha" || got.WorkerType != "agent" || got.ReplayKey != "replay-1" {
		t.Fatalf("plan request = %#v, want decoded dispatch fields", got)
	}
	if len(got.WorkIDs) != 2 || got.WorkIDs[0] != "work-1" || got.WorkIDs[1] != "work-2" {
		t.Fatalf("workIds = %#v, want [work-1 work-2]", got.WorkIDs)
	}

	var response runtimeDispatchPlanHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.DispatchPlanOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func TestPlanDispatch_EncodesDuplicateIdempotentOutcome(t *testing.T) {
	t.Parallel()

	fake := &runtimeRootFake{
		planDispatch: func(_ context.Context, req factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
			return factoryruntime.PlanDispatchResult{
				Outcome:       factoryruntime.DispatchPlanOutcomeDuplicateIdempotent,
				DispatchID:    req.DispatchID,
				CorrelationID: req.CorrelationID,
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	rec := httptest.NewRecorder()
	adapter.PlanDispatch(
		rec,
		httptest.NewRequest(
			http.MethodPost,
			"/runtime/dispatch/plan",
			strings.NewReader(`{"dispatchId":"dispatch-1","correlationId":"corr-1","workIds":["work-1"],"workstationName":"ws","workerType":"agent","replayKey":"replay-1"}`),
		),
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response runtimeDispatchPlanHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("outcome = %q, want DUPLICATE_IDEMPOTENT", response.Outcome)
	}
}

func TestAcceptDispatchResult_ForwardsDecodedFieldsToRoot(t *testing.T) {
	t.Parallel()

	var got factoryruntime.AcceptDispatchResultRequest
	fake := &runtimeRootFake{
		acceptDispatchResult: func(_ context.Context, req factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
			got = req
			return factoryruntime.AcceptDispatchResultResult{
				Outcome:       factoryruntime.DispatchPlanOutcomeRetired,
				DispatchID:    req.DispatchID,
				CorrelationID: req.CorrelationID,
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	body := strings.NewReader(`{
		"dispatchId":"dispatch-1",
		"correlationId":"corr-1",
		"workId":"work-1",
		"resultOutcome":"SUCCESS"
	}`)
	rec := httptest.NewRecorder()
	adapter.AcceptDispatchResult(rec, httptest.NewRequest(http.MethodPost, "/runtime/dispatch/accept-result", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.DispatchID != "dispatch-1" || got.CorrelationID != "corr-1" ||
		got.WorkID != "work-1" || got.ResultOutcome != factoryruntime.DispatchResultOutcomeSuccess {
		t.Fatalf("accept request = %#v, want decoded dispatch result fields", got)
	}

	var response runtimeDispatchPlanHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.DispatchPlanOutcomeRetired {
		t.Fatalf("outcome = %q, want RETIRED", response.Outcome)
	}
}

func TestPlanDispatch_MapsTypedDispatchFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		programmed error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "duplicate intent",
			programmed: factoryruntime.ErrDuplicateDispatchIntent,
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
			wantMsg:    "dispatch intent conflicts with an existing plan",
		},
		{
			name:       "invalid boundary",
			programmed: factoryruntime.ErrInvalidDispatchResultBoundary,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "invalid dispatch result boundary",
		},
		{
			name:       "not found",
			programmed: factoryruntime.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "factory runtime target not found",
		},
		{
			name:       "not running",
			programmed: factoryruntime.ErrNotRunning,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "SERVICE_UNAVAILABLE",
			wantMsg:    "factory runtime is not running",
		},
		{
			name:       "capability unavailable",
			programmed: factoryruntime.ErrCapabilityUnavailable,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "SERVICE_UNAVAILABLE",
			wantMsg:    "factory runtime capability is unavailable",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &runtimeRootFake{
				planDispatch: func(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
					return factoryruntime.PlanDispatchResult{}, tc.programmed
				},
			}
			adapter := NewAdapter(fake)
			rec := httptest.NewRecorder()
			adapter.PlanDispatch(
				rec,
				httptest.NewRequest(
					http.MethodPost,
					"/runtime/dispatch/plan",
					strings.NewReader(`{"dispatchId":"dispatch-1","correlationId":"corr-1","workIds":["work-1"],"workstationName":"ws","workerType":"agent","replayKey":"replay-1"}`),
				),
			)
			assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
		})
	}
}

func TestAcceptDispatchResult_MapsTypedDispatchFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		programmed error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "unknown correlation",
			programmed: factoryruntime.ErrUnknownDispatchCorrelation,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "dispatch correlation not found",
		},
		{
			name:       "invalid boundary",
			programmed: factoryruntime.ErrInvalidDispatchResultBoundary,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "invalid dispatch result boundary",
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
				acceptDispatchResult: func(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
					return factoryruntime.AcceptDispatchResultResult{}, tc.programmed
				},
			}
			adapter := NewAdapter(fake)
			rec := httptest.NewRecorder()
			adapter.AcceptDispatchResult(
				rec,
				httptest.NewRequest(
					http.MethodPost,
					"/runtime/dispatch/accept-result",
					strings.NewReader(`{"dispatchId":"dispatch-1","correlationId":"corr-1","workId":"work-1","resultOutcome":"SUCCESS"}`),
				),
			)
			assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
		})
	}
}

func TestPlanDispatch_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.PlanDispatch(rec, httptest.NewRequest(http.MethodPost, "/runtime/dispatch/plan", strings.NewReader("{")))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestHandlerReturnsAPIReportWithoutPartialOutput(t *testing.T) {
	t.Parallel()
	amount := "0"
	query := costs.CostsQuery(func(_ context.Context, request costs.QueryRequest) (costs.Report, error) {
		if request.FactorySessionID != "session-a" {
			t.Fatalf("FactorySessionID = %q, want session-a", request.FactorySessionID)
		}
		return costs.Report{
			Scope:           costs.Scope{Kind: costs.ScopeFactorySession, FactorySessionID: "session-a"},
			Currency:        "USD",
			Status:          costs.StatusPriced,
			PricedSubtotal:  &amount,
			LineItems:       []costs.LineItem{},
			WorkItems:       []costs.Rollup{},
			WorkerSessions:  []costs.Rollup{},
			ProviderModels:  []costs.ProviderModelRollup{},
			FactorySessions: []costs.Rollup{},
		}, nil
	})
	handler := NewHandler(NewAdapter(query, "metrics", "settings"), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics/costs?session_id=session-a", nil)
	handler.GetMetricsCosts(recorder, request, factoryapi.GetMetricsCostsParams{SessionId: stringPointer("session-a")})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var report factoryapi.CostsReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if report.PricedSubtotal == nil || *report.PricedSubtotal != "0" || report.Status != factoryapi.CostsReportStatus("PRICED") {
		t.Fatalf("report = %#v", report)
	}
}

func TestHandlerMapsInvalidCostsRequestToBadRequest(t *testing.T) {
	t.Parallel()
	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		return costs.Report{}, &costs.QueryError{Kind: costs.QueryErrorInvalidInput, Message: "metrics root is required"}
	})
	handler := NewHandler(NewAdapter(query, "", ""), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetMetricsCosts(recorder, httptest.NewRequest(http.MethodGet, "/metrics/costs", nil), factoryapi.GetMetricsCostsParams{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCode("COSTS_INVALID_REQUEST") || response.Message != "metrics root is required" {
		t.Fatalf("error response = %#v", response)
	}
}

func TestHandlerMapsCostsFailureToInternalError(t *testing.T) {
	t.Parallel()
	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		return costs.Report{}, &costs.QueryError{Kind: costs.QueryErrorMetricsFailed, Message: "runtime metrics query failed", Cause: errors.New("fixture")}
	})
	handler := NewHandler(NewAdapter(query, "metrics", "settings"), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetMetricsCosts(recorder, httptest.NewRequest(http.MethodGet, "/metrics/costs", nil), factoryapi.GetMetricsCostsParams{})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCode("COSTS_QUERY_FAILED") || response.Message != "runtime metrics query failed" {
		t.Fatalf("error response = %#v", response)
	}
}

func stringPointer(value string) *string { return &value }

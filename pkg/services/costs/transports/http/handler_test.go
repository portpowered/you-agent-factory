package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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

func TestHandlerCharacterizesSlowCostsQueryAtLowAndConcurrentLoad(t *testing.T) {
	t.Parallel()

	t.Run("low load", func(t *testing.T) {
		runBlockedCostsLoad(t, 1)
	})
	t.Run("representative concurrent load", func(t *testing.T) {
		runBlockedCostsLoad(t, 16)
	})
}

// runBlockedCostsLoad is a characterization harness for the pre-bound
// metrics-costs route. The gate models a canonical metrics read that has not
// completed; channels make the observation deterministic without sleeps, a
// live daemon, or assumptions about artifact layout. Story 004 can retain
// this harness while changing the assertion from "waits for the query" to a
// bounded timeout/cancellation result.
func runBlockedCostsLoad(t *testing.T, requestCount int) {
	t.Helper()
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseQuery := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseQuery()
	started := make(chan struct{}, requestCount)
	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		started <- struct{}{}
		<-release
		return costs.Report{Status: costs.StatusNoUsage}, nil
	})
	handler := NewHandler(NewAdapter(query, "metrics", "settings"), zap.NewNop())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.GetMetricsCosts(w, r, factoryapi.GetMetricsCostsParams{})
	}))
	defer server.Close()

	type result struct {
		status int
		err    error
	}
	results := make(chan result, requestCount)
	for i := 0; i < requestCount; i++ {
		go func() {
			response, err := server.Client().Get(server.URL + "/metrics/costs")
			item := result{err: err}
			if response != nil {
				item.status = response.StatusCode
				_ = response.Body.Close()
			}
			results <- item
		}()
	}

	for i := 0; i < requestCount; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("request %d did not reach the canonical costs query", i+1)
		}
	}
	select {
	case got := <-results:
		t.Fatalf("costs request completed before the canonical query was released: %#v", got)
	default:
	}

	releaseQuery()
	completionDeadline := time.After(time.Second)
	for i := 0; i < requestCount; i++ {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatalf("costs request %d error after release: %v", i+1, got.err)
			}
			if got.status != http.StatusOK {
				t.Fatalf("costs request %d status = %d, want %d", i+1, got.status, http.StatusOK)
			}
		case <-completionDeadline:
			t.Fatalf("costs request %d did not complete after the canonical query was released", i+1)
		}
	}
}

func stringPointer(value string) *string { return &value }

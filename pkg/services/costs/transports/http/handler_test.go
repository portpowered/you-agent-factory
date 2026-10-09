package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	handler := NewHandler(NewAdapter(query, staticRuntimePaths("metrics", "settings"), identityScopeResolver()), zap.NewNop())
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
	handler := NewHandler(NewAdapter(query, staticRuntimePaths("", ""), identityScopeResolver()), zap.NewNop())
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

func TestHandlerMapsUnknownSelectorToTypedNotFound(t *testing.T) {
	t.Parallel()

	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		t.Fatal("Costs query invoked for an unknown selector")
		return costs.Report{}, nil
	})
	resolver := metricsScopeResolverFunc(func(context.Context, string) (factorysessions.RuntimeMetricsScope, error) {
		return factorysessions.RuntimeMetricsScope{}, factorysessions.ErrSessionNotFound
	})
	handler := NewHandler(NewAdapter(query, staticRuntimePaths("metrics", "settings"), resolver), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetMetricsCosts(
		recorder,
		httptest.NewRequest(http.MethodGet, "/metrics/costs?session_id=missing-live-id", nil),
		factoryapi.GetMetricsCostsParams{SessionId: stringPointer("missing-live-id")},
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCode(costsSessionNotFoundCode) ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		!strings.Contains(response.Message, "you session list --scope live") {
		t.Fatalf("error response = %#v, want typed actionable not-found", response)
	}
}

func TestHandlerMapsCostsFailureToInternalError(t *testing.T) {
	t.Parallel()
	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		return costs.Report{}, &costs.QueryError{Kind: costs.QueryErrorMetricsFailed, Message: "runtime metrics query failed", Cause: errors.New("fixture")}
	})
	handler := NewHandler(NewAdapter(query, staticRuntimePaths("metrics", "settings"), identityScopeResolver()), zap.NewNop())
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

func TestHandlerDeadlineIsTypedAndRequestsRemainIndependent(t *testing.T) {
	t.Parallel()

	t.Run("single request", func(t *testing.T) {
		runTimedOutCostsRequests(t, 1)
	})
	t.Run("independent concurrent requests", func(t *testing.T) {
		runTimedOutCostsRequests(t, 2)
	})
}

// runTimedOutCostsRequests calls the owned Handler with independent request contexts.
func runTimedOutCostsRequests(t *testing.T, requestCount int) {
	t.Helper()
	const queryTimeout = 25 * time.Millisecond
	started := make(chan struct{}, requestCount)
	query := costs.CostsQuery(func(ctx context.Context, _ costs.QueryRequest) (costs.Report, error) {
		started <- struct{}{}
		<-ctx.Done()
		return costs.Report{}, ctx.Err()
	})
	handler := NewHandlerWithQueryTimeout(NewAdapter(query, staticRuntimePaths("metrics", "settings"), identityScopeResolver()), zap.NewNop(), queryTimeout)
	results := make(chan timedCostsResult, requestCount)
	for i := 0; i < requestCount; i++ {
		go func() {
			recorder := httptest.NewRecorder()
			handler.GetMetricsCosts(recorder, httptest.NewRequest(http.MethodGet, "/metrics/costs", nil), factoryapi.GetMetricsCostsParams{})
			results <- timedCostsResult{status: recorder.Code, body: recorder.Body.Bytes()}
		}()
	}
	awaitTimedCostsQueries(t, started, requestCount)
	assertTimedCostsResults(t, results, requestCount, queryTimeout)
}

type timedCostsResult struct {
	status int
	err    error
	body   []byte
}

func awaitTimedCostsQueries(t *testing.T, started <-chan struct{}, requestCount int) {
	t.Helper()
	for i := 0; i < requestCount; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("request %d did not reach the canonical costs query", i+1)
		}
	}
}

func assertTimedCostsResults(t *testing.T, results <-chan timedCostsResult, requestCount int, queryTimeout time.Duration) {
	t.Helper()
	completionDeadline := time.After(2 * time.Second)
	for i := 0; i < requestCount; i++ {
		got, completed := awaitTimedCostsResult(results, completionDeadline)
		if !completed {
			t.Fatalf("costs request %d did not complete after the server timeout", i+1)
		}
		assertTimedCostsResult(t, got, i+1, queryTimeout)
	}
}

func awaitTimedCostsResult(results <-chan timedCostsResult, deadline <-chan time.Time) (timedCostsResult, bool) {
	select {
	case result := <-results:
		return result, true
	case <-deadline:
		return timedCostsResult{}, false
	}
}

func assertTimedCostsResult(t *testing.T, got timedCostsResult, requestNumber int, queryTimeout time.Duration) {
	t.Helper()
	if got.err != nil {
		t.Fatalf("costs request %d error after server timeout: %v", requestNumber, got.err)
	}
	if got.status != http.StatusGatewayTimeout {
		t.Fatalf("costs request %d status = %d, want %d", requestNumber, got.status, http.StatusGatewayTimeout)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(got.body, &response); err != nil {
		t.Fatalf("costs request %d timeout body: %v", requestNumber, err)
	}
	if response.Code != factoryapi.ErrorResponseCode("COSTS_QUERY_TIMEOUT") || !strings.Contains(response.Message, queryTimeout.String()) {
		t.Fatalf("costs request %d timeout response = %#v, want typed timeout with %s", requestNumber, response, queryTimeout)
	}
	if strings.Contains(string(got.body), "line_items") || strings.Contains(string(got.body), "priced_subtotal") {
		t.Fatalf("costs request %d emitted partial report content: %s", requestNumber, got.body)
	}
}

func TestHandlerMapsCanceledCostsQueryToRequestTimeout(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	query := costs.CostsQuery(func(ctx context.Context, _ costs.QueryRequest) (costs.Report, error) {
		close(started)
		<-ctx.Done()
		return costs.Report{}, ctx.Err()
	})
	handler := NewHandlerWithQueryTimeout(NewAdapter(query, staticRuntimePaths("metrics", "settings"), identityScopeResolver()), zap.NewNop(), time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/metrics/costs", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.GetMetricsCosts(recorder, request, factoryapi.GetMetricsCostsParams{})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("costs query did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("costs handler did not complete after cancellation")
	}

	if recorder.Code != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestTimeout)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode cancellation response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCode("COSTS_QUERY_CANCELED") || !strings.Contains(response.Message, "canceled") {
		t.Fatalf("cancellation response = %#v, want actionable typed cancellation", response)
	}
}

func stringPointer(value string) *string { return &value }

func TestHandlerLogsEncodingFailureThroughInjectedLogger(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core).With(zap.String("factory_session_id", "selected-session"))
	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		return costs.Report{Status: costs.StatusNoUsage}, nil
	})
	handler := NewHandler(NewAdapter(query, staticRuntimePaths("metrics", "settings"), identityScopeResolver()), logger)
	writer := &failedCostsWriter{ResponseRecorder: httptest.NewRecorder()}
	handler.GetMetricsCosts(writer, httptest.NewRequest(http.MethodGet, "/metrics/costs", nil), factoryapi.GetMetricsCostsParams{})
	entries := logs.All()
	if len(entries) != 1 || entries[0].Message != "encode Costs response failed" || entries[0].ContextMap()["factory_session_id"] != "selected-session" {
		t.Fatalf("logs=%#v", entries)
	}
	if writer.Code != http.StatusOK || writer.writes != 1 || writer.Body.Len() != 0 {
		t.Fatalf("encoding failure appended another response: %#v", writer)
	}
}

type failedCostsWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *failedCostsWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("controlled write failure")
}

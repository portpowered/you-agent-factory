package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type metricsScopeResolverFunc func(string) (MetricsSessionScope, error)

func (resolver metricsScopeResolverFunc) ResolveMetricsSessionScope(
	_ context.Context,
	sessionID string,
) (MetricsSessionScope, error) {
	return resolver(sessionID)
}

func TestMetricsHandlerResolvesPublicSessionBeforeQueryAndPreservesEmptyReport(t *testing.T) {
	var gotRequest factoryvisualization.RuntimeMetricsQueryRequest
	query := factoryvisualization.RuntimeMetricsQuery(func(
		_ context.Context,
		request factoryvisualization.RuntimeMetricsQueryRequest,
	) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		gotRequest = request
		return factoryvisualization.RuntimeMetricsQueryResult{}, nil
	})
	resolver := metricsScopeResolverFunc(func(sessionID string) (MetricsSessionScope, error) {
		if sessionID != "public-live-id" {
			t.Fatalf("resolver session ID = %q, want public-live-id", sessionID)
		}
		return MetricsSessionScope{
			RequestedID: "public-live-id",
			RetainedIDs: []string{"runtime-scope-id", "logical-scope-id"},
		}, nil
	})
	handler := NewMetricsHandler(
		NewMetricsAdapter(query, resolver, "/tmp/metrics"),
		zap.NewNop(),
	)

	request := httptest.NewRequest(http.MethodGet, "/metrics?session_id=public-live-id", nil)
	recorder := httptest.NewRecorder()
	handler.GetMetrics(recorder, request, factoryapi.GetMetricsParams{SessionId: stringPointer("public-live-id")})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if gotRequest.SessionID != "runtime-scope-id" {
		t.Fatalf("query effective session ID = %q, want runtime-scope-id", gotRequest.SessionID)
	}
	if len(gotRequest.SessionIDs) != 2 || gotRequest.SessionIDs[0] != "runtime-scope-id" || gotRequest.SessionIDs[1] != "logical-scope-id" {
		t.Fatalf("query retained IDs = %#v, want resolved candidates", gotRequest.SessionIDs)
	}
	var report factoryapi.MetricsReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode metrics report: %v", err)
	}
	if report.Scope.Kind != "FACTORY_SESSION" || report.Scope.FactorySessionId == nil || *report.Scope.FactorySessionId != "public-live-id" {
		t.Fatalf("report scope = %#v, want public live scope", report.Scope)
	}
	if report.Workstations == nil || report.WorkerTypes == nil || report.Providers == nil || report.UsageRows == nil {
		t.Fatalf("empty report arrays must be explicit: %#v", report)
	}
}

func TestMetricsHandlerMapsTypedSessionScopeFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		wantCode   factoryapi.ErrorResponseCode
		wantPhrase string
	}{
		{
			name:       "unknown public session",
			err:        NewMetricsSessionNotFoundError("missing-live-id", nil),
			status:     http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCode(MetricsSessionNotFoundCode),
			wantPhrase: "you session list --scope live",
		},
		{
			name:       "known session without retained scope",
			err:        NewMetricsScopeUnavailableError("known-live-id", nil),
			status:     http.StatusServiceUnavailable,
			wantCode:   factoryapi.ErrorResponseCode(MetricsScopeUnavailableCode),
			wantPhrase: "you session list --scope live",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
				t.Fatal("query invoked after scope resolution failed")
				return factoryvisualization.RuntimeMetricsQueryResult{}, nil
			})
			resolver := metricsScopeResolverFunc(func(string) (MetricsSessionScope, error) { return MetricsSessionScope{}, test.err })
			handler := NewMetricsHandler(NewMetricsAdapter(query, resolver, "/tmp/metrics"), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.GetMetrics(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{SessionId: stringPointer("selected-live-id")})

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != test.wantCode || !strings.Contains(response.Message, test.wantPhrase) {
				t.Fatalf("error response = %#v, want code %q and phrase %q", response, test.wantCode, test.wantPhrase)
			}
		})
	}
}

func TestMetricsHandlerAllowsValidEmptyUnscopedReport(t *testing.T) {
	query := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		return factoryvisualization.RuntimeMetricsQueryResult{}, nil
	})
	handler := NewMetricsHandler(NewMetricsAdapter(query, nil, "/tmp/metrics"), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetMetrics(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func stringPointer(value string) *string { return &value }

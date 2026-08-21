package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestServerRoutesAuthoredMetricsToVisualizationHandler(t *testing.T) {
	query := factoryvisualization.RuntimeMetricsQuery(func(
		_ context.Context,
		request factoryvisualization.RuntimeMetricsQueryRequest,
	) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		if request.SessionID != "retained-runtime-id" || len(request.SessionIDs) != 1 || request.SessionIDs[0] != "retained-runtime-id" {
			t.Fatalf("metrics query request = %#v, want resolved live scope", request)
		}
		return factoryvisualization.RuntimeMetricsQueryResult{}, nil
	})
	resolver := metricsRouteScopeResolver(func(string) (factoryvisualizationhttp.MetricsSessionScope, error) {
		return factoryvisualizationhttp.MetricsSessionScope{RetainedIDs: []string{"retained-runtime-id"}}, nil
	})
	metricsHandler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, resolver, "/metrics-root"),
		zap.NewNop(),
	)
	server := NewServerWithRecordingsAndMetricsAndCosts(
		nil, nil, nil, nil, nil, nil, zap.NewNop(), metricsHandler, nil,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics?session_id=live-public-id", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var report factoryapi.MetricsReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode routed metrics report: %v", err)
	}
	if report.Scope.FactorySessionId == nil || *report.Scope.FactorySessionId != "live-public-id" {
		t.Fatalf("routed report scope = %#v, want live-public-id", report.Scope)
	}
}

type metricsRouteScopeResolver func(string) (factoryvisualizationhttp.MetricsSessionScope, error)

func (resolver metricsRouteScopeResolver) ResolveMetricsSessionScope(
	_ context.Context,
	sessionID string,
) (factoryvisualizationhttp.MetricsSessionScope, error) {
	return resolver(sessionID)
}

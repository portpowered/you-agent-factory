package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_ActivateInvokesVisualizationRoot(t *testing.T) {
	t.Parallel()

	root := &httpVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	result, err := handler.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !root.activateInvoked {
		t.Fatal("Activate was not invoked through the injected Visualization root")
	}
	if result.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("result.State = %q, want STARTED", result.State)
	}
}

func TestHandlerFromRoot_ActivateRequiresInjectedRoot(t *testing.T) {
	t.Parallel()

	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{},
		zap.NewNop(),
	)

	_, err := handler.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err == nil {
		t.Fatal("Activate without injected root = nil, want error")
	}
}

type httpVisualizationRootFake struct {
	activateInvoked bool
}

var _ factoryvisualization.Root = (*httpVisualizationRootFake)(nil)

func (fake *httpVisualizationRootFake) Activate(
	_ context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	fake.activateInvoked = true
	if req.Mode == "" {
		return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
			Kind:    factoryvisualization.LifecycleErrorMissingParameters,
			Message: "activate Factory visualization: required request parameters are missing",
		}
	}
	return factoryvisualization.ActivateResult{
		State: factoryvisualization.LifecycleStateStarted,
	}, nil
}

func (fake *httpVisualizationRootFake) Join(
	context.Context,
	factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	panic("unexpected Join call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	panic("unexpected StopDrain call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) Observe(
	context.Context,
	factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	panic("unexpected Observe call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) OpenPresentation(
	context.Context,
	factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	panic("unexpected OpenPresentation call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	panic("unexpected PresentProgress call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) FinalizePresentation(
	context.Context,
	factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	panic("unexpected FinalizePresentation call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) ClosePresentation(
	context.Context,
	factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	panic("unexpected ClosePresentation call in HTTP adapter root seam test")
}

type metricsScopeResolverFunc func(string) (factoryvisualizationhttp.MetricsSessionScope, error)

func (resolver metricsScopeResolverFunc) ResolveMetricsSessionScope(
	_ context.Context,
	sessionID string,
) (factoryvisualizationhttp.MetricsSessionScope, error) {
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
	resolver := metricsScopeResolverFunc(func(sessionID string) (factoryvisualizationhttp.MetricsSessionScope, error) {
		if sessionID != "public-live-id" {
			t.Fatalf("resolver session ID = %q, want public-live-id", sessionID)
		}
		return factoryvisualizationhttp.MetricsSessionScope{
			RequestedID: "public-live-id",
			RetainedIDs: []string{"runtime-scope-id", "logical-scope-id"},
		}, nil
	})
	handler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, resolver, "/tmp/metrics"),
		zap.NewNop(),
	)

	request := httptest.NewRequest(http.MethodGet, "/metrics?session_id=public-live-id", nil)
	recorder := httptest.NewRecorder()
	handler.GetMetrics(recorder, request, factoryapi.GetMetricsParams{SessionId: stringPointer("public-live-id")})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if gotRequest.SessionID != "runtime-scope-id" || len(gotRequest.SessionIDs) != 2 || gotRequest.SessionIDs[1] != "logical-scope-id" {
		t.Fatalf("metrics query request = %#v, want resolved retained IDs", gotRequest)
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
			err:        factoryvisualizationhttp.NewMetricsSessionNotFoundError("missing-live-id", nil),
			status:     http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCode(factoryvisualizationhttp.MetricsSessionNotFoundCode),
			wantPhrase: "you session list --scope live",
		},
		{
			name:       "known session without retained scope",
			err:        factoryvisualizationhttp.NewMetricsScopeUnavailableError("known-live-id", nil),
			status:     http.StatusServiceUnavailable,
			wantCode:   factoryapi.ErrorResponseCode(factoryvisualizationhttp.MetricsScopeUnavailableCode),
			wantPhrase: "you session list --scope live",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
				t.Fatal("query invoked after scope resolution failed")
				return factoryvisualization.RuntimeMetricsQueryResult{}, nil
			})
			resolver := metricsScopeResolverFunc(func(string) (factoryvisualizationhttp.MetricsSessionScope, error) {
				return factoryvisualizationhttp.MetricsSessionScope{}, test.err
			})
			handler := factoryvisualizationhttp.NewMetricsHandler(factoryvisualizationhttp.NewMetricsAdapter(query, resolver, "/tmp/metrics"), zap.NewNop())
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
	handler := factoryvisualizationhttp.NewMetricsHandler(factoryvisualizationhttp.NewMetricsAdapter(query, nil, "/tmp/metrics"), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetMetrics(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func stringPointer(value string) *string { return &value }

package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

type metricsScopeResolverFunc func(string) (factorysessions.RuntimeMetricsScope, error)

func (resolver metricsScopeResolverFunc) ResolveRuntimeMetricsScope(
	_ context.Context,
	sessionID string,
) (factorysessions.RuntimeMetricsScope, error) {
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
	resolver := metricsScopeResolverFunc(func(sessionID string) (factorysessions.RuntimeMetricsScope, error) {
		if sessionID != "public-live-id" {
			t.Fatalf("resolver session ID = %q, want public-live-id", sessionID)
		}
		return factorysessions.RuntimeMetricsScope{
			RequestedFactorySessionID: "public-live-id",
			RetainedFactorySessionIDs: []string{"runtime-scope-id", "logical-scope-id"},
		}, nil
	})
	handler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, resolver, staticMetricsRoot("/tmp/metrics")),
		zap.NewNop(),
	)

	request := httptest.NewRequest(http.MethodGet, "/metrics?session_id=public-live-id", nil)
	recorder := httptest.NewRecorder()
	handler.GetMetrics(recorder, request, factoryapi.GetMetricsParams{SessionId: stringPointer("public-live-id")})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	wantSessionIDs := []string{"runtime-scope-id", "logical-scope-id", "public-live-id"}
	if gotRequest.SessionID != "runtime-scope-id" || !reflect.DeepEqual(gotRequest.SessionIDs, wantSessionIDs) {
		t.Fatalf("metrics query request = %#v, want resolved retained IDs plus public selector %#v", gotRequest, wantSessionIDs)
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
		wantFamily factoryapi.ErrorFamily
	}{
		{
			name:       "unknown public session",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
			err:        factoryvisualizationhttp.NewMetricsSessionNotFoundError("missing-live-id", nil),
			status:     http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCode(factoryvisualizationhttp.MetricsSessionNotFoundCode),
			wantPhrase: "you session list --scope live",
		},
		{
			name:       "known session without retained scope",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
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
			resolver := metricsScopeResolverFunc(func(string) (factorysessions.RuntimeMetricsScope, error) {
				return factorysessions.RuntimeMetricsScope{}, test.err
			})
			handler := factoryvisualizationhttp.NewMetricsHandler(factoryvisualizationhttp.NewMetricsAdapter(query, resolver, staticMetricsRoot("/tmp/metrics")), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.GetMetrics(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{SessionId: stringPointer("selected-live-id")})

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			assertMetricsErrorContract(t, recorder, test.wantFamily, test.wantCode, test.err.Error())
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
	handler := factoryvisualizationhttp.NewMetricsHandler(factoryvisualizationhttp.NewMetricsAdapter(query, nil, staticMetricsRoot("/tmp/metrics")), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetMetrics(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMetricsHandlerEncodesMetricDetails(t *testing.T) {
	inputTokens, outputTokens := int64(11), int64(7)
	p50, p95 := 12.5, 21.5
	query := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		return factoryvisualization.RuntimeMetricsQueryResult{
			Cost: factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostUnavailable},
			Totals: factoryvisualization.RuntimeMetricsAggregate{
				InputTokens: 11, OutputTokens: 7, CompletedDispatches: 2,
				FailuresByReason: map[string]float64{"timeout": 1},
				DispatchDuration: &factoryvisualization.RuntimeMetricsDuration{
					Samples: 2, P50: &p50, P95: &p95,
				},
			},
			Workstations: []factoryvisualization.RuntimeMetricsBreakdown{{
				Key: "workstation-a", Aggregate: factoryvisualization.RuntimeMetricsAggregate{CompletedDispatches: 2},
			}},
			Providers: []factoryvisualization.RuntimeMetricsBreakdown{{
				Key: "provider-a", Aggregate: factoryvisualization.RuntimeMetricsAggregate{CompletedDispatches: 2},
			}},
			UsageRows: []factoryvisualization.RuntimeMetricsUsageRow{{
				WorkID: "work-a", DispatchID: "dispatch-a", WorkerSessionID: "worker-session-a",
				Provider: "provider-a", Model: "model-a", InputTokens: &inputTokens, OutputTokens: &outputTokens,
			}},
		}, nil
	})
	handler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, nil, staticMetricsRoot("/tmp/metrics")), zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	handler.GetMetrics(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var report factoryapi.MetricsReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode metrics report: %v", err)
	}
	if report.Cost.Availability != string(factoryvisualization.RuntimeMetricsCostUnavailable) ||
		report.Totals.FailuresByReason["timeout"] != 1 || report.Totals.DispatchLatency.Samples != 2 ||
		report.Totals.DispatchLatency.Unit != "milliseconds" {
		t.Fatalf("report aggregate = %#v, want mapped failures and duration", report.Totals)
	}
	if len(report.Workstations) != 1 || len(report.Providers) != 1 || len(report.UsageRows) != 1 {
		t.Fatalf("report detail counts = (%d, %d, %d), want one each", len(report.Workstations), len(report.Providers), len(report.UsageRows))
	}
	row := report.UsageRows[0]
	if row.WorkId == nil || *row.WorkId != "work-a" || row.FactorySessionId != nil || row.InputTokens == nil || *row.InputTokens != inputTokens {
		t.Fatalf("usage row = %#v, want optional identity/token mapping", row)
	}
}

func TestMetricsHandlerMapsQueryFailures(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    factoryapi.ErrorResponseCode
		family  factoryapi.ErrorFamily
		message string
	}{
		{
			name: "invalid request",
			err: &factoryvisualization.RuntimeMetricsQueryError{
				Kind: factoryvisualization.RuntimeMetricsQueryInvalidInput, Message: "invalid metrics request",
			},
			family: factoryapi.ErrorFamilyBadRequest, message: "invalid metrics request",
			status: http.StatusBadRequest, code: factoryapi.ErrorResponseCode(factoryvisualizationhttp.MetricsInvalidRequestCode),
		},
		{family: factoryapi.ErrorFamilyInternalServerError, message: "failed to query runtime metrics", name: "query failure", err: errors.New("reader failed"), status: http.StatusInternalServerError, code: factoryapi.ErrorResponseCode(factoryvisualizationhttp.MetricsQueryFailedCode)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
				return factoryvisualization.RuntimeMetricsQueryResult{}, test.err
			})
			handler := factoryvisualizationhttp.NewMetricsHandler(
				factoryvisualizationhttp.NewMetricsAdapter(query, nil, staticMetricsRoot("/tmp/metrics")), zap.NewNop(),
			)
			recorder := httptest.NewRecorder()
			handler.GetMetrics(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{})

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			assertMetricsErrorContract(t, recorder, test.family, test.code, test.message)
			if response.Code != test.code || response.Message == "" {
				t.Fatalf("error response = %#v, want code %q and message", response, test.code)
			}
		})
	}
}

func TestMetricsScopeErrorUnwrapsCauseAndConstructorsHandleNil(t *testing.T) {
	cause := errors.New("projection unavailable")
	err := factoryvisualizationhttp.NewMetricsScopeUnavailableError("session-a", cause)
	if !errors.Is(err, cause) {
		t.Fatalf("scope error = %v, want cause %v", err, cause)
	}
	if factoryvisualizationhttp.NewMetricsAdapter(nil, nil, staticMetricsRoot("")) != nil {
		t.Fatal("NewMetricsAdapter(nil) returned a handler")
	}
	if factoryvisualizationhttp.NewMetricsHandler(nil, zap.NewNop()) != nil {
		t.Fatal("NewMetricsHandler(nil) returned a handler")
	}
}

func stringPointer(value string) *string { return &value }

func assertMetricsErrorContract(t *testing.T, rec *httptest.ResponseRecorder, family factoryapi.ErrorFamily, code factoryapi.ErrorResponseCode, message string) {
	t.Helper()
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"family": string(family), "code": string(code), "message": message}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("body = %#v, want %#v", body, want)
	}
}

func staticMetricsRoot(root string) func(context.Context) string {
	return func(context.Context) string { return root }
}

func TestMetricsAdapterSharesOwnersButIsolatesHostPaths(t *testing.T) {
	t.Parallel()
	query := factoryvisualization.RuntimeMetricsQuery(func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		if request.MetricsRoot != request.SessionID+"/metrics" {
			t.Errorf("cross-host metrics path: %#v", request)
		}
		return factoryvisualization.RuntimeMetricsQueryResult{}, nil
	})
	resolver := metricsScopeResolverFunc(func(id string) (factorysessions.RuntimeMetricsScope, error) {
		return factorysessions.RuntimeMetricsScope{RequestedFactorySessionID: id, RetainedFactorySessionIDs: []string{id}}, nil
	})
	adapter := factoryvisualizationhttp.NewMetricsAdapter(query, resolver, factoryvisualizationhttp.MetricsRoot)
	for _, id := range []string{"selected", "peer"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			ctx := factoryvisualizationhttp.WithMetricsRoot(context.Background(), " "+id+"/metrics ")
			if _, err := adapter.GetMetrics(ctx, id); err != nil {
				t.Fatal(err)
			}
		})
	}
}

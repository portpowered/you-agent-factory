package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestObserveHTTP_DecodesModeAndMapsSuccess(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 7, 28, 2, 30, 0, 0, time.UTC)
	root := &observeVisualizationRootFake{
		observeView: factoryvisualization.ProjectedView{
			TickCount:          9,
			RetainedEventCount: 3,
			ObservedAt:         observedAt,
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.ObserveHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	if err != nil {
		t.Fatalf("ObserveHTTP: %v", err)
	}
	if !root.observeInvoked {
		t.Fatal("Observe was not invoked through the injected Visualization root")
	}
	if root.lastObserveMode != factoryvisualization.ObserveModeRetainedThenLive {
		t.Fatalf("lastObserveMode = %q, want RETAINED_THEN_LIVE", root.lastObserveMode)
	}
	if root.lastObserveReconnect != nil {
		t.Fatalf("lastObserveReconnect = %#v, want nil", root.lastObserveReconnect)
	}
	if response.View.TickCount != 9 {
		t.Fatalf("response.View.TickCount = %d, want 9", response.View.TickCount)
	}
	if response.View.RetainedEventCount != 3 {
		t.Fatalf("response.View.RetainedEventCount = %d, want 3", response.View.RetainedEventCount)
	}
	if !response.View.ObservedAt.Equal(observedAt) {
		t.Fatalf("response.View.ObservedAt = %v, want %v", response.View.ObservedAt, observedAt)
	}
}

func TestObserveHTTP_DecodesReconnectCursorAndMapsSuccess(t *testing.T) {
	t.Parallel()

	afterSequence := 7
	root := &observeVisualizationRootFake{
		observeView: factoryvisualization.ProjectedView{
			TickCount:          4,
			RetainedEventCount: 2,
			ObservedAt:         time.Date(2026, 7, 28, 2, 31, 0, 0, time.UTC),
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.ObserveHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE","reconnect":{"after_event_id":"evt-1","after_sequence":7}}`),
	)
	if err != nil {
		t.Fatalf("ObserveHTTP: %v", err)
	}
	if !root.observeInvoked {
		t.Fatal("Observe was not invoked through the injected Visualization root")
	}
	if root.lastObserveReconnect == nil {
		t.Fatal("lastObserveReconnect = nil, want reconnect cursor")
	}
	if root.lastObserveReconnect.AfterEventID != "evt-1" {
		t.Fatalf("AfterEventID = %q, want evt-1", root.lastObserveReconnect.AfterEventID)
	}
	if root.lastObserveReconnect.AfterSequence == nil || *root.lastObserveReconnect.AfterSequence != afterSequence {
		t.Fatalf("AfterSequence = %#v, want %d", root.lastObserveReconnect.AfterSequence, afterSequence)
	}
	if response.View.TickCount != 4 {
		t.Fatalf("response.View.TickCount = %d, want 4", response.View.TickCount)
	}
}

func TestObserveHTTP_RejectsMalformedJSONBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.ObserveHTTP(context.Background(), strings.NewReader(`{"mode":`))
	if err == nil {
		t.Fatal("ObserveHTTP malformed JSON = nil, want error")
	}
	if root.observeInvoked {
		t.Fatal("malformed Observe HTTP request must not invoke root")
	}
}

func TestObserveHTTP_AcceptsUnknownFieldsBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	response, err := handler.ObserveHTTP(
		context.Background(),
		strings.NewReader(`{"mode":"RETAINED_THEN_LIVE","extra":true}`),
	)
	if err != nil {
		t.Fatalf("ObserveHTTP unknown field: %v", err)
	}
	if !root.observeInvoked || response.View.TickCount != 0 {
		t.Fatalf("root invocation/response = %v/%#v, want successful known request", root.observeInvoked, response)
	}
}

func TestObserveHTTP_PassesMissingModeToRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	_, err := handler.ObserveHTTP(context.Background(), strings.NewReader(`{}`))
	if err == nil {
		t.Fatal("ObserveHTTP missing mode = nil, want root invalid-input error")
	}
	if !root.observeInvoked {
		t.Fatal("missing-mode Observe HTTP request must invoke root for typed invalid-input failure")
	}
	var projErr *factoryvisualization.ProjectionError
	if !errors.As(err, &projErr) || projErr.Kind != factoryvisualization.ProjectionErrorInvalidInput {
		t.Fatalf("err = %v, want ProjectionErrorInvalidInput", err)
	}
}

func TestHandleObserve_HTTPRoundTripSuccess(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 7, 28, 2, 32, 0, 0, time.UTC)
	root := &observeVisualizationRootFake{
		observeView: factoryvisualization.ProjectedView{
			TickCount:          11,
			RetainedEventCount: 5,
			ObservedAt:         observedAt,
		},
	}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/observe", strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`))
	rec := httptest.NewRecorder()
	handler.HandleObserve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response factoryvisualizationhttp.ObserveHTTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.View.TickCount != 11 || response.View.RetainedEventCount != 5 {
		t.Fatalf("response.View = %#v, want tick 11 retained 5", response.View)
	}
	if !response.View.ObservedAt.Equal(observedAt) {
		t.Fatalf("response.View.ObservedAt = %v, want %v", response.View.ObservedAt, observedAt)
	}
}

func TestHandleObserve_HTTPRoundTripBadRequestBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &observeVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-visualization/observe", strings.NewReader(`not-json`))
	rec := httptest.NewRecorder()
	handler.HandleObserve(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Family != factoryapi.ErrorFamilyBadRequest || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error response = %#v, want bad-request ErrorResponse", response)
	}
	if root.observeInvoked {
		t.Fatal("malformed Observe HTTP request must not invoke root")
	}
}

type observeVisualizationRootFake struct {
	observeInvoked bool

	lastObserveMode      factoryvisualization.ObserveMode
	lastObserveReconnect *factoryvisualization.ObserveReconnectCursor
	observeView          factoryvisualization.ProjectedView
}

var _ factoryvisualization.Root = (*observeVisualizationRootFake)(nil)

func (fake *observeVisualizationRootFake) Activate(
	context.Context,
	factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	panic("unexpected Activate call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) Join(
	context.Context,
	factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	panic("unexpected Join call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	panic("unexpected StopDrain call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) Observe(
	_ context.Context,
	req factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	fake.observeInvoked = true
	fake.lastObserveMode = req.Mode
	fake.lastObserveReconnect = req.Reconnect
	if req.Mode == "" {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: required request parameters are missing",
		}
	}
	if req.Reconnect != nil && req.Reconnect.AfterEventID == "" && req.Reconnect.AfterSequence == nil {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: reconnect cursor is empty",
		}
	}
	view := fake.observeView
	if view.ObservedAt.IsZero() {
		view.ObservedAt = time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	}
	return factoryvisualization.ObserveResult{View: view}, nil
}

func (fake *observeVisualizationRootFake) OpenPresentation(
	context.Context,
	factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	panic("unexpected OpenPresentation call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	panic("unexpected PresentProgress call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) FinalizePresentation(
	context.Context,
	factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	panic("unexpected FinalizePresentation call in observe HTTP adapter test")
}

func (fake *observeVisualizationRootFake) ClosePresentation(
	context.Context,
	factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	panic("unexpected ClosePresentation call in observe HTTP adapter test")
}

// diagnosticObserveRoot controls only the root boundary of the HTTP Adapter.
type diagnosticObserveRoot struct {
	*observeVisualizationRootFake
	observe func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error)
}

func (root *diagnosticObserveRoot) Observe(ctx context.Context, req factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
	return root.observe(ctx, req)
}

func diagnosticAdapter(root factoryvisualization.Root, logger *zap.Logger, fromRoot bool) *factoryvisualizationhttp.Adapter {
	if fromRoot {
		return factoryvisualizationhttp.NewHandlerFromRoot(factoryvisualizationhttp.RootBinding{Visualization: root}, logger)
	}
	return factoryvisualizationhttp.NewHandler(factoryvisualizationhttp.Dependencies{VisualizationRoot: root}, logger)
}

func assertObserveFailure(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response = %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"family": "INTERNAL_SERVER_ERROR", "code": "INTERNAL_ERROR", "message": "factory visualization request failed"}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("body = %#v, want %#v", body, want)
	}
}

func assertSelectedDiagnostic(t *testing.T, logs *observer.ObservedLogs, message, name, correlation, failure string) {
	t.Helper()
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("diagnostics = %#v, want one", entries)
	}
	entry := entries[0]
	if entry.Level != zap.ErrorLevel || entry.Message != message || entry.LoggerName != name {
		t.Fatalf("diagnostic = %#v", entry)
	}
	want := map[string]any{"correlation": correlation, "error": failure}
	if !reflect.DeepEqual(entry.ContextMap(), want) {
		t.Fatalf("fields = %#v, want %#v", entry.ContextMap(), want)
	}
}

func TestHandleObserveSelectedDiagnosticPolicy(t *testing.T) {
	t.Parallel()
	for _, fromRoot := range []bool{false, true} {
		for _, policy := range []string{"capture", "disabled", "noop"} {
			t.Run(fmt.Sprintf("fromRoot=%t/%s", fromRoot, policy), func(t *testing.T) {
				t.Parallel()
				logger, logs, hooks := selectedDiagnosticLogger(policy, "selected", "request-a")
				calls := 0
				req := httptest.NewRequest(http.MethodPost, "/observe", strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`))
				root := &diagnosticObserveRoot{observe: func(ctx context.Context, input factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
					calls++
					if ctx != req.Context() || input.Mode != factoryvisualization.ObserveModeRetainedThenLive || input.Reconnect != nil {
						t.Errorf("delegate input/context = %#v/%v", input, ctx)
					}
					return factoryvisualization.ObserveResult{}, errors.New("controlled observe failure")
				}}
				rec := httptest.NewRecorder()
				diagnosticAdapter(root, logger, fromRoot).HandleObserve(rec, req)
				assertObserveFailure(t, rec)
				if calls != 1 {
					t.Fatalf("calls = %d", calls)
				}
				if policy == "capture" {
					assertSelectedDiagnostic(t, logs, "factory visualization observe request failed", "selected", "request-a", "controlled observe failure")
				} else if logs.Len() != 0 || hooks.Load() != 0 {
					t.Fatal("disabled logger emitted diagnostics")
				}
			})
		}
	}
}

type diagnosticFailingWriter struct {
	header http.Header
	status int
	writes int
}

func (w *diagnosticFailingWriter) Header() http.Header    { return w.header }
func (w *diagnosticFailingWriter) WriteHeader(status int) { w.status = status }
func (w *diagnosticFailingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("controlled write failure")
}

func TestHTTPEncodingSelectedDiagnosticPolicy(t *testing.T) {
	t.Parallel()
	for _, component := range []string{"adapter", "metrics", "absent-metrics"} {
		for _, policy := range []string{"capture", "disabled", "noop"} {
			t.Run(component+"/"+policy, func(t *testing.T) {
				t.Parallel()
				logger, logs, hooks := selectedDiagnosticLogger(policy, "encoder", "response-a")
				writer := &diagnosticFailingWriter{header: make(http.Header)}
				message := "encode response failed"
				status := http.StatusOK
				switch component {
				case "adapter":
					root := &observeVisualizationRootFake{}
					diagnosticAdapter(root, logger, true).HandleObserve(writer, httptest.NewRequest(http.MethodPost, "/observe", strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`)))
					if !root.observeInvoked {
						t.Fatal("delegate was not called")
					}
				case "metrics":
					calls := 0
					query := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
						calls++
						return factoryvisualization.RuntimeMetricsQueryResult{}, nil
					})
					handler := factoryvisualizationhttp.NewMetricsHandler(factoryvisualizationhttp.NewMetricsAdapter(query, nil, "/controlled"), logger)
					handler.GetMetrics(writer, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{})
					if calls != 1 {
						t.Fatalf("query calls = %d", calls)
					}
					message = "encode metrics response failed"
				case "absent-metrics":
					var handler *factoryvisualizationhttp.MetricsHandler
					handler.GetMetrics(writer, httptest.NewRequest(http.MethodGet, "/metrics", nil), factoryapi.GetMetricsParams{})
					status = http.StatusInternalServerError
				}
				if writer.status != status || writer.header.Get("Content-Type") != "application/json" || writer.writes != 1 {
					t.Fatalf("writer = %#v", writer)
				}
				if policy == "capture" && component != "absent-metrics" {
					assertSelectedDiagnostic(t, logs, message, "encoder", "response-a", "controlled write failure")
				} else if logs.Len() != 0 || hooks.Load() != 0 {
					t.Fatal("disabled or absent handler emitted diagnostics")
				}
			})
		}
	}
}

func TestHandleObserveParallelSelectedLoggers(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	defer close(release)
	done := make(chan struct{}, 2)
	responses := [2]*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	logs := [2]*observer.ObservedLogs{}
	calls := [2]int{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Failure ceiling; channels prove overlap.
	defer cancel()
	for i := range responses {
		core, observed := observer.New(zap.ErrorLevel)
		logs[i] = observed
		logger := zap.New(core).Named(fmt.Sprintf("adapter-%d", i)).With(zap.String("correlation", fmt.Sprintf("request-%d", i)))
		root := &diagnosticObserveRoot{observe: func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			calls[i]++
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return factoryvisualization.ObserveResult{}, ctx.Err()
			}
			if i == 0 {
				return factoryvisualization.ObserveResult{}, errors.New("isolated failure")
			}
			return factoryvisualization.ObserveResult{View: factoryvisualization.ProjectedView{TickCount: 17}}, nil
		}}
		handler := diagnosticAdapter(root, logger, i == 0)
		go func() {
			handler.HandleObserve(responses[i], httptest.NewRequest(http.MethodPost, "/observe", strings.NewReader(`{"mode":"RETAINED_THEN_LIVE"}`)))
			done <- struct{}{}
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("requests did not overlap")
		}
	}
	release <- struct{}{}
	release <- struct{}{}
	for range 2 {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("requests did not finish")
		}
	}
	assertObserveFailure(t, responses[0])
	assertSelectedDiagnostic(t, logs[0], "factory visualization observe request failed", "adapter-0", "request-0", "isolated failure")
	if logs[1].Len() != 0 || calls != [2]int{1, 1} {
		t.Fatalf("peer logs/calls = %v/%v", logs[1].All(), calls)
	}
	assertObservePeerSuccess(t, responses[1])
}

func assertObservePeerSuccess(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var body factoryvisualizationhttp.ObserveHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" || body.View.TickCount != 17 {
		t.Fatalf("peer response = %#v %s", rec, rec.Body.String())
	}
}

func selectedDiagnosticLogger(policy, name, correlation string) (*zap.Logger, *observer.ObservedLogs, *atomic.Int32) {
	level := zap.ErrorLevel
	if policy == "disabled" {
		level = zap.FatalLevel
	}
	core, logs := observer.New(level)
	logger := zap.New(core).Named(name).With(zap.String("correlation", correlation))
	hooks := &atomic.Int32{}
	if policy == "noop" {
		logger = zap.NewNop().WithOptions(zap.Hooks(func(zapcore.Entry) error { hooks.Add(1); return nil }))
	}
	return logger, logs, hooks
}

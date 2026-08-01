package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	factoryevents "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func TestListPackagedFactoriesReturnsPublishedCatalog(t *testing.T) {
	srv := NewServer(nil, nil, nil, zap.NewNop())
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/packaged-factories", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response factoryapi.PackagedFactoryCatalogResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Factories) == 0 {
		t.Fatal("catalog response contained no factories")
	}
	for _, factory := range response.Factories {
		if factory.Name == "" || factory.Project == "" || factory.Slug == "" || len(factory.Json) == 0 || factory.Yaml == "" {
			t.Fatalf("catalog entry is incomplete: %#v", factory)
		}
	}
}

type strictModelsServiceFake struct {
	modelinference.Service
	list   func(context.Context) (modelinference.List, error)
	get    func(context.Context, string) (modelinference.Detail, error)
	invoke func(context.Context, string, modelinference.Request) (modelinference.Result, error)
	pull   func(context.Context, string) (modelinference.PullResult, error)
}

func (fake strictModelsServiceFake) ListCatalog(ctx context.Context, _ modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	if fake.list == nil {
		panic("unexpected models.Service.ListCatalog call")
	}
	list, err := fake.list(ctx)
	return modelinference.ListModelsResult{Models: list.Results}, err
}

func (fake strictModelsServiceFake) GetCatalogModel(ctx context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
	if fake.get == nil {
		panic("unexpected models.Service.GetCatalogModel call")
	}
	detail, err := fake.get(ctx, request.Name)
	return modelinference.GetModelResult{Model: detail}, err
}

func (fake strictModelsServiceFake) InvokeModel(ctx context.Context, name string, request modelinference.Request) (modelinference.Result, error) {
	if fake.invoke == nil {
		panic("unexpected models.Service.InvokeModel call")
	}
	return fake.invoke(ctx, name, request)
}

func (fake strictModelsServiceFake) PullModelForScope(ctx context.Context, request modelinference.PullModelRequest) (modelinference.PullResult, error) {
	if fake.pull == nil {
		panic("unexpected models.Service.PullModelForScope call")
	}
	return fake.pull(ctx, request.Name)
}

func newStrictModelTestServer(models strictModelsServiceFake) *Server {
	logger := zap.NewNop()
	return newServerFromRoles(
		nil, nil, nil, nil, nil, nil,
		modelshttp.NewHandler(modelshttp.NewAdapter(models, models, modelHTTPContentPreparation{}, modelHTTPTestScope()), logger),
		nil, httpFactoryValidator{}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
}

func modelHTTPTestScope() modelinference.RuntimeScopeRef {
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("factory-session:http-transport-test")
	if err != nil {
		panic(err)
	}
	return scope
}

type modelHTTPContentPreparation struct{}

func (modelHTTPContentPreparation) PrepareWorkContent(_ context.Context, content []work.WorkContentPart) ([]work.WorkContentPart, error) {
	return content, nil
}

func newEventStreamTestServer() *Server {
	logger := zap.NewNop()
	return &Server{Adapter: factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{}, logger), logger: logger}
}

func canonicalFactoryEventForHTTPTest(t *testing.T, event factoryapi.FactoryEvent) interfaces.FactoryEvent {
	t.Helper()
	canonical, err := interfaces.NewFactoryEvent(event)
	if err != nil {
		t.Fatalf("create canonical Factory Event: %v", err)
	}
	return canonical
}

func TestReconnectCursorFromParams(t *testing.T) {
	afterEventID := factoryapi.AfterEventId("event-2")
	afterSequence := factoryapi.AfterSequence(7)

	if got := reconnectCursorFromParams(nil, nil); got != nil {
		t.Fatalf("reconnect cursor = %#v, want nil", got)
	}

	got := reconnectCursorFromParams(&afterEventID, &afterSequence)
	if got == nil {
		t.Fatal("reconnect cursor = nil, want values")
	}
	if got.AfterEventID != "event-2" {
		t.Fatalf("afterEventID = %q, want event-2", got.AfterEventID)
	}
	if got.AfterSequence == nil || *got.AfterSequence != 7 {
		t.Fatalf("afterSequence = %#v, want 7", got.AfterSequence)
	}
}

func TestSessionScopedGetEvents_WritesHistoricalAndLiveSSE(t *testing.T) {
	srv := newEventStreamTestServer()
	liveEvents := make(chan interfaces.FactoryEvent, 1)
	liveEvents <- canonicalFactoryEventForHTTPTest(t, factoryapi.FactoryEvent{Id: "event-live", Type: factoryapi.FactoryEventTypeDispatchRequest})
	close(liveEvents)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-a/events", nil)
	rec := httptest.NewRecorder()
	srv.getEvents(rec, req, true, func(context.Context) (*interfaces.FactoryEventStream, error) {
		return &interfaces.FactoryEventStream{
			History: []interfaces.FactoryEvent{canonicalFactoryEventForHTTPTest(t, factoryapi.FactoryEvent{Id: "event-history", Type: factoryapi.FactoryEventTypeWorkRequest})},
			Events:  liveEvents,
		}, nil
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache-control = %q, want no-cache", got)
	}

	reader := bufio.NewReader(bytes.NewReader(rec.Body.Bytes()))
	historical := readSSEFactoryEvent(t, reader)
	if historical.Id != "event-history" {
		t.Fatalf("historical event id = %q, want event-history", historical.Id)
	}
	live := readSSEFactoryEvent(t, reader)
	if live.Id != "event-live" {
		t.Fatalf("live event id = %q, want event-live", live.Id)
	}
}

func TestSessionScopedGetEvents_ErrorResponses(t *testing.T) {
	srv := newEventStreamTestServer()

	tests := []struct {
		name       string
		writer     http.ResponseWriter
		subscribe  func(context.Context) (*interfaces.FactoryEventStream, error)
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:   "streaming_unsupported",
			writer: &nonFlushingResponseWriter{},
			subscribe: func(context.Context) (*interfaces.FactoryEventStream, error) {
				t.Fatal("subscribe should not be called when streaming is unsupported")
				return nil, nil
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "streaming unsupported",
		},
		{
			name:   "factory_session_not_found",
			writer: httptest.NewRecorder(),
			subscribe: func(context.Context) (*interfaces.FactoryEventStream, error) {
				return nil, apisurface.ErrFactorySessionNotFound
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "factory session not found",
		},
		{
			name:   "invalid_reconnect_cursor",
			writer: httptest.NewRecorder(),
			subscribe: func(context.Context) (*interfaces.FactoryEventStream, error) {
				return nil, factoryevents.ErrReconnectCursorNotFound
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "invalid event reconnect cursor",
		},
		{
			name:   "internal_subscription_error",
			writer: httptest.NewRecorder(),
			subscribe: func(context.Context) (*interfaces.FactoryEventStream, error) {
				return nil, errors.New("boom")
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "failed to subscribe to factory events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-a/events", nil)
			srv.getEvents(tt.writer, req, true, tt.subscribe)

			switch writer := tt.writer.(type) {
			case *httptest.ResponseRecorder:
				assertJSONError(t, writer, tt.wantStatus, tt.wantCode, tt.wantMsg)
			case *nonFlushingResponseWriter:
				assertJSONErrorResponse(t, writer.status, writer.header, writer.body.String(), tt.wantStatus, tt.wantCode, tt.wantMsg)
			default:
				t.Fatalf("unexpected writer type %T", tt.writer)
			}
		})
	}
}

func TestSessionScopedGetEvents_SessionHandshakeWritesResolvedIdentityHeaders(t *testing.T) {
	srv := newEventStreamTestServer()
	liveEvents := make(chan interfaces.FactoryEvent)
	close(liveEvents)

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-a/events", nil)
	rec := httptest.NewRecorder()
	srv.getEvents(rec, req, true, func(context.Context) (*interfaces.FactoryEventStream, error) {
		return &interfaces.FactoryEventStream{
			BackendScopeID:      "backend-scope-001",
			LogicalSessionKeyID: "/workspace/root::default::",
			FactorySessionID:    "f7c2a9b1-4d3e-4f8a-9b0c-1a2b3c4d5e6f",
			StreamGenerationID:  "stream-gen-live-001",
			Events:              liveEvents,
		}, nil
	})

	if got := rec.Header().Get(sessionEventStreamBackendScopeHeader); got != "backend-scope-001" {
		t.Fatalf("%s = %q, want backend-scope-001", sessionEventStreamBackendScopeHeader, got)
	}
	if got := rec.Header().Get(sessionEventStreamLogicalSessionKeyHeader); got != "/workspace/root::default::" {
		t.Fatalf("%s = %q, want /workspace/root::default::", sessionEventStreamLogicalSessionKeyHeader, got)
	}
	if got := rec.Header().Get(sessionEventStreamFactorySessionHeader); got != "f7c2a9b1-4d3e-4f8a-9b0c-1a2b3c4d5e6f" {
		t.Fatalf("%s = %q, want resolved UUID factory session id", sessionEventStreamFactorySessionHeader, got)
	}
	if got := rec.Header().Get(sessionEventStreamGenerationHeader); got != "stream-gen-live-001" {
		t.Fatalf("%s = %q, want stream-gen-live-001", sessionEventStreamGenerationHeader, got)
	}
}

func TestListModels_ReturnsInternalErrorWhenRuntimeFails(t *testing.T) {
	srv := newStrictModelTestServer(strictModelsServiceFake{list: func(context.Context) (modelinference.List, error) {
		return modelinference.List{}, errors.New("list failed")
	}})

	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list models")
}

func TestGetModel_ReturnsInternalErrorWhenRuntimeFails(t *testing.T) {
	srv := newStrictModelTestServer(strictModelsServiceFake{get: func(context.Context, string) (modelinference.Detail, error) {
		return modelinference.Detail{}, errors.New("detail failed")
	}})

	req := httptest.NewRequest(http.MethodGet, "/models/OMNIVOICE_Q4_K_M", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load model")
}

func TestInvokeModel_ErrorMappings(t *testing.T) {
	t.Run("request_validation", testInvokeModelRequestValidation)
	t.Run("runtime_and_fallback_errors", testInvokeModelRuntimeErrors)
}

type invokeModelErrorCase struct {
	name       string
	body       string
	invokeErr  error
	wantStatus int
	wantCode   string
	wantMsg    string
}

func testInvokeModelRequestValidation(t *testing.T) {
	t.Helper()

	tests := []invokeModelErrorCase{
		{
			name:       "missing_body",
			body:       "",
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "request body is required",
		},
		{
			name:       "invalid_content_shape",
			body:       `{"operation":"TTS","content":{}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "content must be an array",
		},
		{
			name:       "missing_operation",
			body:       `{"content":[{"type":"TEXT","text":"hello world"}]}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "operation is required",
		},
	}

	assertInvokeModelErrors(t, tests)
}

func testInvokeModelRuntimeErrors(t *testing.T) {
	t.Helper()

	validBody := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`

	tests := []invokeModelErrorCase{
		{
			name:       "model_not_found",
			body:       validBody,
			invokeErr:  modelinference.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "model not found",
		},
		{
			name:       "runtime_loading",
			body:       validBody,
			invokeErr:  fmt.Errorf("%w: download in progress", modelinference.ErrLoading),
			wantStatus: http.StatusConflict,
			wantCode:   "MODEL_RUNTIME_LOADING",
			wantMsg:    "managed runtime loading: download in progress",
		},
		{
			name:       "runtime_failed",
			body:       validBody,
			invokeErr:  fmt.Errorf("%w: crash loop", modelinference.ErrFailed),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "MODEL_RUNTIME_FAILED",
			wantMsg:    "managed runtime failed: crash loop",
		},
		{
			name:       "runtime_unsupported",
			body:       validBody,
			invokeErr:  fmt.Errorf("%w: unsupported accelerator", modelinference.ErrUnsupported),
			wantStatus: http.StatusBadRequest,
			wantCode:   "MODEL_RUNTIME_UNSUPPORTED",
			wantMsg:    "managed runtime unsupported: unsupported accelerator",
		},
		{
			name:       "unsupported_operation",
			body:       validBody,
			invokeErr:  modelinference.ErrUnsupportedOperation,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "model invocation operation is not supported",
		},
		{
			name: "provider_execution_timeout",
			body: validBody,
			invokeErr: &workers.InferenceFailure{
				Class:   workers.InferenceFailureClassTimeout,
				Message: "inference timed out for model \"OMNIVOICE_Q4_K_M\" operation \"TTS\": wait and retry the request",
			},
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "MODEL_INFERENCE_TIMEOUT",
			wantMsg:    "inference timed out for model \"OMNIVOICE_Q4_K_M\" operation \"TTS\": wait and retry the request",
		},
		{
			name:       "fallback_bad_request",
			body:       validBody,
			invokeErr:  errors.New("  bad model input  "),
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "bad model input",
		},
	}

	assertInvokeModelErrors(t, tests)
}

func assertInvokeModelErrors(t *testing.T, tests []invokeModelErrorCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newStrictModelTestServer(strictModelsServiceFake{invoke: func(context.Context, string, modelinference.Request) (modelinference.Result, error) {
				return modelinference.Result{}, tt.invokeErr
			}})

			req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/invocations", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			assertJSONError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMsg)
		})
	}
}

func TestPullModel_ErrorMappings(t *testing.T) {
	tests := []struct {
		name             string
		pullErr          error
		wantStatus       int
		wantCode         string
		wantMsg          string
		wantManagedState factoryapi.ManagedRuntimeReadinessState
		wantPullOutcome  factoryapi.ManagedRuntimePullOutcome
	}{
		{
			name:       "model_not_found",
			pullErr:    modelinference.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "model not found",
		},
		{
			name:       "pull_unsupported",
			pullErr:    modelinference.ErrPullUnsupported,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "model pull is not supported",
		},
		{
			name: "managed_runtime_timeout",
			pullErr: &modelinference.PullError{
				Result: modelinference.PullResult{
					ModelName:          "OMNIVOICE_Q4_K_M",
					ProviderLocality:   interfaces.ModelLocalityLocal,
					ManagedPullOutcome: "TIMED_OUT",
					ReadinessState:     "FAILED",
				},
				Cause: context.DeadlineExceeded,
			},
			wantStatus:       http.StatusGatewayTimeout,
			wantManagedState: factoryapi.ManagedRuntimeReadinessStateFAILED,
			wantPullOutcome:  factoryapi.ManagedRuntimePullOutcomeTIMEDOUT,
		},
		{
			name:       "generic_internal_error",
			pullErr:    errors.New("  pull failed  "),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "pull failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newStrictModelTestServer(strictModelsServiceFake{pull: func(context.Context, string) (modelinference.PullResult, error) {
				return modelinference.PullResult{}, tt.pullErr
			}})

			req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/pull", nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if tt.wantCode != "" {
				assertJSONError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMsg)
				return
			}

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			response := decodeJSONResponse[factoryapi.ModelPullResponse](t, rec)
			if response.ManagedRuntimePull.ReadinessState != tt.wantManagedState {
				t.Fatalf("managed runtime readiness = %s, want %s", response.ManagedRuntimePull.ReadinessState, tt.wantManagedState)
			}
			if response.ManagedRuntimePull.PullOutcome != tt.wantPullOutcome {
				t.Fatalf("managed runtime pull outcome = %s, want %s", response.ManagedRuntimePull.PullOutcome, tt.wantPullOutcome)
			}
		})
	}
}

type nonFlushingResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *nonFlushingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *nonFlushingResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *nonFlushingResponseWriter) WriteHeader(status int) {
	w.status = status
}

func assertJSONErrorResponse(t *testing.T, gotStatus int, header http.Header, body string, wantStatus int, wantCode string, wantMessage string) {
	t.Helper()

	if gotStatus != wantStatus {
		t.Fatalf("status = %d, want %d: %s", gotStatus, wantStatus, body)
	}
	if got := header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	rec := httptest.NewRecorder()
	rec.Code = gotStatus
	rec.HeaderMap = header
	rec.Body.WriteString(body)
	assertJSONError(t, rec, wantStatus, wantCode, wantMessage)
}

func TestServer_ListModels_RoutesThroughInjectedModelsService(t *testing.T) {
	srv := newStrictModelTestServer(strictModelsServiceFake{list: func(context.Context) (modelinference.List, error) {
		return modelinference.List{
			Results: []modelinference.Summary{{Name: "OMNIVOICE_Q4_K_M"}},
		}, nil
	}})
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /models status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	response := decodeJSONResponse[factoryapi.ListModelsResponse](t, rec)
	if len(response.Results) != 1 || response.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("models = %#v, want one OMNIVOICE summary", response.Results)
	}
}

func modelWiringFactoryConfig(includeResource bool) map[string]any {
	worker := map[string]any{
		"name":          "voice-local",
		"type":          interfaces.WorkerTypeModel,
		"modelProvider": "CODEX",
		"model":         "OMNIVOICE_Q4_K_M",
		"modelLocality": interfaces.ModelLocalityLocal,
		"operations": []map[string]any{{
			"name": "TTS",
			"inputs": []map[string]any{{
				"name":         "text",
				"contentTypes": []string{interfaces.ModelOperationContentTypeText},
				"required":     true,
			}},
			"outputs": []map[string]any{{
				"name":         "audio",
				"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}
	cfg := map[string]any{
		"name":    "factory",
		"workers": []map[string]any{worker},
	}
	if includeResource {
		worker["resources"] = []map[string]any{{"name": "omnivoice-cache", "capacity": 1}}
		cfg["resources"] = []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       interfaces.ResourceTypeModel,
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}}
	}
	return cfg
}

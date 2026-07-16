package http

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	"go.uber.org/zap"
)

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

// TestCompatibilityGetEvents_* exercises handler regressions for compatibility-only GET /events.
func TestCompatibilityGetEvents_WritesHistoricalAndLiveSSE(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	liveEvents := make(chan interfaces.FactoryEvent, 1)
	liveEvents <- testutil.FactoryEvent(t, factoryapi.FactoryEvent{Id: "event-live", Type: factoryapi.FactoryEventTypeDispatchRequest})
	close(liveEvents)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	srv.getEvents(rec, req, false, func(context.Context) (*interfaces.FactoryEventStream, error) {
		return &interfaces.FactoryEventStream{
			History: []interfaces.FactoryEvent{testutil.FactoryEvent(t, factoryapi.FactoryEvent{Id: "event-history", Type: factoryapi.FactoryEventTypeWorkRequest})},
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

func TestCompatibilityGetEvents_ErrorResponses(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	tests := []struct {
		name       string
		writer     http.ResponseWriter
		subscribe  func(context.Context) (*interfaces.FactoryEventStream, error)
		session    bool
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
			session:    false,
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
			session:    false,
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
			session:    false,
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
			session:    false,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "failed to subscribe to factory events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/events", nil)
			srv.getEvents(tt.writer, req, tt.session, tt.subscribe)

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
	srv := newTestServer(&testutil.MockFactory{})
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
	srv := newTestServer(&testutil.MockFactory{ListModelsErr: errors.New("list failed")})

	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list models")
}

func TestGetModel_ReturnsInternalErrorWhenRuntimeFails(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{GetModelErr: errors.New("detail failed")})

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
			invokeErr:  apisurface.ErrModelNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "model not found",
		},
		{
			name:       "runtime_loading",
			body:       validBody,
			invokeErr:  fmt.Errorf("%w: download in progress", apisurface.ErrManagedRuntimeLoading),
			wantStatus: http.StatusConflict,
			wantCode:   "MODEL_RUNTIME_LOADING",
			wantMsg:    "managed runtime loading: download in progress",
		},
		{
			name:       "runtime_failed",
			body:       validBody,
			invokeErr:  fmt.Errorf("%w: crash loop", apisurface.ErrManagedRuntimeFailed),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "MODEL_RUNTIME_FAILED",
			wantMsg:    "managed runtime failed: crash loop",
		},
		{
			name:       "runtime_unsupported",
			body:       validBody,
			invokeErr:  fmt.Errorf("%w: unsupported accelerator", apisurface.ErrManagedRuntimeUnsupported),
			wantStatus: http.StatusBadRequest,
			wantCode:   "MODEL_RUNTIME_UNSUPPORTED",
			wantMsg:    "managed runtime unsupported: unsupported accelerator",
		},
		{
			name:       "unsupported_operation",
			body:       validBody,
			invokeErr:  apisurface.ErrModelInvocationUnsupportedOperation,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "model invocation operation is not supported",
		},
		{
			name: "provider_execution_timeout",
			body: validBody,
			invokeErr: &apisurface.InferenceFailure{
				Class:   apisurface.InferenceFailureClassTimeout,
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
			srv := newTestServer(&testutil.MockFactory{InvokeModelErr: tt.invokeErr})

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
			pullErr:    apisurface.ErrModelNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "model not found",
		},
		{
			name:       "pull_unsupported",
			pullErr:    apisurface.ErrModelPullUnsupported,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "model pull is not supported",
		},
		{
			name: "managed_runtime_timeout",
			pullErr: &apisurface.ManagedRuntimePullError{
				Result: apisurface.ModelPullResult{
					ModelName:          "OMNIVOICE_Q4_K_M",
					ProviderLocality:   workerconfig.ModelLocalityLocal,
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
			srv := newTestServer(&testutil.MockFactory{PullModelErr: tt.pullErr})

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

type listModelsAPI struct{ apisurface.ModelAPI }

func (listModelsAPI) ListModels(context.Context) (factoryapi.ListModelsResponse, error) {
	return factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "OMNIVOICE_Q4_K_M"}}}, nil
}

func TestServer_ListModels_RoutesThroughWiredModelService(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, modelWiringFactoryConfig(true))

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:                      dir,
		MockWorkersConfig:        factoryconfig.NewEmptyMockWorkersConfig(),
		Logger:                   zap.NewNop(),
		SystemConfigHomeDir:      dir,
		RuntimeFileLoggingPolicy: service.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:     service.RuntimeMetricsPolicyDisabled,
		ModelAPI:                 listModelsAPI{},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	srv := NewServer(svc, 0, zap.NewNop())
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

func TestServerCompositionRejectsInvalidFactoryBeforeServing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{Dir: t.TempDir()}

	_, errService := service.BuildFactoryService(ctx, cfg)

	if errService == nil {
		t.Fatal("expected server dependency construction to fail without factory.json")
	}
}

func modelWiringFactoryConfig(includeResource bool) map[string]any {
	worker := map[string]any{
		"name":          "voice-local",
		"type":          interfaces.WorkerTypeModel,
		"modelProvider": "CODEX",
		"model":         "OMNIVOICE_Q4_K_M",
		"modelLocality": workerconfig.ModelLocalityLocal,
		"operations": []map[string]any{{
			"name": "TTS",
			"inputs": []map[string]any{{
				"name":         "text",
				"contentTypes": []string{workerconfig.ModelOperationContentTypeText},
				"required":     true,
			}},
			"outputs": []map[string]any{{
				"name":         "audio",
				"contentTypes": []string{workerconfig.ModelOperationContentTypeAudio},
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
			"type":       factoryresource.TypeModel,
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}}
	}
	return cfg
}

func TestParseCodexSessionDetails_PreservesLongMessageContent(t *testing.T) {
	longPart := strings.Repeat("skill description ", 90) + "final-visible-tail"
	session := strings.Join([]string{
		`{"timestamp":"2026-06-04T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-06-04T10:00:01Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"permissions block"},{"type":"input_text","text":` + strconv.Quote(longPart) + `}]}}`,
	}, "\n")

	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}

	if len(parsed.Transcript) != 1 {
		t.Fatalf("transcript = %#v, want one developer message transcript entry", parsed.Transcript)
	}
	got := stringValue(parsed.Transcript[0].Text)
	if !strings.Contains(got, "permissions block") || !strings.Contains(got, "final-visible-tail") {
		t.Fatalf("transcript text length = %d, want full joined message content with tail; text=%q", len(got), got)
	}
	if strings.HasSuffix(got, "...") {
		t.Fatalf("transcript text = %q, want no backend truncation suffix", got)
	}
}

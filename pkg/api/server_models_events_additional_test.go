package api

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
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

func TestGetEvents_WritesHistoricalAndLiveSSE(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	liveEvents := make(chan factoryapi.FactoryEvent, 1)
	liveEvents <- factoryapi.FactoryEvent{Id: "event-live", Type: factoryapi.FactoryEventTypeDispatchRequest}
	close(liveEvents)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	srv.getEvents(rec, req, func(context.Context) (*interfaces.FactoryEventStream, error) {
		return &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{{Id: "event-history", Type: factoryapi.FactoryEventTypeWorkRequest}},
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

func TestGetEvents_ErrorResponses(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

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
			req := httptest.NewRequest(http.MethodGet, "/events", nil)
			srv.getEvents(tt.writer, req, tt.subscribe)

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
			name:       "provider_execution_failure",
			body:       validBody,
			invokeErr:  errors.New("provider execution failed: upstream timeout"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "provider execution failed: upstream timeout",
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

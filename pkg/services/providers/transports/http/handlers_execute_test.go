package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAdapter_ExecuteHTTPInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked providers.ExecuteRequest
	session := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "session-attempt-1",
	}
	fake := &rootFake{
		execute: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			invoked = request
			return providers.ExecuteResult{
				Content:    "hello-result",
				SessionRef: &session,
				Diagnostics: &providers.ExecuteDiagnostics{
					DurationMillis: 42,
				},
			}, nil
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()
	body := `{"attemptId":"attempt-1","userMessage":"hello"}`

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(body)),
		"codex",
	)

	if invoked.Provider != providers.IDCodex || invoked.AttemptID != "attempt-1" {
		t.Fatalf("ExecuteHTTP invoked root with request = %#v", invoked)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body = %s", recorder.Code, recorder.Body.String())
	}
	var response ExecuteResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Content != "hello-result" ||
		response.SessionRef == nil ||
		response.SessionRef.ID != "session-attempt-1" ||
		response.Diagnostics == nil ||
		response.Diagnostics.DurationMillis != 42 {
		t.Fatalf("response = %#v, want encoded execute success", response)
	}
}

func TestAdapter_ExecuteHTTPRejectsInvalidProviderIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			t.Fatal("fake root must not be invoked for invalid provider id")
			return providers.ExecuteResult{}, nil
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers//execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"   ",
	)

	assertExecuteHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", "invalid provider id")
}

func TestAdapter_ExecuteHTTPRejectsMissingAttemptIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			t.Fatal("fake root must not be invoked for missing attempt id")
			return providers.ExecuteResult{}, nil
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"   "}`)),
		"codex",
	)

	assertExecuteHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", "invalid provider execution request")
}

func TestAdapter_ExecuteHTTPMapsUnknownProviderFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrUnknownProvider
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/missing/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"missing",
	)

	assertExecuteHTTPError(t, recorder, http.StatusNotFound, "NOT_FOUND", "provider not found")
}

func TestAdapter_ExecuteHTTPMapsExecuteFailureFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindInvalidRequest,
				Message: "missing user message",
			}
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"codex",
	)

	assertExecuteHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", "missing user message")
}

func TestAdapter_ExecuteHTTPMapsExecuteFailedFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrExecuteFailed
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ExecuteHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/providers/codex/execute", strings.NewReader(`{"attemptId":"attempt-1"}`)),
		"codex",
	)

	assertExecuteHTTPError(t, recorder, http.StatusInternalServerError, "PROVIDER_EXECUTION_FAILED", "provider execution failed")
}

func TestWriteExecuteOrInternalError_SanitizesUnmappedFailures(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()
	err := errors.New("pkg/services/providers/internal/execution: boom")

	WriteExecuteOrInternalErrorForTest(adapter, recorder, err)

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"family":"INTERNAL_SERVER_ERROR"`) ||
		!strings.Contains(body, `"message":"provider execution failed"`) ||
		strings.Contains(body, "pkg/services/providers") ||
		strings.Contains(body, "boom") {
		t.Fatalf("response = %d %s, want sanitized internal error", recorder.Code, body)
	}
}

func assertExecuteHTTPError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantCode string,
	wantMessage string,
) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d body = %s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(response.Code) != wantCode {
		t.Fatalf("code = %q, want %q", response.Code, wantCode)
	}
	if response.Message != wantMessage {
		t.Fatalf("message = %q, want %q", response.Message, wantMessage)
	}
	if strings.Contains(response.Message, "pkg/services/providers/internal") {
		t.Fatalf("message leaks internal package path: %q", response.Message)
	}
}

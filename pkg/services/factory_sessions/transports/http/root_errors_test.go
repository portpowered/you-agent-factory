package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_CloseFactorySessionNotFoundReturnsTypedErrorResponse(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onClose: func(context.Context, string) error {
			return factorysessions.ErrSessionNotFound
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.CloseFactorySession(recorder, httptest.NewRequest(http.MethodDelete, "/factory-sessions/missing-session", nil), "missing-session")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	assertErrorResponse(t, recorder.Body.Bytes(), factoryapi.ErrorFamilyNotFound, factoryapi.ErrorResponseCodeNOTFOUND, "factory session not found")
}

func TestHandlerFromRoot_StartDurableFactorySessionAsyncValidationErrorReturnsTypedErrorResponse(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onStartAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return factorysessions.AsyncStartResult{}, &factorysessions.ExecutionValidationError{
				Field:   "requestId",
				Message: "requestId is required",
			}
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"requestId":"req-alpha","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)

	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/execution/async", body))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	assertErrorResponse(t, recorder.Body.Bytes(), factoryapi.ErrorFamilyBadRequest, factoryapi.ErrorResponseCodeBADREQUEST, "requestId is required")
}

func TestHandlerFromRoot_PauseDurableFactorySessionControlConflictReturnsTypedLifecycleResponse(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onPauseDurable: func(_ context.Context, sessionID string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return factorysessions.LifecycleControlResult{}, &factorysessions.ControlError{
				Operation: factorysessions.LifecycleControlPause,
				Outcome:   factorysessions.LifecycleControlOutcomeInvalidState,
				Status:    factorysessions.LifecycleStatusRunning,
				Message:   "factory session is already running",
			}
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PauseFactorySession(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/dur-sess-pause-conflict/pause", bytes.NewBufferString(`{"reason":"operator pause"}`)),
		"dur-sess-pause-conflict",
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("outcome = %q, want INVALID_STATE", response.Outcome)
	}
}

func TestHandlerFromRoot_GetFactorySessionUnmappedRootErrorDoesNotLeakInternalDetail(t *testing.T) {
	t.Parallel()

	internalErr := fmt.Errorf("pkg/services/factory_sessions/internal/execution/runtime_service.go: unexpected failure")
	root := &httpSessionsRootFake{
		getSession: func(context.Context, string) (factorysessions.SessionProjection, error) {
			return factorysessions.SessionProjection{}, internalErr
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil), "session-alpha")

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "pkg/services/factory_sessions/internal") {
		t.Fatalf("body leaks internal package path: %s", body)
	}
	assertErrorResponse(t, recorder.Body.Bytes(), factoryapi.ErrorFamilyInternalServerError, factoryapi.ErrorResponseCodeINTERNALERROR, "failed to get factory session")
}

func TestHandlerFromRoot_StartDurableFactorySessionAsyncRequestIDConflictReturnsTypedErrorResponse(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onStartAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return factorysessions.AsyncStartResult{}, factorysessions.ErrExecutionRequestIDConflict
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"requestId":"req-conflict","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)

	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/execution/async", body))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	assertErrorResponse(t, recorder.Body.Bytes(), factoryapi.ErrorFamilyConflict, factoryapi.ErrorResponseCodeEXECUTIONREQUESTIDCONFLICT, "requestId was already used with different execution inputs.")
}

func assertErrorResponse(
	t *testing.T,
	body []byte,
	wantFamily factoryapi.ErrorFamily,
	wantCode factoryapi.ErrorResponseCode,
	wantMessage string,
) {
	t.Helper()

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Family != wantFamily {
		t.Fatalf("family = %q, want %q", errResp.Family, wantFamily)
	}
	if errResp.Code != wantCode {
		t.Fatalf("code = %q, want %q", errResp.Code, wantCode)
	}
	if errResp.Message != wantMessage {
		t.Fatalf("message = %q, want %q", errResp.Message, wantMessage)
	}
}

func TestSessionsRootErrorResponseReturnsFalseForNilError(t *testing.T) {
	t.Parallel()

	if status, response, ok := factorysessionshttp.SessionsRootErrorResponseForTest("", nil); ok {
		t.Fatalf("sessionsRootErrorResponse(nil) = (%d, %#v, true), want false", status, response)
	}
}

func TestSessionsRootErrorResponseMapsWrappedNotFound(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("lookup failed: %w", factorysessions.ErrDurableSessionNotFound)
	status, response, ok := factorysessionshttp.SessionsRootErrorResponseForTest("dur-sess-001", wrapped)
	if !ok {
		t.Fatal("sessionsRootErrorResponse = false, want true")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("response = %#v, want ErrorResponse", response)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestSessionsRootErrorResponseReturnsFalseForUnknownError(t *testing.T) {
	t.Parallel()

	if _, _, ok := factorysessionshttp.SessionsRootErrorResponseForTest("", errors.New("opaque")); ok {
		t.Fatal("sessionsRootErrorResponse = true, want false")
	}
}

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHandlerFromRoot_OpenFactorySessionEncodesRootResult(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onOpen: func(_ context.Context, request factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
			if request.FolderPath != "/workspace/alpha" {
				t.Fatalf("folderPath = %q, want /workspace/alpha", request.FolderPath)
			}
			return &factorysessions.OpenResult{
				SessionID: "session-open-alpha",
				Session: &factorysessions.ScopedLiveSessionSummary{
					ID:         "session-open-alpha",
					FolderPath: "/workspace/alpha",
				},
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"folderPath":"/workspace/alpha"}`)

	handler.OpenFactorySession(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/open", body))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Session == nil || response.Session.Id != "session-open-alpha" {
		t.Fatalf("session = %#v, want encoded open session", response.Session)
	}
}

func TestHandlerFromRoot_OpenFactorySessionAcceptsUnknownFieldsWithWarning(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.WarnLevel)
	root := &httpSessionsRootFake{
		onOpen: func(_ context.Context, request factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
			if request.FolderPath != "/workspace/alpha" {
				t.Fatalf("folderPath = %q, want /workspace/alpha", request.FolderPath)
			}
			return &factorysessions.OpenResult{SessionID: "session-open-alpha"}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.New(core))
	recorder := httptest.NewRecorder()
	handler.OpenFactorySession(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/factory-sessions/open",
			strings.NewReader(`{"folderPath":"/workspace/alpha","future":{"value":"secret"}}`),
		),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if warning := recorder.Header().Get("Warning"); !strings.Contains(warning, "299") || !strings.Contains(warning, "$.future") {
		t.Fatalf("Warning = %q, want code 299 and $.future", warning)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("warning log count = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["warning_code"] != int64(httpcompat.WarningCode) || fields["boundary"] != "factory_sessions.http" ||
		fields["operation"] != "open_factory_session" {
		t.Fatalf("warning fields = %#v, want HTTP compatibility metadata", fields)
	}
	if got, ok := fields["json_paths"].([]interface{}); !ok || !reflect.DeepEqual(got, []interface{}{"$.future"}) {
		t.Fatalf("json_paths = %#v, want [$.future]", fields["json_paths"])
	}
}

func TestHandlerFromRoot_OpenFactorySessionMissingFolderPathReturnsBadRequestWithoutRootCall(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onOpen: func(context.Context, factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
			t.Fatal("fake root must not be invoked when folderPath is missing")
			return nil, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"folderPath":""}`)

	handler.OpenFactorySession(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/open", body))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
}

func TestHandlerFromRoot_StartDurableFactorySessionAsyncInvokesRootWithDecodedRequest(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onStartAsync: func(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			if request.RequestID != "req-async-alpha" {
				t.Fatalf("requestId = %q, want req-async-alpha", request.RequestID)
			}
			return factorysessions.AsyncStartResult{
				SessionID: "dur-sess-async-alpha",
				Status:    string(factorysessions.LifecycleStatusRunning),
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"requestId":"req-async-alpha","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)

	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/execution/async", body))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.FactorySessionExecutionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.SessionId != "dur-sess-async-alpha" {
		t.Fatalf("sessionId = %q, want dur-sess-async-alpha", response.SessionId)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestHandlerFromRoot_StartDurableFactorySessionAsyncInvalidSourceReturnsBadRequestWithoutRootCall(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onStartAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			t.Fatal("fake root must not be invoked for invalid execution source")
			return factorysessions.AsyncStartResult{}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"requestId":"req-bad-inline","source":{"kind":"INLINE_WORKFLOW"}}`)

	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/execution/async", body))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("family = %q, want bad_request", errResp.Family)
	}
	if !strings.Contains(errResp.Message, "inlineWorkflow") {
		t.Fatalf("message = %q, want inline workflow validation text", errResp.Message)
	}
}

func TestHandlerFromRoot_CloseFactorySessionInvokesRoot(t *testing.T) {
	t.Parallel()

	closed := false
	root := &httpSessionsRootFake{
		onClose: func(_ context.Context, sessionID string) error {
			if sessionID != "session-close-alpha" {
				t.Fatalf("sessionId = %q, want session-close-alpha", sessionID)
			}
			closed = true
			return nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.CloseFactorySession(recorder, httptest.NewRequest(http.MethodDelete, "/factory-sessions/session-close-alpha", nil), "session-close-alpha")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if !closed {
		t.Fatal("fake root close was not invoked")
	}
}

func TestHandlerFromRoot_PauseDurableFactorySessionEncodesRootLifecycleControl(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onPauseDurable: func(_ context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			if sessionID != "dur-sess-pause-alpha" {
				t.Fatalf("sessionId = %q, want dur-sess-pause-alpha", sessionID)
			}
			if control.Reason != "operator pause" {
				t.Fatalf("reason = %q, want operator pause", control.Reason)
			}
			return factorysessions.LifecycleControlResult{
				SessionID: sessionID,
				Operation: factorysessions.LifecycleControlPause,
				Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
				Status:    factorysessions.LifecycleStatusPaused,
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"reason":"operator pause"}`)

	handler.PauseFactorySession(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/dur-sess-pause-alpha/pause", body), "dur-sess-pause-alpha")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func TestHandlerFromRoot_PauseLiveFactorySessionEncodesRootLifecycleControl(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onPauseLive: func(_ context.Context, sessionID string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			if sessionID != "session-live-pause" {
				t.Fatalf("sessionId = %q, want session-live-pause", sessionID)
			}
			return factorysessions.LifecycleControlResult{
				SessionID: sessionID,
				Operation: factorysessions.LifecycleControlPause,
				Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
				Status:    factorysessions.LifecycleStatusPaused,
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PauseFactorySession(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/session-live-pause/pause", io.NopCloser(bytes.NewBuffer(nil))), "session-live-pause")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
}

func TestHandlerFromRoot_LiveCancelAndTerminateUseSupportedLifecycleControls(t *testing.T) {
	t.Parallel()

	const sessionID = "session-live-stop"
	root := &httpSessionsRootFake{
		onCancelLive: func(_ context.Context, gotSessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			if gotSessionID != sessionID || control.Reason != "operator cancel" {
				t.Fatalf("cancel request = %q/%#v, want %q/operator cancel", gotSessionID, control, sessionID)
			}
			return factorysessions.LifecycleControlResult{
				SessionID: sessionID,
				Operation: factorysessions.LifecycleControlCancel,
				Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
				Status:    factorysessions.LifecycleStatusSucceeded,
			}, nil
		},
		onTerminateLive: func(_ context.Context, gotSessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			if gotSessionID != sessionID || control.Reason != "operator terminate" {
				t.Fatalf("terminate request = %q/%#v, want %q/operator terminate", gotSessionID, control, sessionID)
			}
			return factorysessions.LifecycleControlResult{
				SessionID: sessionID,
				Operation: factorysessions.LifecycleControlTerminate,
				Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
				Status:    factorysessions.LifecycleStatusSucceeded,
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())

	cancelRecorder := httptest.NewRecorder()
	handler.CancelFactorySession(
		cancelRecorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/"+sessionID+"/cancel", strings.NewReader(`{"reason":"operator cancel"}`)),
		factoryapi.SessionID(sessionID),
	)
	if cancelRecorder.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want 200: %s", cancelRecorder.Code, cancelRecorder.Body.String())
	}
	var cancelResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cancelRecorder.Body.Bytes(), &cancelResponse); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if cancelResponse.Operation != factoryapi.FactorySessionLifecycleControlKindCancel || cancelResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("cancel response = %#v, want accepted CANCEL", cancelResponse)
	}

	terminateRecorder := httptest.NewRecorder()
	handler.TerminateFactorySession(
		terminateRecorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/"+sessionID+"/terminate", strings.NewReader(`{"reason":"operator terminate"}`)),
		factoryapi.SessionID(sessionID),
	)
	if terminateRecorder.Code != http.StatusOK {
		t.Fatalf("terminate status = %d, want 200: %s", terminateRecorder.Code, terminateRecorder.Body.String())
	}
	var terminateResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(terminateRecorder.Body.Bytes(), &terminateResponse); err != nil {
		t.Fatalf("decode terminate response: %v", err)
	}
	if terminateResponse.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate || terminateResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("terminate response = %#v, want accepted TERMINATE", terminateResponse)
	}
}

func TestHandlerFromRoot_LiveLifecycleControlConflictIsTyped(t *testing.T) {
	t.Parallel()

	const sessionID = "session-live-conflict"
	root := &httpSessionsRootFake{
		onTerminateLive: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return factorysessions.LifecycleControlResult{}, &factorysessions.ControlError{
				Operation: factorysessions.LifecycleControlTerminate,
				Outcome:   factorysessions.LifecycleControlOutcomeConflict,
				Status:    factorysessions.LifecycleStatusRunning,
				Message:   "runtime must be stopped before deletion",
			}
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.TerminateFactorySession(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/"+sessionID+"/terminate", nil),
		factoryapi.SessionID(sessionID),
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate || response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeConflict || response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("conflict response = %#v, want typed running conflict", response)
	}
}

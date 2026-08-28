package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestContinueWorkerSessionReturnsAcceptedLineageAfterAdmission(t *testing.T) {
	service := &fakeObservationService{continueResult: workersessions.ContinueResult{
		RequestID: "request-1", SourceWorkerSessionID: "source-1", SuccessorWorkerSessionID: "successor-1",
		Session: workersessions.Session{ID: "successor-1", State: workersessions.StateRunning},
	}}
	handler := NewHandler(NewAdapterWithStartAndContinue(service, service, service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/continue", strings.NewReader(`{
		"requestId": " request-1 ",
		"successorWorkerSessionId": " successor-1 ",
		"followUpInput": "  continue the work  "
	}`))

	handler.ContinueWorkerSession(recorder, request, factoryapi.WorkerSessionID(" source-1 "))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionContinueResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Accepted || response.RequestId != "request-1" || response.SourceWorkerSessionId != "source-1" ||
		response.SuccessorWorkerSessionId != "successor-1" || response.PredecessorWorkerSessionId != "source-1" {
		t.Fatalf("response = %#v, want admitted source-to-successor lineage", response)
	}
	if response.EventTopic != "worker-session/successor-1/events" {
		t.Fatalf("event topic = %q, want deterministic successor topic", response.EventTopic)
	}
	if !service.continueCalled || service.continueRequest.RequestID != "request-1" ||
		service.continueRequest.SourceWorkerSessionID != "source-1" ||
		service.continueRequest.SuccessorWorkerSessionID != "successor-1" ||
		service.continueRequest.FollowUpInput != "  continue the work  " {
		t.Fatalf("continuation request = %#v, want normalized identities and preserved input", service.continueRequest)
	}
}

func TestContinueWorkerSessionRejectsMalformedPayloadBeforeService(t *testing.T) {
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapterWithStartAndContinue(service, service, service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ContinueWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/continue", strings.NewReader(`{
		"requestId":"request-1",
		"successorWorkerSessionId":"successor-1",
		"followUpInput":"follow up",
	`)), factoryapi.WorkerSessionID("source-1"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.continueCalled {
		t.Fatal("Worker Sessions Continue was called for malformed input")
	}
}

func TestContinueWorkerSessionAcceptsUnknownFieldsWithWarning(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapterWithStartAndContinue(service, service, service, workServiceStub{}), zap.New(core))
	recorder := httptest.NewRecorder()
	handler.ContinueWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/continue", strings.NewReader(`{
		"requestId":"request-1",
		"successorWorkerSessionId":"successor-1",
		"followUpInput":"follow up",
		"future":{"value":"secret"}
	}`)), factoryapi.WorkerSessionID("source-1"))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", recorder.Code, recorder.Body.String())
	}
	if !service.continueCalled || service.continueRequest.RequestID != "request-1" ||
		service.continueRequest.SourceWorkerSessionID != "source-1" {
		t.Fatalf("continue request = %#v, want known fields preserved", service.continueRequest)
	}
	if warning := recorder.Header().Get("Warning"); !strings.Contains(warning, "299") || !strings.Contains(warning, "$.future") {
		t.Fatalf("Warning = %q, want code 299 and $.future", warning)
	}
	entries := logs.All()
	if len(entries) != 1 || entries[0].ContextMap()["boundary"] != "worker_sessions.http" ||
		entries[0].ContextMap()["operation"] != "continue_worker_session" {
		t.Fatalf("warning logs = %#v, want continue compatibility warning", entries)
	}
}

func TestContinueWorkerSessionMapsStableFailures(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		code factoryapi.ErrorResponseCode
		want int
	}{
		{name: "source not found", err: workersessions.ErrContinuationSourceNotFound, code: factoryapi.ErrorResponseCodeNOTFOUND, want: http.StatusNotFound},
		{name: "source active", err: workersessions.ErrContinuationSourceActive, code: factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONCONFLICT, want: http.StatusConflict},
		{name: "request id conflict", err: workersessions.ErrContinuationRequestIDConflict, code: factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONREQUESTIDCONFLICT, want: http.StatusConflict},
		{name: "provider unavailable", err: workersessions.ErrContinuationProviderSessionInvalid, code: factoryapi.ErrorResponseCodeWORKERSESSIONPROVIDERCONTINUATIONINVALID, want: http.StatusConflict},
		{name: "admission unavailable", err: workersessions.ErrContinuationNotAccepted, code: factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONADMISSIONFAILED, want: http.StatusServiceUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{continueErr: testCase.err}
			handler := NewHandler(NewAdapterWithStartAndContinue(service, service, service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.ContinueWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/continue", strings.NewReader(`{
				"requestId":"request-1","successorWorkerSessionId":"successor-1","followUpInput":"follow up"
			}`)), factoryapi.WorkerSessionID("source-1"))
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.want, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != testCase.code {
				t.Fatalf("error code = %q, want %q", response.Code, testCase.code)
			}
		})
	}
}

func TestInterruptWorkerSessionReturnsPhaseAwareSnapshotsAfterAdmission(t *testing.T) {
	service := &fakeObservationService{interruptResult: workersessions.InterruptResult{
		RequestID: "request-1", SourceWorkerSessionID: "source-1", SuccessorWorkerSessionID: "successor-1",
		Phase: workersessions.InterruptPhaseSuccessorAdmission, Accepted: true,
		Source:    workersessions.Session{ID: "source-1", State: workersessions.StateCanceled},
		Successor: workersessions.Session{ID: "successor-1", State: workersessions.StateRunning},
	}}
	handler := NewHandler(NewAdapterWithStartAndContinueAndInterrupt(service, service, service, service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/interrupt", strings.NewReader(`{
		"requestId": " request-1 ",
		"successorWorkerSessionId": " successor-1 ",
		"replacementMessage": "  replace the work  "
	}`))

	handler.InterruptWorkerSession(recorder, request, factoryapi.WorkerSessionID(" source-1 "))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionInterruptResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Accepted || response.Phase != factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission ||
		response.Source.WorkerSessionId != "source-1" || response.Source.State != factoryapi.WorkerSessionInterruptSnapshotStateCanceled ||
		response.Successor.WorkerSessionId != "successor-1" || response.Successor.State != factoryapi.WorkerSessionInterruptSnapshotStateRunning {
		t.Fatalf("response = %#v, want phase-aware source/successor snapshots", response)
	}
	if !service.interruptCalled || service.interruptRequest.RequestID != "request-1" ||
		service.interruptRequest.SourceWorkerSessionID != "source-1" ||
		service.interruptRequest.SuccessorWorkerSessionID != "successor-1" ||
		service.interruptRequest.ReplacementMessage != "  replace the work  " {
		t.Fatalf("interrupt request = %#v, want normalized identities and preserved input", service.interruptRequest)
	}
}

func TestInterruptWorkerSessionRejectsMalformedPayloadBeforeService(t *testing.T) {
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapterWithStartAndContinueAndInterrupt(service, service, service, service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.InterruptWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/interrupt", strings.NewReader(`{
		"requestId":"request-1",
		"successorWorkerSessionId":"successor-1",
		"replacementMessage":"replace",
	`)), factoryapi.WorkerSessionID("source-1"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionInterruptError
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != "BAD_REQUEST" || response.Phase != factoryapi.WorkerSessionInterruptErrorPhaseValidation {
		t.Fatalf("error response = %#v, want validation phase", response)
	}
	if service.interruptCalled {
		t.Fatal("Worker Sessions Interrupt was called for malformed input")
	}
}

func TestInterruptWorkerSessionAcceptsUnknownFieldsWithWarning(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapterWithStartAndContinueAndInterrupt(service, service, service, service, workServiceStub{}), zap.New(core))
	recorder := httptest.NewRecorder()
	handler.InterruptWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/interrupt", strings.NewReader(`{
		"requestId":"request-1",
		"successorWorkerSessionId":"successor-1",
		"replacementMessage":"replace",
		"future":{"value":"secret"}
	}`)), factoryapi.WorkerSessionID("source-1"))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", recorder.Code, recorder.Body.String())
	}
	if !service.interruptCalled || service.interruptRequest.RequestID != "request-1" ||
		service.interruptRequest.SourceWorkerSessionID != "source-1" {
		t.Fatalf("interrupt request = %#v, want known fields preserved", service.interruptRequest)
	}
	if warning := recorder.Header().Get("Warning"); !strings.Contains(warning, "299") || !strings.Contains(warning, "$.future") {
		t.Fatalf("Warning = %q, want code 299 and $.future", warning)
	}
	entries := logs.All()
	if len(entries) != 1 || entries[0].ContextMap()["boundary"] != "worker_sessions.http" ||
		entries[0].ContextMap()["operation"] != "interrupt_worker_session" {
		t.Fatalf("warning logs = %#v, want interrupt compatibility warning", entries)
	}
}

func TestInterruptWorkerSessionMapsStablePhaseAwareFailures(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		err   error
		code  string
		phase factoryapi.WorkerSessionInterruptErrorPhase
		want  int
	}{
		{name: "source not found", err: workersessions.ErrInterruptSourceNotFound, code: "NOT_FOUND", phase: factoryapi.WorkerSessionInterruptErrorPhaseValidation, want: http.StatusNotFound},
		{name: "request id conflict", err: workersessions.ErrInterruptRequestIDConflict, code: "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT", phase: factoryapi.WorkerSessionInterruptErrorPhaseValidation, want: http.StatusConflict},
		{name: "source cancellation", err: workersessions.ErrInterruptSourceCancellationFailed, code: "WORKER_SESSION_INTERRUPT_SOURCE_CANCELLATION_FAILED", phase: factoryapi.WorkerSessionInterruptErrorPhaseSourceCancellation, want: http.StatusServiceUnavailable},
		{name: "successor admission", err: workersessions.ErrInterruptSuccessorAdmissionFailed, code: "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED", phase: factoryapi.WorkerSessionInterruptErrorPhaseSuccessorAdmission, want: http.StatusServiceUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{interruptErr: testCase.err}
			handler := NewHandler(NewAdapterWithStartAndContinueAndInterrupt(service, service, service, service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.InterruptWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/source-1/interrupt", strings.NewReader(`{
				"requestId":"request-1","successorWorkerSessionId":"successor-1","replacementMessage":"replace"
			}`)), factoryapi.WorkerSessionID("source-1"))
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.want, recorder.Body.String())
			}
			var response factoryapi.WorkerSessionInterruptError
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != testCase.code || response.Phase != testCase.phase {
				t.Fatalf("error response = %#v, want code=%q phase=%q", response, testCase.code, testCase.phase)
			}
		})
	}
}

func TestStartWorkerSessionAcceptsUnknownFieldsWithWarning(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapterWithStart(service, service, workServiceStub{}), zap.New(core))
	recorder := httptest.NewRecorder()
	body := `{"requestId":"request-1","workerSessionId":"worker-1","futureRoot":"secret-root","execution":{"workstationName":"swe","futureExecution":{"value":"secret-execution"},"dispatch":{"dispatchId":"dispatch-1","workstationName":"swe","futureDispatch":true}}}`

	handler.StartWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions", strings.NewReader(body)))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", recorder.Code, recorder.Body.String())
	}
	assertKnownStartWorkerSessionFields(t, service)
	assertCompatibilityWarningHeader(t, recorder)
	assertCompatibilityWarningLog(t, logs, recorder)
}

func assertKnownStartWorkerSessionFields(t *testing.T, service *fakeObservationService) {
	t.Helper()
	if !service.startCalled || service.startRequest.RequestID != "request-1" ||
		service.startRequest.Execution.WorkstationName != "swe" ||
		service.startRequest.Execution.Execution.Dispatch.DispatchID != "dispatch-1" {
		t.Fatalf("start request = %#v, want known fields preserved", service.startRequest)
	}
}

func assertCompatibilityWarningHeader(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	warning := recorder.Header().Get("Warning")
	for _, path := range []string{"$.execution.dispatch.futureDispatch", "$.execution.futureExecution", "$.futureRoot"} {
		if !strings.Contains(warning, path) {
			t.Fatalf("Warning = %q, want %s", warning, path)
		}
	}
}

func assertCompatibilityWarningLog(t *testing.T, logs *observer.ObservedLogs, recorder *httptest.ResponseRecorder) {
	t.Helper()
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("warning log count = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["warning_code"] != int64(httpcompat.WarningCode) ||
		fields["boundary"] != "worker_sessions.http" ||
		fields["operation"] != "start_worker_session" {
		t.Fatalf("warning fields = %#v, want HTTP compatibility metadata", fields)
	}
	if got, ok := fields["json_paths"].([]interface{}); !ok || !reflect.DeepEqual(got, []interface{}{
		"$.execution.dispatch.futureDispatch", "$.execution.futureExecution", "$.futureRoot",
	}) {
		t.Fatalf("json_paths = %#v, want sorted ignored paths", fields["json_paths"])
	}
	if strings.Contains(entries[0].Message, "secret") || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatal("compatibility diagnostics exposed an ignored field value")
	}
}

func TestWorkerSessionControlRoutesPreserveActionAndIdentity(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		action workersessions.ControlAction
		call   func(*Handler, http.ResponseWriter, *http.Request, factoryapi.WorkerSessionID)
		state  workersessions.State
	}{
		{name: "pause", action: workersessions.ControlActionPause, call: (*Handler).PauseWorkerSession, state: workersessions.StatePaused},
		{name: "resume", action: workersessions.ControlActionResume, call: (*Handler).ResumeWorkerSession, state: workersessions.StateRunning},
		{name: "cancel", action: workersessions.ControlActionCancel, call: (*Handler).CancelWorkerSession, state: workersessions.StateCanceled},
		{name: "terminate", action: workersessions.ControlActionTerminate, call: (*Handler).TerminateWorkerSession, state: workersessions.StateTerminated},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &controlHTTPServiceFake{fakeObservationService: fakeObservationService{}, results: make(map[workersessions.ControlAction]workersessions.ControlResult), errors: make(map[workersessions.ControlAction]error)}
			service.results[testCase.action] = workersessions.ControlResult{
				Session: workersessions.Session{ID: "worker-1", State: testCase.state},
				Action:  testCase.action, Outcome: workersessions.ControlOutcomeApplied, DispatchID: "dispatch-1",
			}
			handler := NewHandler(NewAdapterWithStartAndContinueAndInterruptAndControl(service, service, service, service, service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			testCase.call(handler, recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/worker-1/"+testCase.name, nil), factoryapi.WorkerSessionID(" worker-1 "))

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
			}
			var response factoryapi.WorkerSessionControlResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.WorkerSessionId != "worker-1" || workersessions.ControlAction(response.Action) != testCase.action ||
				response.Outcome != factoryapi.WorkerSessionControlResponseOutcomeApplied || response.DispatchId != "dispatch-1" {
				t.Fatalf("response = %#v, want exact control result", response)
			}
			if service.controlRequest.ID != "worker-1" || service.controlAction != testCase.action {
				t.Fatalf("control request = %#v action=%q, want worker-1/%q", service.controlRequest, service.controlAction, testCase.action)
			}
		})
	}
}

func TestWorkerSessionControlMapsStableFailures(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		code string
		want int
	}{
		{name: "invalid identity", err: workersessions.ErrInvalidSessionID, code: "WORKER_SESSION_CONTROL_INVALID", want: http.StatusBadRequest},
		{name: "unknown session", err: workersessions.ErrSessionNotFound, code: "NOT_FOUND", want: http.StatusNotFound},
		{name: "invalid state", err: workersessions.ErrInvalidState, code: "WORKER_SESSION_CONTROL_CONFLICT", want: http.StatusConflict},
		{name: "boundary failure", err: errors.New("cancel boundary failed"), code: "WORKER_SESSION_CONTROL_FAILED", want: http.StatusServiceUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &controlHTTPServiceFake{fakeObservationService: fakeObservationService{}, results: make(map[workersessions.ControlAction]workersessions.ControlResult), errors: make(map[workersessions.ControlAction]error)}
			service.errors[workersessions.ControlActionCancel] = testCase.err
			handler := NewHandler(NewAdapterWithStartAndContinueAndInterruptAndControl(service, service, service, service, service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.CancelWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions/worker-1/cancel", nil), factoryapi.WorkerSessionID("worker-1"))
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.want, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if string(response.Code) != testCase.code {
				t.Fatalf("error code = %q, want %q", response.Code, testCase.code)
			}
		})
	}
}

type controlHTTPServiceFake struct {
	fakeObservationService
	results        map[workersessions.ControlAction]workersessions.ControlResult
	errors         map[workersessions.ControlAction]error
	controlAction  workersessions.ControlAction
	controlRequest workersessions.ControlRequest
}

func (f *controlHTTPServiceFake) ensureMaps() {
	if f.results == nil {
		f.results = make(map[workersessions.ControlAction]workersessions.ControlResult)
	}
	if f.errors == nil {
		f.errors = make(map[workersessions.ControlAction]error)
	}
}

func (f *controlHTTPServiceFake) control(action workersessions.ControlAction, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	f.ensureMaps()
	f.controlAction, f.controlRequest = action, request
	return f.results[action], f.errors[action]
}

func (f *controlHTTPServiceFake) Pause(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionPause, request)
}

func (f *controlHTTPServiceFake) Resume(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionResume, request)
}

func (f *controlHTTPServiceFake) Cancel(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionCancel, request)
}

func (f *controlHTTPServiceFake) Terminate(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionTerminate, request)
}

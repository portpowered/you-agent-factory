package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

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
				t.Fatalf("decode error response: %v", err)
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

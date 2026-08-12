package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
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
		"unexpected":true
	}`)), factoryapi.WorkerSessionID("source-1"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.continueCalled {
		t.Fatal("Worker Sessions Continue was called for malformed input")
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
		"unexpected":true
	}`)), factoryapi.WorkerSessionID("source-1"))

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

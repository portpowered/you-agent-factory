package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestGetWorkerSessionObservationByWorkerSessionIDProjectsProviderNeutralHistory(t *testing.T) {
	service := &fakeObservationService{getByWorkerResult: workersessions.Observation{
		WorkerSessionID:          "worker-no-provider",
		ProviderSessionAvailable: false,
		WorkIDs:                  []string{"work-1"},
		AttemptID:                "dispatch-1",
		State:                    workersessions.StateCompleted,
		DurationBasis:            workersessions.DurationBasisUnavailable,
		Transcript:               workersessions.TranscriptAvailabilityUnavailable,
	}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetWorkerSessionObservationByWorkerSessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/worker-sessions/worker-no-provider", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.WorkerSessionID(" worker-no-provider "),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.WorkerSessionId != "worker-no-provider" || response.ProviderSession != nil || response.ProviderSessionAvailable {
		t.Fatalf("response = %#v, want provider-neutral Worker observation", response)
	}
	if !service.getByWorkerCalled || service.getWorkerSessionID != "worker-no-provider" {
		t.Fatalf("service lookup = called=%t id=%q, want canonical Worker ID", service.getByWorkerCalled, service.getWorkerSessionID)
	}
}

func TestReadWorkerSessionTranscriptByWorkerSessionIDUsesCanonicalIdentity(t *testing.T) {
	text := "historical response"
	service := &fakeObservationService{readResult: workersessions.ReadTranscriptResult{
		WorkerSessionID: "worker-1",
		ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
		WorkIDs:         []string{"work-1"},
		AttemptID:       "dispatch-1",
		State:           workersessions.StateCompleted,
		Entries:         []workersessions.TranscriptEntry{{Order: 1, Type: workersessions.TranscriptAssistantMessage, Text: &text}},
	}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ReadWorkerSessionTranscriptByWorkerSessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/worker-sessions/worker-1/transcript", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.WorkerSessionID(" worker-1 "),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionTranscriptResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.WorkerSessionId != "worker-1" || len(response.Entries) != 1 || response.Entries[0].Text == nil || *response.Entries[0].Text != text {
		t.Fatalf("response = %#v, want normalized Worker transcript", response)
	}
	if !service.readCalled || service.readWorkerSessionID != "worker-1" || service.readProviderSession != (providers.SessionRef{}) {
		t.Fatalf("read request = called=%t worker=%q provider=%#v, want Worker-ID-only request", service.readCalled, service.readWorkerSessionID, service.readProviderSession)
	}
}

func TestReadWorkerSessionTranscriptByWorkerSessionIDMapsUnavailableProviderDetail(t *testing.T) {
	service := &fakeObservationService{readErr: workersessions.ErrObservationTranscriptUnavailable}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ReadWorkerSessionTranscriptByWorkerSessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/worker-sessions/worker-1/transcript", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.WorkerSessionID("worker-1"),
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTUNAVAILABLE {
		t.Fatalf("error code = %q, want typed transcript unavailable", response.Code)
	}
}

func TestWorkerSessionIDHistoryHandlersRejectMissingIdentity(t *testing.T) {
	handler := NewHandler(NewAdapter(&fakeObservationService{}, workServiceStub{}), zap.NewNop())

	observationRecorder := httptest.NewRecorder()
	handler.GetWorkerSessionObservationByWorkerSessionId(
		observationRecorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions/", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.WorkerSessionID(" "),
	)
	if observationRecorder.Code != http.StatusBadRequest {
		t.Fatalf("observation status = %d, want 400", observationRecorder.Code)
	}

	transcriptRecorder := httptest.NewRecorder()
	handler.ReadWorkerSessionTranscriptByWorkerSessionId(
		transcriptRecorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions/", nil),
		factoryapi.SessionID(" "),
		factoryapi.WorkerSessionID("worker-1"),
	)
	if transcriptRecorder.Code != http.StatusBadRequest {
		t.Fatalf("transcript status = %d, want 400", transcriptRecorder.Code)
	}
}

func TestWorkerSessionIDHistoryHandlersRejectMalformedIdentity(t *testing.T) {
	testCases := []struct {
		name string
		call func(*httptest.ResponseRecorder)
	}{
		{
			name: "observation",
			call: func(recorder *httptest.ResponseRecorder) {
				handler := NewHandler(NewAdapter(&fakeObservationService{
					getByWorkerErr: workersessions.ErrInvalidSessionID,
				}, workServiceStub{}), zap.NewNop())
				handler.GetWorkerSessionObservationByWorkerSessionId(
					recorder, httptest.NewRequest(http.MethodGet, "/worker-sessions/bad%20id", nil),
					factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("bad id"),
				)
			},
		},
		{
			name: "transcript",
			call: func(recorder *httptest.ResponseRecorder) {
				handler := NewHandler(NewAdapter(&fakeObservationService{
					readErr: workersessions.ErrInvalidSessionID,
				}, workServiceStub{}), zap.NewNop())
				handler.ReadWorkerSessionTranscriptByWorkerSessionId(
					recorder, httptest.NewRequest(http.MethodGet, "/worker-sessions/bad%20id/transcript", nil),
					factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("bad id"),
				)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			testCase.call(recorder)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
				t.Fatalf("error code = %q, want BAD_REQUEST", response.Code)
			}
		})
	}
}

func TestReadTranscriptRequestWorkerSessionIDDoesNotAcceptProviderReference(t *testing.T) {
	request := workersessions.ReadTranscriptRequest{
		WorkerSessionID: "worker-1",
		ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
	}
	if err := request.Validate(); err == nil {
		t.Fatal("ReadTranscriptRequest.Validate() accepted ambiguous Worker and Provider identities")
	}
}

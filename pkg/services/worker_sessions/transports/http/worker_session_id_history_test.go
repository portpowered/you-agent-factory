package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	handler.GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(
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
	if response.Model != nil || response.ReasoningEffort != nil {
		t.Fatalf("legacy execution facts = model:%#v reasoningEffort:%#v, want absent", response.Model, response.ReasoningEffort)
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != "session-1" {
		t.Fatalf("factory session scope = %#v, want session-1", response.FactorySessionId)
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
	handler.ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId(
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
	if response.FactorySessionId == nil || *response.FactorySessionId != "session-1" {
		t.Fatalf("transcript factory session scope = %#v, want session-1", response.FactorySessionId)
	}
	if response.RecordingHealth != factoryapi.WorkerSessionTranscriptRecordingHealthComplete {
		t.Fatalf("provider-backed recording health = %q, want COMPLETE", response.RecordingHealth)
	}
	if !service.readCalled || service.readWorkerSessionID != "worker-1" || service.readProviderSession != (providers.SessionRef{}) {
		t.Fatalf("read request = called=%t worker=%q provider=%#v, want Worker-ID-only request", service.readCalled, service.readWorkerSessionID, service.readProviderSession)
	}
}

func TestGetWorkerSessionObservationByWorkerSessionIDProjectsRecordingHealthAndScope(t *testing.T) {
	reason := "PROCESS_INTERRUPTED"
	service := &fakeObservationService{getByWorkerResult: workersessions.Observation{
		WorkerSessionID:          "worker-incomplete",
		ProviderSessionAvailable: false,
		WorkIDs:                  []string{"work-1"},
		AttemptID:                "dispatch-1",
		State:                    workersessions.StateTerminated,
		DurationBasis:            workersessions.DurationBasisRecordedTimestamps,
		RecordingHealth:          recordings.WorkerRecordingStatusIncomplete,
		RecordingHealthReason:    reason,
	}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-scope/worker-sessions/worker-incomplete", nil),
		factoryapi.SessionID("session-scope"),
		factoryapi.WorkerSessionID("worker-incomplete"),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RecordingHealth == nil || string(*response.RecordingHealth) != string(recordings.WorkerRecordingStatusIncomplete) || response.RecordingHealthReason == nil || *response.RecordingHealthReason != reason {
		t.Fatalf("recording health = %#v/%#v, want INCOMPLETE/%q", response.RecordingHealth, response.RecordingHealthReason, reason)
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != "session-scope" {
		t.Fatalf("factory session scope = %#v, want session-scope", response.FactorySessionId)
	}
}

func TestWorkerSessionIDHistoryMapsRecordingHealthReadFailures(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		err    error
		code   factoryapi.ErrorResponseCode
		status int
	}{
		{name: "corrupt", err: workersessions.ErrObservationRecordingCorrupt, code: factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGCORRUPT, status: http.StatusInternalServerError},
		{name: "unavailable", err: workersessions.ErrObservationRecordingUnavailable, code: factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGUNAVAILABLE, status: http.StatusServiceUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler := NewHandler(NewAdapter(&fakeObservationService{getByWorkerErr: testCase.err}, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(
				recorder,
				httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/worker-sessions/worker-1", nil),
				factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("worker-1"),
			)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.status, recorder.Body.String())
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

func TestReadWorkerSessionTranscriptByWorkerSessionIDMapsUnavailableProviderDetail(t *testing.T) {
	service := &fakeObservationService{readErr: workersessions.ErrObservationTranscriptUnavailable}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId(
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
	handler.GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(
		observationRecorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions/", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.WorkerSessionID(" "),
	)
	if observationRecorder.Code != http.StatusBadRequest {
		t.Fatalf("observation status = %d, want 400", observationRecorder.Code)
	}

	transcriptRecorder := httptest.NewRecorder()
	handler.ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId(
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
				handler.GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(
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
				handler.ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId(
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

func TestGetWorkerSessionObservationBySessionIDProjectsFailureDiagnostics(t *testing.T) {
	total := 17
	duration := int64(2500)
	failure := &workersessions.FailureCause{
		Kind:                 workersessions.FailureCauseIncompleteOutput,
		Detail:               "the Workers result did not include the required final output",
		AgentRunFailureClass: workers.AgentRunFailureClassProvider,
		ProviderFailureKind:  providers.ExecuteFailureKindDependency,
	}
	service := &fakeObservationService{getResult: workersessions.Observation{
		WorkerSessionID:          "worker-session-1",
		Model:                    stringPtr("gpt-5.6-luna"),
		ReasoningEffort:          stringPtr("medium"),
		ProviderSession:          providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
		ProviderSessionAvailable: true,
		WorkIDs:                  []string{"work-1"}, TurnID: "turn-1", AttemptID: "attempt-1",
		State: workersessions.StateFailed, Duration: durationPtr(2500 * time.Millisecond),
		DurationBasis: workersessions.DurationBasisRecordedTimestamps,
		TokenUsage:    &workersessions.TokenUsage{TotalTokens: &total}, Transcript: workersessions.TranscriptAvailabilityAvailable,
		TurnUsage: &workersessions.TurnUsage{TurnCount: 3, FinalContextTokens: 450, PeakContextTokens: 450},
		Failure:   failure,
		Parse:     workersessions.ParseDiagnostics{EventCount: 4, Errors: []workersessions.ParseDiagnostic{{Code: "provider_session_parse_error", LineNumber: 3, Message: "malformed event"}}},
	}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/detail?provider=codex&kind=session_id&id=provider-session-1", nil)

	handler.GetWorkerSessionObservationBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.GetWorkerSessionObservationBySessionIdParams{
		Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "provider-session-1",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	assertFailureObservationResponse(t, recorder.Body.Bytes(), service, total, duration)
}

func assertFailureObservationResponse(t *testing.T, payload []byte, service *fakeObservationService, total int, duration int64) {
	t.Helper()
	var response factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertFailureObservationIdentity(t, response)
	assertFailureObservationCause(t, response)
	assertFailureObservationUsage(t, response, total, duration)
	assertFailureObservationTurnUsage(t, response)
	assertFailureObservationParse(t, response)
	assertFailureObservationExecutionFacts(t, response)
	assertFailureObservationRequest(t, service)
}

func assertFailureObservationIdentity(t *testing.T, response factoryapi.WorkerSessionObservation) {
	t.Helper()
	if response.WorkerSessionId != "worker-session-1" || response.State != factoryapi.WorkerSessionObservationStateFailed || response.AttemptId != "attempt-1" {
		t.Fatalf("identity/state = %#v, want failed attempt projection", response)
	}
}

func assertFailureObservationCause(t *testing.T, response factoryapi.WorkerSessionObservation) {
	t.Helper()
	if response.Failure == nil || response.Failure.Kind != string(workersessions.FailureCauseIncompleteOutput) ||
		response.Failure.Detail != "the Workers result did not include the required final output" || response.Failure.ProviderFailureKind == nil ||
		response.Failure.AgentRunFailureClass == nil || *response.Failure.AgentRunFailureClass != workers.AgentRunFailureClassProvider {
		t.Fatalf("failure = %#v, want structured failure diagnostics", response.Failure)
	}
}

func assertFailureObservationUsage(t *testing.T, response factoryapi.WorkerSessionObservation, total int, duration int64) {
	t.Helper()
	if response.TokenUsage == nil || response.TokenUsage.TotalTokens == nil || *response.TokenUsage.TotalTokens != total || response.DurationMillis == nil || *response.DurationMillis != duration {
		t.Fatalf("usage/duration = %#v/%v, want %d/%d", response.TokenUsage, response.DurationMillis, total, duration)
	}
}

func assertFailureObservationParse(t *testing.T, response factoryapi.WorkerSessionObservation) {
	t.Helper()
	if response.Parse.EventCount != 4 || len(response.Parse.Errors) != 1 {
		t.Fatalf("parse = %#v, want event and parse diagnostics", response.Parse)
	}
}

func assertFailureObservationRequest(t *testing.T, service *fakeObservationService) {
	t.Helper()
	if service.getProviderSession != (providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}) {
		t.Fatalf("service identity = %#v, want exact provider session ref", service.getProviderSession)
	}
}

func TestReadWorkerSessionTranscriptBySessionIDProjectsNormalizedEntries(t *testing.T) {
	text := "assistant response"
	toolName := "lookup"
	arguments := `{"key":"value"}`
	service := &fakeObservationService{readResult: workersessions.ReadTranscriptResult{
		WorkerSessionID: "worker-session-1",
		ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
		WorkIDs:         []string{"work-1"}, TurnID: "turn-1", AttemptID: "attempt-1", State: workersessions.StateFailed,
		Entries: []workersessions.TranscriptEntry{
			{Order: 1, Type: workersessions.TranscriptToolCall, Name: &toolName, Arguments: &arguments},
			{Order: 2, Type: workersessions.TranscriptAssistantMessage, Text: &text},
		},
	}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/transcript", nil)
	handler.ReadWorkerSessionTranscriptBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.ReadWorkerSessionTranscriptBySessionIdParams{
		Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "provider-session-1",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	assertTranscriptResponse(t, recorder.Body.Bytes(), service, text, toolName)
}

func assertTranscriptResponse(t *testing.T, payload []byte, service *fakeObservationService, text, toolName string) {
	t.Helper()
	var response factoryapi.WorkerSessionTranscriptResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.WorkerSessionId != "worker-session-1" || response.State != string(workersessions.StateFailed) || response.AttemptId != "attempt-1" || response.TurnId == nil || *response.TurnId != "turn-1" {
		t.Fatalf("response envelope = %#v, want correlated terminal session", response)
	}
	if len(response.Entries) != 2 || response.Entries[0].Type != factoryapi.ProviderSessionTranscriptEntryType(workersessions.TranscriptToolCall) || response.Entries[0].Name == nil || *response.Entries[0].Name != toolName || response.Entries[1].Text == nil || *response.Entries[1].Text != text {
		t.Fatalf("entries = %#v, want ordered normalized tool and assistant entries", response.Entries)
	}
	wantRef := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
	if !service.readCalled || service.readProviderSession != wantRef {
		t.Fatalf("read request = called=%t ref=%#v, want exact %v", service.readCalled, service.readProviderSession, wantRef)
	}
}

func TestReadWorkerSessionTranscriptBySessionIDMapsDistinctTranscriptFailures(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		err    error
		status int
		code   factoryapi.ErrorResponseCode
	}{
		{name: "missing", err: workersessions.ErrObservationSessionNotFound, status: http.StatusNotFound, code: factoryapi.ErrorResponseCodeNOTFOUND},
		{name: "active", err: workersessions.ErrObservationTranscriptActive, status: http.StatusConflict, code: factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTACTIVE},
		{name: "unavailable", err: workersessions.ErrObservationTranscriptUnavailable, status: http.StatusInternalServerError, code: factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTUNAVAILABLE},
		{name: "projection", err: workersessions.ErrObservationTranscriptProjectionUnavailable, status: http.StatusInternalServerError, code: factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTPROJECTIONUNAVAILABLE},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{readErr: testCase.err}
			handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/transcript", nil)
			handler.ReadWorkerSessionTranscriptBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.ReadWorkerSessionTranscriptBySessionIdParams{
				Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "provider-session-1",
			})
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.status, recorder.Body.String())
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

func TestAdapterGetWorkerSessionObservationUsesExactProviderSessionIdentity(t *testing.T) {
	service := &fakeObservationService{getResult: workersessions.Observation{
		WorkerSessionID: "worker-session-1", ProviderSessionAvailable: true,
		ProviderSession: providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "cursor-session-1"},
		AttemptID:       "attempt-1", State: workersessions.StateCompleted,
		DurationBasis: workersessions.DurationBasisUnavailable, Transcript: workersessions.TranscriptAvailabilityUnavailable,
	}}
	adapter := NewAdapter(service, workServiceStub{})
	response, err := adapter.GetWorkerSessionObservation(context.Background(), "session-1", " cursor ", " session_id ", " cursor-session-1 ")
	if err != nil {
		t.Fatalf("GetWorkerSessionObservation() error = %v", err)
	}
	if response.WorkerSessionId != "worker-session-1" || response.ProviderSession == nil || response.ProviderSession.Id != "cursor-session-1" {
		t.Fatalf("response = %#v, want detached observation", response)
	}
	want := providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "cursor-session-1"}
	if service.getProviderSession != want {
		t.Fatalf("service identity = %#v, want %#v", service.getProviderSession, want)
	}
}

func TestGetWorkerSessionObservationBySessionIDMapsMissingAndUnavailable(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		err    error
		status int
		code   factoryapi.ErrorResponseCode
	}{
		{name: "missing", err: workersessions.ErrObservationSessionNotFound, status: http.StatusNotFound, code: factoryapi.ErrorResponseCodeNOTFOUND},
		{name: "unavailable", err: workersessions.ErrObservationProjectionUnavailable, status: http.StatusInternalServerError, code: factoryapi.ErrorResponseCode("PROJECTION_UNAVAILABLE")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{getErr: testCase.err}
			handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/detail", nil)
			handler.GetWorkerSessionObservationBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.GetWorkerSessionObservationBySessionIdParams{
				Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "missing",
			})
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.status, recorder.Body.String())
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

func TestGetWorkerSessionObservationBySessionIDRejectsUnsupportedIdentity(t *testing.T) {
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	for _, testCase := range []struct {
		name     string
		provider string
		kind     string
		code     factoryapi.ErrorResponseCode
	}{
		{name: "provider", provider: "other", kind: "session_id", code: factoryapi.ErrorResponseCode("PROVIDER_UNSUPPORTED")},
		{name: "kind", provider: "codex", kind: "rollout", code: factoryapi.ErrorResponseCode("SESSION_KIND_UNSUPPORTED")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/detail", nil)
			handler.GetWorkerSessionObservationBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.GetWorkerSessionObservationBySessionIdParams{
				Provider: factoryapi.LoadableProviderSessionProvider(testCase.provider), Kind: factoryapi.LoadableProviderSessionKind(testCase.kind), Id: "session-1",
			})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != testCase.code {
				t.Fatalf("error code = %q, want %q", response.Code, testCase.code)
			}
			if service.getCalled {
				t.Fatal("observation service called for unsupported identity")
			}
		})
	}
}

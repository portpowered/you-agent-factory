package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"go.uber.org/zap"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWorkerSessionAddressErrorHTTP(t *testing.T) {
	t.Parallel()
	work := "work-owner"
	candidates := []workersessions.AddressCandidate{
		{FactorySessionID: "owner", WorkerSessionID: "legacy", WorkID: &work, State: workersessions.StateRunning},
		{FactorySessionID: "peer", WorkerSessionID: "legacy", State: workersessions.StateCompleted},
	}
	ambiguous := (workersessions.AmbiguousAddressError{Candidates: candidates}).Clone()
	err := fmt.Errorf("lookup: %w", ambiguous)
	handler := &Handler{}
	for _, operation := range []string{"show", "continue", "interrupt"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			switch operation {
			case "show":
				handler.writeMappedObservationError(recorder, err)
			case "continue":
				handler.writeMappedContinueError(recorder, err)
			case "interrupt":
				wrapped := &workersessions.InterruptError{Phase: workersessions.InterruptPhaseValidation, Cause: err}
				handler.writeMappedInterruptError(recorder, wrapped, "legacy", factoryapi.WorkerSessionInterruptRequest{})
			}
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Code    string                               `json:"code"`
				Family  string                               `json:"family"`
				Phase   string                               `json:"phase"`
				Details workersessions.AmbiguousAddressError `json:"details"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code != "WORKER_SESSION_AMBIGUOUS" || payload.Family != "CONFLICT" || !reflect.DeepEqual(payload.Details.Candidates, candidates) {
				t.Fatalf("diagnostic = %s", recorder.Body.String())
			}
			if (operation == "interrupt" && payload.Phase != "VALIDATION") || (operation != "interrupt" && payload.Phase != "") {
				t.Fatalf("phase = %q for %s", payload.Phase, operation)
			}
		})
	}
}

func TestWorkerSessionAddressShowScopeHTTP(t *testing.T) {
	for _, scope := range []string{"factory-left", "factory-right"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			service := &fakeObservationService{getByWorkerResult: workersessions.Observation{
				WorkerSessionID: "legacy", FactorySessionID: scope, State: workersessions.StateCompleted,
			}}
			handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.GetWorkerSessionObservationByWorkerSessionId(recorder,
				httptest.NewRequest(http.MethodGet, "/worker-sessions/legacy?factorySessionId="+scope, nil),
				factoryapi.WorkerSessionID("legacy"), factoryapi.GetWorkerSessionObservationByWorkerSessionIdParams{FactorySessionId: &scope})
			if recorder.Code != http.StatusOK || service.getWorkerSessionID != "legacy" || service.getWorkerFactorySessionID != scope {
				t.Fatalf("status=%d worker=%q scope=%q body=%s", recorder.Code, service.getWorkerSessionID, service.getWorkerFactorySessionID, recorder.Body.String())
			}
			var response factoryapi.WorkerSessionObservation
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.FactorySessionId == nil || *response.FactorySessionId != scope {
				t.Fatalf("selected observation = %#v", response)
			}
		})
	}
	t.Run("unknown owner", func(t *testing.T) {
		t.Parallel()
		scope := "unrelated"
		service := &fakeObservationService{getByWorkerErr: workersessions.ErrObservationSessionNotFound}
		handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
		recorder := httptest.NewRecorder()
		handler.GetWorkerSessionObservationByWorkerSessionId(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "legacy",
			factoryapi.GetWorkerSessionObservationByWorkerSessionIdParams{FactorySessionId: &scope})
		if recorder.Code != http.StatusNotFound || service.getWorkerFactorySessionID != scope {
			t.Fatalf("status=%d scope=%q body=%s", recorder.Code, service.getWorkerFactorySessionID, recorder.Body.String())
		}
	})
	t.Run("empty owner", func(t *testing.T) {
		t.Parallel()
		scope := " "
		service := &fakeObservationService{}
		handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
		recorder := httptest.NewRecorder()
		handler.GetWorkerSessionObservationByWorkerSessionId(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "legacy",
			factoryapi.GetWorkerSessionObservationByWorkerSessionIdParams{FactorySessionId: &scope})
		if recorder.Code != http.StatusBadRequest || service.getByWorkerCalled {
			t.Fatalf("status=%d lookup=%v", recorder.Code, service.getByWorkerCalled)
		}
	})
}

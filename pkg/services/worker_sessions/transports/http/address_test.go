package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

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
	ambiguous := workersessions.NewAmbiguousAddressError(candidates)
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

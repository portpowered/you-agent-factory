package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestWorkerSessionAddressErrorCLI(t *testing.T) {
	t.Parallel()
	work := "work-owner"
	candidates := []workersessions.AddressCandidate{
		{FactorySessionID: "owner", WorkerSessionID: "legacy", WorkID: &work, State: workersessions.StateRunning},
		{FactorySessionID: "peer", WorkerSessionID: "legacy", State: workersessions.StateCompleted},
	}
	ambiguous := (workersessions.AmbiguousAddressError{Candidates: candidates}).Clone()
	encoded, err := json.Marshal(struct {
		Code    string                                `json:"code"`
		Message string                                `json:"message"`
		Phase   string                                `json:"phase"`
		Details *workersessions.AmbiguousAddressError `json:"details"`
	}{"WORKER_SESSION_AMBIGUOUS", ambiguous.Error(), "VALIDATION", ambiguous})
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"show", "continue", "interrupt"} {
		for _, placement := range []string{"remote", "local"} {
			if operation == "show" && placement == "local" {
				continue
			}
			t.Run(operation+"/"+placement, func(t *testing.T) {
				t.Parallel()
				response := &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(string(encoded)))}
				mapped := mapWorkerAddressTestError(operation, placement, response, ambiguous)
				var output bytes.Buffer
				emitErr := emitWorkerAddressTestError(operation, &output, mapped)
				if !errors.Is(emitErr, mapped) {
					t.Fatalf("emitter changed error: %v", emitErr)
				}
				var payload struct {
					Code    string                               `json:"code"`
					Phase   string                               `json:"phase"`
					Details workersessions.AmbiguousAddressError `json:"details"`
				}
				if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Code != "WORKER_SESSION_AMBIGUOUS" || !reflect.DeepEqual(payload.Details.Candidates, candidates) {
					t.Fatalf("diagnostic = %s", output.String())
				}
				if operation == "interrupt" && payload.Phase != "VALIDATION" {
					t.Fatalf("phase = %q", payload.Phase)
				}
			})
		}
	}
}

func mapWorkerAddressTestError(operation, placement string, response *http.Response, ambiguous *workersessions.AmbiguousAddressError) error {
	switch operation {
	case "show":
		return workerSessionShowHTTPError(response, response.StatusCode)
	case "continue":
		if placement == "local" {
			return mapContinueServiceError(ambiguous, false)
		}
		return remoteContinueHTTPError(response, response.StatusCode)
	default:
		if placement == "local" {
			return mapInterruptServiceError(&workersessions.InterruptError{Phase: workersessions.InterruptPhaseValidation, Cause: ambiguous})
		}
		return remoteInterruptHTTPError(response, response.StatusCode)
	}
}

func emitWorkerAddressTestError(operation string, output *bytes.Buffer, err error) error {
	ctx := context.Background()
	switch operation {
	case "show":
		return emitShowCLIError(ShowConfig{Context: ctx, Output: output}, true, err)
	case "continue":
		return emitContinueCLIError(ContinueConfig{Context: ctx, Output: output}, true, err)
	default:
		return emitInterruptCLIError(InterruptConfig{Context: ctx, Output: output}, true, err)
	}
}

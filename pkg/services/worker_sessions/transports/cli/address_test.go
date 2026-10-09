package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestWorkerSessionGeneralErrorDetailsCLI(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"show", "continue", "interrupt"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			body := `{"code":"INTERNAL_ERROR","family":"INTERNAL_SERVER_ERROR","message":"server failure","details":"private-upstream-data"}`
			response := &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(body))}
			mapped := mapWorkerAddressTestError(operation, "remote", response, nil)
			var output bytes.Buffer
			_ = emitWorkerAddressTestError(operation, &output, mapped)
			var payload struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code != "INTERNAL_ERROR" || payload.Message != "server failure" || strings.Contains(output.String(), "private-upstream-data") {
				t.Fatalf("diagnostic = %s", output.String())
			}
		})
	}
}

func TestRemoteInterruptGeneralDetails(t *testing.T) {
	t.Parallel()
	for _, details := range []string{`"private-upstream-data"`, `{"candidates":[],"private":"private-upstream-data"}`, `["private-upstream-data"]`, `42`, `true`, `null`, ``} {
		for _, phase := range []string{"", "VALIDATION", "SOURCE_CANCELLATION", "SUCCESSOR_ADMISSION", "RESPONSE", "WAIT"} {
			t.Run(details+"/"+phase, func(t *testing.T) {
				t.Parallel()
				body := `{"code":"EXISTING_STRING_DETAILS_CODE","message":"Controlled legacy error","phase":"` + phase + `"`
				if details != "" {
					body += `,"details":` + details
				}
				response := &http.Response{Body: io.NopCloser(strings.NewReader(body + `}`))}
				mapped := remoteInterruptHTTPError(response, http.StatusConflict)
				var typed *CLIError
				if !errors.As(mapped, &typed) {
					t.Fatalf("error type = %T", mapped)
				}
				wantPhase := phase
				if wantPhase == "" {
					wantPhase = "VALIDATION"
				}
				if typed.Code != "EXISTING_STRING_DETAILS_CODE" || typed.Message != "Controlled legacy error" || typed.Phase != wantPhase || typed.Details != nil {
					t.Fatalf("error = %#v", typed)
				}
			})
		}
	}
}

func TestRemoteInterruptFallbacks(t *testing.T) {
	t.Parallel()
	statuses := []struct {
		status      int
		code, phase string
	}{
		{400, "BAD_REQUEST", "VALIDATION"},
		{404, "NOT_FOUND", "VALIDATION"},
		{409, "WORKER_SESSION_INTERRUPT_CONFLICT", "VALIDATION"},
		{503, "WORKER_SESSION_INTERRUPT_ADMISSION_FAILED", "SUCCESSOR_ADMISSION"},
		{500, "INTERNAL_ERROR", "SUCCESSOR_ADMISSION"},
	}
	for _, expected := range statuses {
		for _, body := range []string{`{private`, ``, `{}`, `{"message":" "}`, `{"message":"safe message"}`, `{"code":" ","phase":" ","message":"safe message"}`} {
			t.Run(expected.code+"/"+body, func(t *testing.T) {
				t.Parallel()
				mapped := remoteInterruptHTTPError(&http.Response{Body: io.NopCloser(strings.NewReader(body))}, expected.status)
				var typed *CLIError
				if !errors.As(mapped, &typed) || typed.Code != expected.code || typed.Phase != expected.phase || typed.Details != nil {
					t.Fatalf("error = %#v", mapped)
				}
				if strings.Contains(body, "safe message") {
					if typed.Message != "safe message" {
						t.Fatalf("message = %q", typed.Message)
					}
				} else if typed.Message != fmt.Sprintf("remote Worker Session interrupt failed (%d)", expected.status) {
					t.Fatalf("message = %q", typed.Message)
				}
			})
		}
	}
	for _, response := range []*http.Response{nil, {}, {Body: io.NopCloser(interruptErrorReader{})}} {
		mapped := remoteInterruptHTTPError(response, 409)
		var typed *CLIError
		if !errors.As(mapped, &typed) || typed.Code != "WORKER_SESSION_INTERRUPT_CONFLICT" || typed.Message != "remote Worker Session interrupt failed (409)" || typed.Phase != "VALIDATION" {
			t.Fatalf("missing/unreadable body = %v", mapped)
		}
	}
}

type interruptErrorReader struct{}

func (interruptErrorReader) Read([]byte) (int, error) { return 0, errors.New("private read error") }

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

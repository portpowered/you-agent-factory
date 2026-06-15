package sessionexecution_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func TestWriteExecutionError_JSONModeRendersDeterministicPayload(t *testing.T) {
	var output bytes.Buffer
	err := &sessionexecution.ExecutionError{
		Code:    sessionexecution.ErrorCodeUnsupportedMode,
		Message: `session execution mode "batch" is unsupported: use sync or async`,
		Field:   "mode",
	}
	if !sessionexecution.WriteExecutionError(&output, err, true) {
		t.Fatal("WriteExecutionError = false, want true")
	}

	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	}
	if decodeErr := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if payload.Code != sessionexecution.ErrorCodeUnsupportedMode {
		t.Fatalf("code = %q", payload.Code)
	}
	if payload.Field != "mode" {
		t.Fatalf("field = %q", payload.Field)
	}
	if !strings.Contains(payload.Message, "unsupported") {
		t.Fatalf("message = %q", payload.Message)
	}
}

func TestWriteExecutionError_HumanModeRendersStableLine(t *testing.T) {
	var output bytes.Buffer
	err := &sessionexecution.ExecutionError{
		Code:    sessionexecution.ErrorCodeMissingSource,
		Message: "session execution source is required",
		Field:   "source",
	}
	if !sessionexecution.WriteExecutionError(&output, err, false) {
		t.Fatal("WriteExecutionError = false, want true")
	}
	got := strings.TrimSpace(output.String())
	want := sessionexecution.ErrorCodeMissingSource + ": session execution source is required (source)"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestWriteExecutionError_MapsValidationErrors(t *testing.T) {
	var output bytes.Buffer
	err := factorysessionexecution.NewValidationError("requestId", "requestId is required")
	if !sessionexecution.WriteExecutionError(&output, err, true) {
		t.Fatal("WriteExecutionError = false, want true")
	}
	var payload struct {
		Code  string `json:"code"`
		Field string `json:"field"`
	}
	if decodeErr := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if payload.Code != sessionexecution.ErrorCodeValidation {
		t.Fatalf("code = %q", payload.Code)
	}
	if payload.Field != "requestId" {
		t.Fatalf("field = %q", payload.Field)
	}
}

func TestWriteExecutionError_ReturnsFalseForUnknownErrors(t *testing.T) {
	if sessionexecution.WriteExecutionError(nil, errors.New("other"), true) {
		t.Fatal("WriteExecutionError = true, want false")
	}
}

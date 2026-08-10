package clidiag

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNormalizeUnclassifiedFailureUsesSafeDiagnosticAndPreservesCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("payload=secret-token")
	normalized := Normalize(cause)
	var failure *Failure
	if !errors.As(normalized, &failure) {
		t.Fatalf("normalized error = %T, want *Failure", normalized)
	}
	if failure.Code != DefaultFailureCode || failure.Message != defaultFailureMessage {
		t.Fatalf("fallback = %#v, want stable safe fields", failure)
	}
	if !errors.Is(normalized, cause) {
		t.Fatalf("normalized error = %v, want to preserve cause", normalized)
	}

	var output bytes.Buffer
	if !WriteFailure(&output, normalized) {
		t.Fatal("WriteFailure returned false for normalized failure")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("diagnostic is not one ErrorResponse: %v; output=%q", err, output.String())
	}
	if response.Code != factoryapi.ErrorResponseCode(DefaultFailureCode) ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Message != defaultFailureMessage {
		t.Fatalf("response = %#v, want stable safe fallback", response)
	}
	if strings.Contains(output.String(), "secret-token") {
		t.Fatalf("diagnostic leaked cause: %q", output.String())
	}
}

type testCodedError struct {
	cause error
}

func (err testCodedError) Error() string { return "coded: " + err.cause.Error() }

func (err testCodedError) Unwrap() error { return err.cause }

func (testCodedError) CLIErrorCode() string    { return "TEST_COMMAND_REJECTED" }
func (testCodedError) CLIErrorMessage() string { return "the command was rejected" }

func TestNormalizePreservesAuthoredCodedError(t *testing.T) {
	t.Parallel()

	cause := errors.New("underlying detail")
	authored := testCodedError{cause: cause}
	if got := Normalize(authored); !errors.Is(got, authored) {
		t.Fatalf("Normalize returned %T, want authored error", got)
	}

	writer := NewDiagnosticWriter(&bytes.Buffer{})
	if !WriteFailure(writer, authored) {
		t.Fatal("WriteFailure did not recognize authored coded error")
	}
	if !writer.DiagnosticRendered() {
		t.Fatal("WriteFailure did not mark diagnostic writer")
	}
}

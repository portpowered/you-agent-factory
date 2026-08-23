package clidiag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type testInvocationCodedError struct {
	code    string
	message string
}

func (err testInvocationCodedError) Error() string { return "invocation: " + err.code }

func (err testInvocationCodedError) InvocationErrorCode() string { return err.code }

func (err testInvocationCodedError) InvocationErrorMessage() string { return err.message }

type testResponseCodedError struct {
	response factoryapi.ErrorResponse
}

func (err testResponseCodedError) Error() string { return err.response.Message }

func (err testResponseCodedError) CLIErrorResponse() factoryapi.ErrorResponse { return err.response }

func TestWriteFailurePreservesDecodedServerResponse(t *testing.T) {
	t.Parallel()

	authored := testResponseCodedError{response: factoryapi.ErrorResponse{
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		Family:  factoryapi.ErrorFamilyNotFound,
		Message: "server supplied message",
	}}
	var output bytes.Buffer
	if !WriteFailure(&output, authored) {
		t.Fatal("WriteFailure did not recognize response-coded failure")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode response-coded diagnostic: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeNOTFOUND || response.Family != factoryapi.ErrorFamilyNotFound || response.Message != "server supplied message" {
		t.Fatalf("response = %#v, want decoded server fields", response)
	}
}

func TestFailureNilAndDefaultEdges(t *testing.T) {
	t.Parallel()

	var nilFailure *Failure
	if nilFailure.Error() != "" || nilFailure.Unwrap() != nil {
		t.Fatalf("nil Failure methods = (%q, %v), want empty/nil", nilFailure.Error(), nilFailure.Unwrap())
	}
	if nilFailure.CLIErrorCode() != DefaultFailureCode || nilFailure.CLIErrorMessage() != defaultFailureMessage {
		t.Fatalf("nil Failure diagnostic fields = (%q, %q), want defaults", nilFailure.CLIErrorCode(), nilFailure.CLIErrorMessage())
	}

	cause := errors.New("private cause")
	failure := &Failure{Cause: cause}
	if got := failure.Error(); got != DefaultFailureCode+": "+defaultFailureMessage+": private cause" {
		t.Fatalf("default Failure error = %q", got)
	}
	if !errors.Is(failure, cause) {
		t.Fatal("default Failure did not unwrap its cause")
	}
}

func TestDiagnosticWriterCoversMarkersAndNilOutputs(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	writer := NewDiagnosticWriter(&output)
	if n, err := writer.Write([]byte("prefix")); err != nil || n != len("prefix") || output.String() != "prefix" {
		t.Fatalf("DiagnosticWriter.Write = (%d, %v), output=%q", n, err, output.String())
	}
	if DiagnosticRendered(writer) {
		t.Fatal("new DiagnosticWriter unexpectedly marked rendered")
	}
	MarkDiagnosticRendered(writer)
	if !writer.DiagnosticRendered() || !DiagnosticRendered(writer) {
		t.Fatal("DiagnosticWriter marker was not visible through the writer helpers")
	}
	MarkDiagnosticRendered(&output)
	if DiagnosticRendered(&output) {
		t.Fatal("plain writer unexpectedly exposed a diagnostic marker")
	}

	var nilWriter *DiagnosticWriter
	if n, err := nilWriter.Write([]byte("ignored")); err != nil || n != len("ignored") {
		t.Fatalf("nil DiagnosticWriter.Write = (%d, %v), want all bytes/nil", n, err)
	}
	if n, err := NewDiagnosticWriter(nil).Write([]byte("ignored")); err != nil || n != len("ignored") {
		t.Fatalf("nil-output DiagnosticWriter.Write = (%d, %v), want all bytes/nil", n, err)
	}
}

func TestDiagnosticContextAndPresentationHelpers(t *testing.T) {
	t.Parallel()

	if CentralDiagnosticsEnabled(nil) {
		t.Fatal("nil context unexpectedly enabled central diagnostics")
	}
	if WithCentralDiagnostics(nil, true) != nil {
		t.Fatal("WithCentralDiagnostics(nil, true) created an implicit context")
	}
	if CentralDiagnosticsEnabled(WithCentralDiagnostics(context.Background(), false)) {
		t.Fatal("WithCentralDiagnostics(false) unexpectedly enabled central diagnostics")
	}

	var output bytes.Buffer
	Printf(&output, false, "hidden")
	Printf(&output, true, "visible %s", "diagnostic")
	Printf(nil, true, "discarded")
	if output.String() != "visible diagnostic\n" {
		t.Fatalf("Printf output = %q", output.String())
	}

	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "", want: DefaultSessionID},
		{input: "session-1", want: "session-1"},
	} {
		if got := SessionLabel(test.input); got != test.want {
			t.Fatalf("SessionLabel(%q) = %q, want %q", test.input, got, test.want)
		}
	}
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "factory.json", want: "json"},
		{input: "factory.MARKDOWN", want: "markdown"},
		{input: "factory.txt", want: "file"},
	} {
		if got := PayloadType(test.input); got != test.want {
			t.Fatalf("PayloadType(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestNormalizeAndWriteFailureCoverInvocationAndInvalidContracts(t *testing.T) {
	t.Parallel()

	if Normalize(nil) != nil {
		t.Fatal("Normalize(nil) returned a failure")
	}

	authored := testInvocationCodedError{code: "INVOCATION_REJECTED", message: "safe invocation rejection"}
	if got := Normalize(authored); !errors.Is(got, authored) {
		t.Fatalf("Normalize(authored invocation) = %T, want original error", got)
	}
	var output bytes.Buffer
	if !WriteFailure(&output, authored) {
		t.Fatal("WriteFailure did not recognize an authored invocation error")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode authored invocation diagnostic: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCode("INVOCATION_REJECTED") || response.Message != "safe invocation rejection" {
		t.Fatalf("authored invocation response = %#v", response)
	}

	invalid := testInvocationCodedError{message: "ignored"}
	normalized := Normalize(invalid)
	var failure *Failure
	if !errors.As(normalized, &failure) || failure.Code != DefaultFailureCode {
		t.Fatalf("invalid authored code normalized to %T (%v), want safe Failure", normalized, normalized)
	}
	if WriteFailure(nil, normalized) != true {
		t.Fatal("WriteFailure(nil, normalized) = false, want true")
	}
	if WriteFailure(&output, errors.New("untyped")) {
		t.Fatal("WriteFailure recognized an untyped error")
	}
}

func TestWriteUsageErrorPreservesCobraTextAndHelpPath(t *testing.T) {
	t.Parallel()

	cause := errors.New(`unknown flag: --not-a-real-flag`)
	usage := NewUsageError("you session show", cause)
	if usage == nil || !errors.Is(usage, cause) {
		t.Fatalf("NewUsageError() = %v, want wrapped Cobra cause", usage)
	}
	var output bytes.Buffer
	if !WriteUsageError(&output, usage) {
		t.Fatal("WriteUsageError returned false for a usage error")
	}
	if got, want := output.String(), "Error: unknown flag: --not-a-real-flag\nRun 'you session show --help' for usage.\n"; got != want {
		t.Fatalf("usage diagnostic = %q, want %q", got, want)
	}
	if DiagnosticRendered(&output) {
		t.Fatal("plain output unexpectedly exposed diagnostic marker")
	}
	writer := NewDiagnosticWriter(&output)
	if WriteUsageError(writer, usage) == false || !writer.DiagnosticRendered() {
		t.Fatal("usage diagnostic writer was not marked rendered")
	}
	if WriteFailure(&output, usage) {
		t.Fatal("usage error was incorrectly rendered as a JSON failure")
	}
}

func TestNewUsageErrorRetainsExistingMetadata(t *testing.T) {
	t.Parallel()

	original := NewUsageError("you work show", errors.New("requires at least 1 arg(s), only received 0"))
	wrapped := NewUsageError("you", fmt.Errorf("outer: %w", original))
	var usage *UsageError
	if !errors.As(wrapped, &usage) {
		t.Fatalf("wrapped usage error = %v, want UsageError", wrapped)
	}
	if usage.CommandPath != "you work show" {
		t.Fatalf("usage command path = %q, want original path", usage.CommandPath)
	}
}

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

func TestLocalFailuresPreserveActionableContextWithoutFilesystemCause(t *testing.T) {
	t.Parallel()

	cause := errors.New(`open ./missing.json: credential=secret-token: no such file or directory`)
	local := NewLocalInputFailure("--replay", "./missing.json", cause)
	if got, want := local.Error(), `failed to load --replay input "./missing.json"`; got != want {
		t.Fatalf("local error = %q, want %q", got, want)
	}
	if !errors.Is(local, cause) {
		t.Fatal("local failure did not preserve its cause")
	}
	if !HasCodedDiagnostic(local) {
		t.Fatal("local failure did not expose a coded diagnostic")
	}

	var output bytes.Buffer
	if !WriteFailure(&output, local) {
		t.Fatal("WriteFailure returned false for local failure")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("local diagnostic is not one ErrorResponse: %v; output=%q", err, output.String())
	}
	if response.Code != factoryapi.ErrorResponseCode(LocalInputFailureCode) ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Message != `failed to load --replay input "./missing.json"` {
		t.Fatalf("local response = %#v, want actionable bad-request fields", response)
	}
	if strings.Contains(output.String(), "secret-token") || strings.Contains(output.String(), "no such file") {
		t.Fatalf("local diagnostic leaked filesystem cause: %q", output.String())
	}
}

func TestFlagConflictFailureNamesBothFlagsAndUsesBadRequestFamily(t *testing.T) {
	t.Parallel()

	local := NewFlagConflictFailure("--resume", "--no-record", nil)
	if got, want := local.Error(), "--resume cannot be used with --no-record"; got != want {
		t.Fatalf("flag conflict = %q, want %q", got, want)
	}
	var output bytes.Buffer
	if !WriteFailure(&output, local) {
		t.Fatal("WriteFailure returned false for flag conflict")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("flag conflict diagnostic is not one ErrorResponse: %v; output=%q", err, output.String())
	}
	if response.Code != factoryapi.ErrorResponseCode(FlagConflictFailureCode) ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Message != "--resume cannot be used with --no-record" {
		t.Fatalf("flag conflict response = %#v, want named bad-request fields", response)
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

type testHTTPDiagnosticError struct {
	method string
	url    string
	status int
	cause  error
}

func (err *testHTTPDiagnosticError) Error() string {
	if err == nil || err.cause == nil {
		return "HTTP request failed"
	}
	return err.cause.Error()
}

func (err *testHTTPDiagnosticError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *testHTTPDiagnosticError) CLIHTTPMethod() string { return err.method }
func (err *testHTTPDiagnosticError) CLIHTTPURL() string    { return err.url }
func (err *testHTTPDiagnosticError) CLIHTTPStatus() int    { return err.status }

func TestWriteDebugFailureRedactsCauseAndHTTPSecrets(t *testing.T) {
	t.Parallel()

	err := &testHTTPDiagnosticError{
		method: "POST",
		url:    "https://user:password@example.test/factory?token=query-secret#private",
		status: 422,
		cause: fmt.Errorf(
			"send request: %w",
			errors.New("authorization=Bearer header-secret payload=body-secret"),
		),
	}
	var output bytes.Buffer
	if !WriteDebugFailure(&output, err) {
		t.Fatal("WriteDebugFailure returned false")
	}
	text := output.String()
	for _, want := range []string{
		"debug: cause[0]=send request:",
		"debug: cause[1]=authorization=<redacted> payload=<redacted>",
		"debug: http method=POST",
		"url=https://example.test/factory",
		"status=422",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("debug output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{"password", "query-secret", "header-secret", "body-secret", "#private"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("debug output leaked %q: %q", forbidden, text)
		}
	}
}

func TestWriteDebugFailureRedactsPromptsBodiesAndUnrestrictedPaths(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf(
		"invoke failed: %w",
		errors.New(`authorization=Bearer private-token prompt="PRIVATE_PROMPT" body="PRIVATE_BODY" cachePath="C:\Users\andre\AppData\Local\you\cache\model.gguf" open ./private/model.gguf`),
	)
	var output bytes.Buffer
	if !WriteDebugFailure(&output, err) {
		t.Fatal("WriteDebugFailure returned false")
	}
	text := output.String()
	for _, forbidden := range []string{
		"private-token", "PRIVATE_PROMPT", "PRIVATE_BODY", "C:\\Users\\andre", "./private/model.gguf",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("debug output leaked %q: %q", forbidden, text)
		}
	}
	for _, want := range []string{"prompt=<redacted>", "body=<redacted>", "cachePath=<redacted>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("debug output = %q, want %q", text, want)
		}
	}
}

func TestDebugFlagEnabledHonorsExplicitCLIValuesAndArgumentBoundary(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "long flag", args: []string{"session", "show", "--debug"}, want: true},
		{name: "short flag", args: []string{"-d", "session", "show"}, want: true},
		{name: "explicit false", args: []string{"--debug", "--debug=false"}, want: false},
		{name: "after separator is positional", args: []string{"run", "--", "--debug"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := DebugFlagEnabled(test.args); got != test.want {
				t.Fatalf("DebugFlagEnabled(%v) = %t, want %t", test.args, got, test.want)
			}
		})
	}
}

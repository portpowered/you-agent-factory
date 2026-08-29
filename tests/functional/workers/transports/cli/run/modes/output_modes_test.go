package modes_test

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	wantPrimaryResult                  = "deterministic workers primary COMPLETE"
	deterministicProviderFailureExit   = 7
	deterministicProviderFailureStderr = "deterministic provider rejection"
	factoryEventRecordType             = "factory_event"
	invocationResultType               = "invocation_result"
)

// TestCLIRunSuccessPrimaryResultTextJSONAndNDJSON proves a successful public
// you run invocation writes the same deterministic primary result across quiet
// text, single-JSON, and NDJSON response-stream presentations at the CLI boundary.
func TestCLIRunSuccessPrimaryResultTextJSONAndNDJSON(t *testing.T) {
	t.Run("quiet text primary result", func(t *testing.T) {
		observations := executeOutputModeCharacterizationSequence(t)
		result := observations["quiet"]
		if strings.TrimSpace(result.stdout) != wantPrimaryResult {
			t.Fatalf("stdout = %q, want raw primary result %q", result.stdout, wantPrimaryResult)
		}
		assertSuccessfulRunStderrEmpty(t, result.stderr)
		assertOutputModeCharacterization(t, observations)
	})

	t.Run("default JSON response stream", func(t *testing.T) {
		result := executeSuccessfulRun(t, []string{"--json"}, nil)
		response := decodeTerminalNDJSONInvocationResult(t, result.stdout).Response
		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
		}
		assertInvocationPrimaryResultText(t, response, wantPrimaryResult)
		assertSuccessfulRunStderrEmpty(t, result.stderr)
	})

	t.Run("NDJSON response-stream invocation_result", func(t *testing.T) {
		result := executeSuccessfulRun(t, []string{"--json"}, []string{"--output", "response-stream"})
		terminal := decodeTerminalNDJSONInvocationResult(t, result.stdout)
		if terminal.RecordType != invocationResultType {
			t.Fatalf("terminal recordType = %q, want %q", terminal.RecordType, invocationResultType)
		}
		if terminal.Response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("status = %q, want %q", terminal.Response.Status, factoryapi.InvocationTerminalStatusCompleted)
		}
		assertInvocationPrimaryResultText(t, terminal.Response, wantPrimaryResult)
		assertSuccessfulRunStderrEmpty(t, result.stderr)
	})
}

// TestCLIRunFailureOmitsFalseSuccessPrimaryResult proves a failed public you run
// invocation never writes a completed-success primary result to stdout in quiet
// text or machine-readable presentations, while stderr still follows the documented
// structured failure contract for JSON modes.
func TestCLIRunFailureOmitsFalseSuccessPrimaryResult(t *testing.T) {
	t.Run("quiet text omits success primary result", func(t *testing.T) {
		result, err := executeFailedRun(t, nil, []string{"--quiet"})
		if err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		assertSingleFailedProviderCall(t, result)
		if strings.TrimSpace(result.stdout) != "" {
			t.Fatalf("stdout = %q, want empty quiet failure stdout without false primary result", result.stdout)
		}
		errorResponse := decodeSingleErrorResponse(t, result.stderr)
		if errorResponse.Code != factoryapi.ErrorResponseCode("INVOCATION_RUNTIME_FAILURE") ||
			errorResponse.Family != factoryapi.ErrorFamilyInternalServerError ||
			strings.TrimSpace(errorResponse.Message) == "" {
			t.Fatalf("quiet failure ErrorResponse = %#v, want one actionable INVOCATION_RUNTIME_FAILURE response", errorResponse)
		}
	})

	t.Run("default JSON failed response stream", func(t *testing.T) {
		result, err := executeFailedRun(t, []string{"--json"}, nil)
		if err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		assertSingleFailedProviderCall(t, result)
		response := decodeTerminalNDJSONInvocationResult(t, result.stdout).Response
		assertFailedInvocationResponse(t, response)
		assertFailedRunErrorResponse(t, result.stderr, response)
		if invocationPrimaryResultPresent(response) {
			t.Fatalf("primaryResult = %#v, want no success primary result on terminal failure", response.PrimaryResult)
		}
	})

	t.Run("NDJSON response-stream failed invocation_result", func(t *testing.T) {
		result, err := executeFailedRun(t, []string{"--json"}, []string{"--output", "response-stream"})
		if err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		assertSingleFailedProviderCall(t, result)
		terminal := decodeTerminalNDJSONInvocationResult(t, result.stdout)
		if terminal.RecordType != invocationResultType {
			t.Fatalf("terminal recordType = %q, want %q", terminal.RecordType, invocationResultType)
		}
		assertFailedInvocationResponse(t, terminal.Response)
		assertFailedRunErrorResponse(t, result.stderr, terminal.Response)
		if invocationPrimaryResultPresent(terminal.Response) {
			t.Fatalf("primaryResult = %#v, want no success primary result on terminal failure", terminal.Response.PrimaryResult)
		}
	})
}

type successfulRunResult struct {
	stdout string
	stderr string
}

type runCharacterizationCase struct {
	name       string
	globalArgs []string
	runArgs    []string
}

type runCharacterizationObservation struct {
	stdout        string
	stderr        string
	err           error
	providerCalls int
}

func assertOutputModeCharacterization(
	t *testing.T,
	observations map[string]runCharacterizationObservation,
) {
	t.Helper()
	assertDuplicateOutputModes(t, observations)
	assertOutputModePrecedence(t, observations)
	assertOutputModeNormalization(t, observations)
	assertJSONFlagPlacement(t, observations)
}

func assertDuplicateOutputModes(
	t *testing.T,
	observations map[string]runCharacterizationObservation,
) {
	t.Helper()
	quiet := observations["quiet"]
	duplicateQuiet := observations["duplicate quiet"]
	assertSuccessfulCharacterization(t, "duplicate --quiet", duplicateQuiet)
	if duplicateQuiet.stdout != quiet.stdout || duplicateQuiet.stderr != quiet.stderr {
		t.Fatalf(
			"duplicate --quiet output = %q/%q, want %q/%q",
			duplicateQuiet.stdout,
			duplicateQuiet.stderr,
			quiet.stdout,
			quiet.stderr,
		)
	}

	duplicateJSON := observations["duplicate json"]
	assertSuccessfulCharacterization(t, "duplicate --json", duplicateJSON)
	assertInvocationPrimaryResultText(
		t,
		decodeTerminalNDJSONInvocationResult(t, duplicateJSON.stdout).Response,
		wantPrimaryResult,
	)

	duplicatePrimary := observations["duplicate primary"]
	assertSuccessfulCharacterization(t, "duplicate --output primary", duplicatePrimary)
	if strings.TrimSpace(duplicatePrimary.stdout) != wantPrimaryResult {
		t.Fatalf("duplicate --output primary stdout = %q, want %q", duplicatePrimary.stdout, wantPrimaryResult)
	}
}

func assertOutputModePrecedence(
	t *testing.T,
	observations map[string]runCharacterizationObservation,
) {
	t.Helper()
	primaryThenStream := observations["primary then response-stream"]
	assertSuccessfulCharacterization(t, "duplicate output primary then response-stream", primaryThenStream)
	assertInvocationPrimaryResultText(
		t,
		decodeTerminalNDJSONInvocationResult(t, primaryThenStream.stdout).Response,
		wantPrimaryResult,
	)

	streamThenPrimary := observations["response-stream then primary"]
	assertSuccessfulCharacterization(t, "duplicate output response-stream then primary", streamThenPrimary)
	assertInvocationPrimaryResultText(
		t,
		decodeInvocationResponse(t, streamThenPrimary.stdout),
		wantPrimaryResult,
	)
}

func assertOutputModeNormalization(
	t *testing.T,
	observations map[string]runCharacterizationObservation,
) {
	t.Helper()
	whitespacePrimary := observations["whitespace primary"]
	assertSuccessfulCharacterization(t, "whitespace primary", whitespacePrimary)
	if strings.TrimSpace(whitespacePrimary.stdout) != wantPrimaryResult {
		t.Fatalf("whitespace primary stdout = %q, want %q", whitespacePrimary.stdout, wantPrimaryResult)
	}

	whitespaceStream := observations["whitespace response-stream"]
	assertSuccessfulCharacterization(t, "whitespace response-stream", whitespaceStream)
	assertInvocationPrimaryResultText(
		t,
		decodeTerminalNDJSONInvocationResult(t, whitespaceStream.stdout).Response,
		wantPrimaryResult,
	)

	uppercasePrimary := observations["uppercase primary"]
	if uppercasePrimary.err == nil || !strings.Contains(uppercasePrimary.err.Error(), "INVOCATION_OUTPUT_UNSUPPORTED") {
		t.Fatalf("uppercase primary error = %v, want unsupported output error", uppercasePrimary.err)
	}
	if uppercasePrimary.stdout != "" {
		t.Fatalf("uppercase primary stdout = %q, want empty", uppercasePrimary.stdout)
	}
	uppercaseError := decodeSingleErrorResponse(t, uppercasePrimary.stderr)
	if uppercaseError.Code != factoryapi.ErrorResponseCode("INVOCATION_OUTPUT_UNSUPPORTED") ||
		uppercaseError.Family != factoryapi.ErrorFamilyBadRequest ||
		uppercaseError.Message != `unsupported --output value "PRIMARY"; supported values are primary (default) and response-stream` {
		t.Fatalf("uppercase primary ErrorResponse = %#v, want exact unsupported-value diagnostic", uppercaseError)
	}
	if uppercasePrimary.providerCalls != 0 {
		t.Fatalf("uppercase primary provider calls = %d, want zero before dispatch", uppercasePrimary.providerCalls)
	}
}

func assertJSONFlagPlacement(
	t *testing.T,
	observations map[string]runCharacterizationObservation,
) {
	t.Helper()
	jsonBeforeRun := observations["json before run"]
	jsonAfterRun := observations["json after run"]
	for name, observation := range map[string]runCharacterizationObservation{
		"json before run": jsonBeforeRun,
		"json after run":  jsonAfterRun,
	} {
		assertSuccessfulCharacterization(t, name, observation)
	}
	assertEquivalentInvocationResponses(
		t,
		decodeTerminalNDJSONInvocationResult(t, jsonBeforeRun.stdout).Response,
		decodeTerminalNDJSONInvocationResult(t, jsonAfterRun.stdout).Response,
	)
}

func assertSuccessfulCharacterization(
	t *testing.T,
	name string,
	observation runCharacterizationObservation,
) {
	t.Helper()
	if observation.err != nil {
		t.Fatalf("%s error = %v", name, observation.err)
	}
	assertSuccessfulRunStderrEmpty(t, observation.stderr)
}

func executeOutputModeCharacterizationSequence(t *testing.T) map[string]runCharacterizationObservation {
	t.Helper()
	fixture := modesFixture(t)

	cases := []runCharacterizationCase{
		{name: "quiet", runArgs: []string{"--quiet"}},
		{name: "duplicate quiet", runArgs: []string{"--quiet", "--quiet"}},
		{name: "duplicate json", globalArgs: []string{"--json", "--json"}},
		{name: "duplicate primary", runArgs: []string{"--output", "primary", "--output", "primary"}},
		{
			name:       "primary then response-stream",
			globalArgs: []string{"--json"},
			runArgs:    []string{"--output", "primary", "--output", "response-stream"},
		},
		{
			name:       "response-stream then primary",
			globalArgs: []string{"--json"},
			runArgs:    []string{"--output", "response-stream", "--output", "primary"},
		},
		{name: "whitespace primary", runArgs: []string{"--output", "  primary  "}},
		{
			name:       "whitespace response-stream",
			globalArgs: []string{"--json"},
			runArgs:    []string{"--output", "  response-stream  "},
		},
		{name: "uppercase primary", runArgs: []string{"--output", "PRIMARY"}},
		{name: "json before run", globalArgs: []string{"--json"}},
		{name: "json after run", runArgs: []string{"--json"}},
	}

	observations := make(map[string]runCharacterizationObservation, len(cases))
	for _, testCase := range cases {
		result := fixture.execute(t, modesInvocationSpec{
			globalArgs:    testCase.globalArgs,
			runArgs:       testCase.runArgs,
			prompt:        "prove workers-owned output characterization",
			includePrompt: true,
			behavior:      modesRouteSuccess,
		})
		observations[testCase.name] = runCharacterizationObservation{
			stdout:        result.stdout,
			stderr:        result.stderr,
			err:           result.err,
			providerCalls: result.providerCalls,
		}
	}
	return observations
}

func executeSuccessfulRun(t *testing.T, globalArgs, runArgs []string) successfulRunResult {
	t.Helper()
	result := modesFixture(t).execute(t, modesInvocationSpec{
		globalArgs:    globalArgs,
		runArgs:       runArgs,
		prompt:        "prove workers-owned output modes",
		includePrompt: true,
		behavior:      modesRouteSuccess,
	})
	if result.err != nil {
		t.Fatalf("Process.Execute(output modes) error = %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
	}
	return successfulRunResult{stdout: result.stdout, stderr: result.stderr}
}

type failedRunResult struct {
	stdout        string
	stderr        string
	providerCalls int
}

func executeFailedRun(t *testing.T, globalArgs, runArgs []string) (failedRunResult, error) {
	t.Helper()
	result := modesFixture(t).execute(t, modesInvocationSpec{
		globalArgs:    globalArgs,
		runArgs:       runArgs,
		prompt:        "prove workers-owned failure output modes",
		includePrompt: true,
		behavior:      modesRouteFailure,
	})
	return failedRunResult{
		stdout:        result.stdout,
		stderr:        result.stderr,
		providerCalls: result.providerCalls,
	}, result.err
}

func assertSingleFailedProviderCall(t *testing.T, result failedRunResult) {
	t.Helper()
	if result.providerCalls != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one failed dispatch", result.providerCalls)
	}
}

func assertSuccessfulRunStderrEmpty(t *testing.T, stderr string) {
	t.Helper()
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", stderr)
	}
}

type ndjsonTerminalInvocationResult struct {
	RecordType string
	Response   factoryapi.InvocationResponse
}

func decodeTerminalNDJSONInvocationResult(t *testing.T, stdout string) ndjsonTerminalInvocationResult {
	t.Helper()
	lines := nonEmptyLines(stdout)
	if len(lines) == 0 {
		t.Fatalf("stdout empty, want NDJSON stream ending in invocation_result\nstdout:\n%s", stdout)
	}

	var terminal ndjsonTerminalInvocationResult
	terminalCount := 0
	for index, line := range lines {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("decode NDJSON record %d: %v\nline: %s", index, err, line)
		}
		var recordType string
		if err := json.Unmarshal(fields["recordType"], &recordType); err != nil {
			t.Fatalf("decode recordType for record %d: %v\nline: %s", index, err, line)
		}
		if recordType != invocationResultType {
			if recordType != factoryEventRecordType {
				t.Fatalf("record %d recordType = %q, want %q or %q", index, recordType, factoryEventRecordType, invocationResultType)
			}
			continue
		}
		terminalCount++
		if terminalCount > 1 {
			t.Fatalf("stdout contains %d invocation_result records, want exactly one\nstdout:\n%s", terminalCount, stdout)
		}
		if index != len(lines)-1 {
			t.Fatalf("invocation_result record index = %d, want terminal index %d", index, len(lines)-1)
		}
		if err := json.Unmarshal(fields["response"], &terminal.Response); err != nil {
			t.Fatalf("decode terminal invocation result %d: %v\nline: %s", index, err, line)
		}
		terminal.RecordType = recordType
	}
	if terminalCount != 1 || terminal.RecordType == "" {
		t.Fatalf("stdout missing terminal invocation_result\nstdout:\n%s", stdout)
	}
	return terminal
}

func decodeInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var response factoryapi.InvocationResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode single InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stdout contains data after single InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	return response
}

func assertEquivalentInvocationResponses(
	t *testing.T,
	before, after factoryapi.InvocationResponse,
) {
	t.Helper()
	if before.Status != after.Status {
		t.Fatalf("JSON placement statuses = %q/%q, want equivalent", before.Status, after.Status)
	}
	beforeWorkName := invocationResponseString(before.WorkName)
	beforeWorkState := invocationResponseString(before.WorkState)
	afterWorkName := invocationResponseString(after.WorkName)
	afterWorkState := invocationResponseString(after.WorkState)
	if beforeWorkName != afterWorkName || beforeWorkState != afterWorkState {
		t.Fatalf(
			"JSON placement work identity = %q/%q and %q/%q, want equivalent",
			beforeWorkName,
			beforeWorkState,
			afterWorkName,
			afterWorkState,
		)
	}
	if before.PrimaryResult == nil || after.PrimaryResult == nil {
		t.Fatalf("JSON placement primary results = %#v/%#v, want equivalent completed content", before.PrimaryResult, after.PrimaryResult)
	}
	assertInvocationPrimaryResultText(t, before, wantPrimaryResult)
	assertInvocationPrimaryResultText(t, after, wantPrimaryResult)
}

func invocationResponseString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func assertFailedInvocationResponse(t *testing.T, response factoryapi.InvocationResponse) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusFailed)
	}
	if response.ErrorCode == nil || response.Message == nil {
		t.Fatalf("failed InvocationResponse lacks error detail: %#v", response)
	}
}

func assertFailedRunErrorResponse(
	t *testing.T,
	stderr string,
	response factoryapi.InvocationResponse,
) {
	t.Helper()
	errorResponse := decodeSingleErrorResponse(t, stderr)
	if errorResponse.Code != factoryapi.ErrorResponseCode(*response.ErrorCode) ||
		errorResponse.Family != factoryapi.ErrorFamilyInternalServerError ||
		!strings.HasPrefix(errorResponse.Message, *response.Message) {
		t.Fatalf("ErrorResponse = %#v, want code %s and message prefix %q", errorResponse, *response.ErrorCode, *response.Message)
	}
}

func decodeSingleErrorResponse(t *testing.T, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stderr))
	var response factoryapi.ErrorResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stderr contains data after ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	return response
}

func invocationPrimaryResultPresent(response factoryapi.InvocationResponse) bool {
	return response.PrimaryResult != nil && len(*response.PrimaryResult) > 0
}

func assertInvocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse, want string) {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text content part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text content: %v", err)
	}
	if part.Text != want {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, want)
	}
}

func nonEmptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

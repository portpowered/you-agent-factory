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
	t.Parallel()
	t.Run("quiet text primary result", func(t *testing.T) {
		result := executeSuccessfulRun(t, nil, []string{"--quiet"})
		if strings.TrimSpace(result.stdout) != wantPrimaryResult {
			t.Fatalf("stdout = %q, want raw primary result %q", result.stdout, wantPrimaryResult)
		}
		assertSuccessfulRunStderrEmpty(t, result.stderr)
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
	t.Parallel()
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

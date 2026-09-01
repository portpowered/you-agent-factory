package modes_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCLIRunPresentationModesCharacterization(t *testing.T) {
	cases := []struct {
		name       string
		globalArgs []string
		runArgs    []string
	}{
		{name: "default human response stream"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := modesFixture(t).execute(t, modesInvocationSpec{
				globalArgs:    testCase.globalArgs,
				runArgs:       testCase.runArgs,
				prompt:        "prove every workers output presentation",
				includePrompt: true,
				behavior:      modesRouteSuccess,
			})
			if result.err != nil {
				t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
			}
			assertSuccessfulPresentation(t, testCase.name, result)
		})
	}
}

func assertSuccessfulPresentation(t *testing.T, name string, result modesInvocationResult) {
	t.Helper()
	if result.providerCalls != 1 {
		t.Fatalf("provider calls = %d, want one successful dispatch", result.providerCalls)
	}
	switch name {
	case "default human response stream", "human response stream":
		assertHumanResponseStream(t, result.stdout)
		if strings.TrimSpace(result.stderr) == "" {
			t.Fatal("human response-stream stderr is empty, want worker progress diagnostics")
		}
	case "primary text", "quiet primary text":
		if strings.TrimSpace(result.stdout) != wantPrimaryResult {
			t.Fatalf("stdout = %q, want raw primary result %q", result.stdout, wantPrimaryResult)
		}
		if result.stderr != "" {
			t.Fatalf("stderr = %q, want empty primary-result diagnostics", result.stderr)
		}
	case "JSON primary":
		assertInvocationPrimaryResultText(t, decodeInvocationResponse(t, result.stdout), wantPrimaryResult)
		if result.stderr != "" {
			t.Fatalf("stderr = %q, want empty JSON diagnostics", result.stderr)
		}
	case "JSON factory events", "JSON response stream":
		terminal := decodeTerminalNDJSONInvocationResult(t, result.stdout)
		if terminal.RecordType != invocationResultType {
			t.Fatalf("terminal recordType = %q, want %q", terminal.RecordType, invocationResultType)
		}
		assertInvocationPrimaryResultText(t, terminal.Response, wantPrimaryResult)
		if result.stderr != "" {
			t.Fatalf("stderr = %q, want empty JSON diagnostics", result.stderr)
		}
	default:
		t.Fatalf("unhandled presentation %q", name)
	}
}

func assertHumanResponseStream(t *testing.T, stdout string) {
	t.Helper()
	markers := []string{
		"[0] factory started",
		"[1] work accepted:",
		"[2] workstation started:",
		"workstation completed:",
		"--- primary result ---",
		wantPrimaryResult,
	}
	position := -1
	for _, marker := range markers {
		next := strings.Index(stdout, marker)
		if next <= position {
			t.Fatalf("human stdout marker %q at %d after %d:\n%s", marker, next, position, stdout)
		}
		position = next
	}
}

func TestCLIRunFailurePresentationModesCharacterization(t *testing.T) {
	cases := []struct {
		name       string
		globalArgs []string
		runArgs    []string
	}{
		{name: "default human"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := modesFixture(t).execute(t, modesInvocationSpec{
				globalArgs:    testCase.globalArgs,
				runArgs:       testCase.runArgs,
				prompt:        "prove failure has one terminal outcome",
				includePrompt: true,
				behavior:      modesRouteFailure,
			})
			if result.err == nil {
				t.Fatal("Process.Execute() error = nil, want provider failure")
			}
			if result.providerCalls != 1 {
				t.Fatalf("provider calls = %d, want one", result.providerCalls)
			}
			assertFailedPresentation(t, testCase.name, result)
		})
	}
}

func assertFailedPresentation(t *testing.T, name string, result modesInvocationResult) {
	t.Helper()
	if strings.Contains(result.stdout, wantPrimaryResult) {
		t.Fatalf("stdout contains false successful primary result: %q", result.stdout)
	}
	errorResponse := decodePresentationErrorResponse(t, name, result.stderr)
	if string(errorResponse.Code) != "INVOCATION_RUNTIME_FAILURE" ||
		errorResponse.Family != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("failure ErrorResponse = %#v, want INVOCATION_RUNTIME_FAILURE internal-server error", errorResponse)
	}
	if strings.HasPrefix(name, "JSON primary") {
		response := decodeInvocationResponse(t, result.stdout)
		assertFailedInvocationResponse(t, response)
		return
	}
	if strings.HasPrefix(name, "JSON") {
		response := decodeTerminalNDJSONInvocationResult(t, result.stdout).Response
		assertFailedInvocationResponse(t, response)
		return
	}
	if name == "primary" || name == "quiet" {
		if strings.TrimSpace(result.stdout) != "" {
			t.Fatalf("%s stdout = %q, want no terminal primary on failure", name, result.stdout)
		}
	}
}

func decodePresentationErrorResponse(t *testing.T, name, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	if name == "default human" || name == "human response stream" {
		lines := nonEmptyLines(stderr)
		if len(lines) == 0 {
			t.Fatalf("human failure stderr is empty")
		}
		stderr = lines[len(lines)-1]
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &response); err != nil {
		t.Fatalf("decode failure ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	return response
}

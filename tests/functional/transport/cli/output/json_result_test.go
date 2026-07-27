package output_test

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	jsonGoalFactoryName          = "@you/goal"
	jsonWantInvocationResultText = "mock worker accepted"
)

// TestCLIJSONSuccessDecodesToPublicInvocationResult proves a successful CLI
// single-JSON run writes exactly one stdout InvocationResponse that decodes
// through the public contract with completed terminal status and no trailing JSON.
func TestCLIJSONSuccessDecodesToPublicInvocationResult(t *testing.T) {
	stdout := runGoalSingleJSON(t)

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var response factoryapi.InvocationResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stdout contains data after InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if got := invocationPrimaryResultText(t, response); got != jsonWantInvocationResultText {
		t.Fatalf("primaryResult = %q, want %q", got, jsonWantInvocationResultText)
	}
}

// TestCLIJSONFailureRemainsValidJSON proves CLI single-JSON failures stay
// machine-parseable: pre-terminal errors emit exactly one stderr ErrorResponse
// with empty stdout, and terminal invocation failures emit one failed
// InvocationResponse on stdout plus one stderr ErrorResponse.
func TestCLIJSONFailureRemainsValidJSON(t *testing.T) {
	t.Run("pre-terminal failure leaves stdout empty with one stderr ErrorResponse", func(t *testing.T) {
		stdout, stderr, err := runSingleJSONInvocation(t, []string{
			"you", "--json", "run", "--named", "@you/missing", "--no-record",
			"deterministic pre-session failure",
		}, "", nil)
		if err == nil {
			t.Fatal("Process.Execute error = nil, want pre-terminal invocation failure")
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty on pre-terminal failure", stdout)
		}
		response := decodeSingleJSONErrorResponse(t, stderr)
		if response.Code != factoryapi.ErrorResponseCode(runcli.InvocationErrorCodeFailed) ||
			response.Family != factoryapi.ErrorFamilyInternalServerError {
			t.Fatalf("ErrorResponse = %#v", response)
		}
	})

	t.Run("terminal failure emits failed InvocationResponse and one stderr ErrorResponse", func(t *testing.T) {
		stdout, stderr, err := runSingleJSONInvocation(t, []string{
			"you", "--json", "run", "--named", jsonGoalFactoryName, "--no-record",
			"deterministic terminal failure",
		}, jsonGoalFactoryName, rejectingGoalMockWorkers())
		if err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		response := decodeSingleJSONInvocationResponse(t, stdout)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusFailed)
		}
		if response.ErrorCode == nil || response.Message == nil {
			t.Fatalf("failed InvocationResponse lacks error detail: %#v", response)
		}
		errorResponse := decodeSingleJSONErrorResponse(t, stderr)
		if errorResponse.Code != factoryapi.ErrorResponseCode(*response.ErrorCode) ||
			errorResponse.Family != factoryapi.ErrorFamilyInternalServerError ||
			!strings.HasPrefix(errorResponse.Message, *response.Message) {
			t.Fatalf("ErrorResponse = %#v, want code %s and message prefix %q", errorResponse, *response.ErrorCode, *response.Message)
		}
	})
}

func runGoalSingleJSON(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, jsonGoalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	args := []string{
		"you", "--json", "run", "--named", jsonGoalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		"deterministic single-json success contract",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
	return inputs.Stdout()
}

func runSingleJSONInvocation(
	t *testing.T,
	args []string,
	packagedFactoryName string,
	mockWorkers *workers.MockWorkersConfig,
) (stdout, stderr string, err error) {
	t.Helper()

	homeDir := t.TempDir()
	if packagedFactoryName != "" {
		support.InstallPackagedFactory(t, homeDir, packagedFactoryName)
	}
	if mockWorkers != nil {
		mockWorkersPath := support.WriteMockWorkersConfig(t, mockWorkers)
		outputFlag := len(args) - 1
		args = append(args[:outputFlag], append([]string{"--with-mock-workers", mockWorkersPath}, args[outputFlag:]...)...)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	err = support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
}

func decodeSingleJSONInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var response factoryapi.InvocationResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stdout contains data after InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	return response
}

func decodeSingleJSONErrorResponse(t *testing.T, stderr string) factoryapi.ErrorResponse {
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

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text content part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text content: %v", err)
	}
	return part.Text
}

package output

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestInvocationFailureOutputContracts(t *testing.T) {
	t.Parallel()

	t.Run("terminal failure emits failed result and standard error", func(t *testing.T) {
		t.Parallel()

		result := executeFailureInvocation(t, []string{
			"you", "--json", "run", "--named", goalFactoryName, "--no-record",
			"--output", "response-stream", "deterministic terminal failure",
		}, goalFactoryName, rejectingGoalWorker())

		if result.err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		lines := nonEmptyLines(result.stdout)
		if len(lines) < 2 {
			t.Fatalf("stdout lines = %#v, want Factory Events and terminal result", lines)
		}
		var terminal struct {
			RecordType string                        `json:"recordType"`
			Response   factoryapi.InvocationResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &terminal); err != nil {
			t.Fatalf("decode terminal stdout record: %v\nstdout:\n%s", err, result.stdout)
		}
		invocationResultCount := 0
		for _, line := range lines {
			var record struct {
				RecordType string `json:"recordType"`
			}
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("decode stdout record: %v\nstdout:\n%s", err, result.stdout)
			}
			if record.RecordType == "invocation_result" {
				invocationResultCount++
			}
		}
		if terminal.RecordType != "invocation_result" || terminal.Response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("terminal record = %#v, want failed invocation_result", terminal)
		}
		if invocationResultCount != 1 {
			t.Fatalf("invocation_result records = %d, want exactly one\nstdout:\n%s", invocationResultCount, result.stdout)
		}
		if terminal.Response.ErrorCode == nil || terminal.Response.Message == nil {
			t.Fatalf("failed InvocationResponse lacks error detail: %#v", terminal.Response)
		}
		for index, line := range lines[:len(lines)-1] {
			var eventRecord struct {
				RecordType string          `json:"recordType"`
				Event      json.RawMessage `json:"event"`
			}
			if err := json.Unmarshal([]byte(line), &eventRecord); err != nil {
				t.Fatalf("decode Factory Event record %d: %v", index, err)
			}
			if eventRecord.RecordType != "factory_event" || len(eventRecord.Event) == 0 {
				t.Fatalf("stdout record %d = %s, want factory_event", index, line)
			}
		}
		response := decodeSingleErrorResponse(t, result.stderr)
		if response.Code != factoryapi.ErrorResponseCode(*terminal.Response.ErrorCode) ||
			response.Family != factoryapi.ErrorFamilyInternalServerError ||
			!strings.HasPrefix(response.Message, *terminal.Response.Message) {
			t.Fatalf("ErrorResponse = %#v", response)
		}
	})

	t.Run("human lifecycle presents canonical failed dispatch", func(t *testing.T) {
		t.Parallel()

		result := executeFailureInvocation(t, []string{
			"you", "run", "--named", goalFactoryName, "--no-record",
			"--output", "response-stream", "deterministic terminal failure",
		}, goalFactoryName, rejectingGoalWorker())

		if result.err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		lines := nonEmptyLines(result.stdout)
		failedIndex := -1
		for index, line := range lines {
			if strings.Contains(line, "workstation failed: execute-goal") {
				failedIndex = index
			}
			if strings.Contains(line, "textDelta") || strings.Contains(line, "toolCall") || strings.Contains(line, "providerSession") {
				t.Fatalf("human lifecycle exposed provider-only output %q\nstdout:\n%s", line, result.stdout)
			}
		}
		if failedIndex < 0 {
			t.Fatalf("stdout lacks canonical failed-dispatch lifecycle\nstdout:\n%s", result.stdout)
		}
		outcomeIndex := strings.Index(result.stdout, "--- invocation outcome ---")
		failureOffset := strings.LastIndex(result.stdout, "workstation failed: execute-goal")
		if outcomeIndex < 0 || failureOffset > outcomeIndex {
			t.Fatalf("canonical failed dispatch does not precede terminal invocation outcome\nstdout:\n%s", result.stdout)
		}
	})
}

func rejectingGoalWorker() *workers.MockWorkersConfig {
	exitCode := 7
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stderr:   "deterministic worker rejection",
				ExitCode: &exitCode,
			},
		}},
	}
}

type failureInvocationResult struct {
	stdout string
	stderr string
	err    error
}

func executeFailureInvocation(
	t *testing.T,
	args []string,
	packagedFactoryName string,
	mockWorkers *workers.MockWorkersConfig,
) failureInvocationResult {
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

	err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	return failureInvocationResult{stdout: inputs.Stdout(), stderr: inputs.Stderr(), err: err}
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

package modes_test

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	wantPrimaryResult        = "deterministic workers primary COMPLETE"
	factoryEventRecordType   = "factory_event"
	invocationResultType     = "invocation_result"
)

// TestCLIRunSuccessPrimaryResultTextJSONAndNDJSON proves a successful public
// you run invocation writes the same deterministic primary result across quiet
// text, single-JSON, and NDJSON response-stream presentations at the CLI boundary.
func TestCLIRunSuccessPrimaryResultTextJSONAndNDJSON(t *testing.T) {
	t.Run("quiet text primary result", func(t *testing.T) {
		result := executeSuccessfulRun(t, nil, []string{"--quiet"})
		if strings.TrimSpace(result.stdout) != wantPrimaryResult {
			t.Fatalf("stdout = %q, want raw primary result %q", result.stdout, wantPrimaryResult)
		}
		assertSuccessfulRunStderrEmpty(t, result.stderr)
	})

	t.Run("single JSON InvocationResponse", func(t *testing.T) {
		result := executeSuccessfulRun(t, []string{"--json"}, nil)
		response := decodeSingleInvocationResponse(t, result.stdout)
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

type successfulRunResult struct {
	stdout string
	stderr string
}

func executeSuccessfulRun(t *testing.T, globalArgs, runArgs []string) successfulRunResult {
	t.Helper()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, support.NewStaticSuccessCommandRunner(wantPrimaryResult), nil)

	args := []string{"you"}
	args = append(args, globalArgs...)
	args = append(args,
		"run",
		"--factory", factoryPath,
		"--no-record",
	)
	args = append(args, runArgs...)
	args = append(args, "prove workers-owned output modes")

	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	if err := support.BuildProcess(t, edges).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	return successfulRunResult{stdout: inputs.Stdout(), stderr: inputs.Stderr()}
}

func scaffoldProviderBackedFactory(t *testing.T) string {
	t.Helper()

	cfg := map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func assertSuccessfulRunStderrEmpty(t *testing.T, stderr string) {
	t.Helper()
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", stderr)
	}
}

func decodeSingleInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
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
		if index != len(lines)-1 {
			t.Fatalf("invocation_result record index = %d, want terminal index %d", index, len(lines)-1)
		}
		if err := json.Unmarshal(fields["response"], &terminal.Response); err != nil {
			t.Fatalf("decode terminal invocation result %d: %v\nline: %s", index, err, line)
		}
		terminal.RecordType = recordType
	}
	if terminal.RecordType == "" {
		t.Fatalf("stdout missing terminal invocation_result\nstdout:\n%s", stdout)
	}
	return terminal
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

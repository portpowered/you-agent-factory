// Package loading holds customer functional scenarios for inline JavaScript
// Factory loading through the public CLI and customer process boundary.
package loading_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const inlineJavaScriptSuccessResult = "<SUCCESS>"

var privateJavaScriptVMDiagnosticMarkers = []string{
	"goja",
	"Runtime",
	"stack frame",
	"heap dump",
}

// TestInlineJavaScriptFactoryRunsFromCLI proves an inline JavaScript Factory
// definition completes through the public you run customer process boundary with
// a terminal COMPLETED primary outcome and without private VM
// internals in success diagnostics.
func TestInlineJavaScriptFactoryRunsFromCLI(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "inline-javascript-loading",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("` + inlineJavaScriptSuccessResult + `");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	mockWorkersPath := writeEmptyMockWorkersConfig(t, dir)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"hello",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	if err := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful JSON invocation", inputs.Stderr())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for inline factory without child dispatch", runner.CallCount())
	}

	result := decodeSingleInvocationResponse(t, inputs.Stdout())
	assertInlineJavaScriptSuccessOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

func writeEmptyMockWorkersConfig(t *testing.T, dir string) string {
	t.Helper()

	path := filepath.Join(dir, "mock-workers.json")
	if err := os.WriteFile(path, []byte(`{"mockWorkers":[]}`), 0o600); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func decodeSingleInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result factoryapi.InvocationResponse
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode CLI stdout as InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contained more than one terminal JSON result: %v\nstdout:\n%s", err, stdout)
	}
	return result
}

func assertInlineJavaScriptSuccessOutcome(t *testing.T, result factoryapi.InvocationResponse) {
	t.Helper()

	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("terminal outcome = %s, want COMPLETED", result.Status)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != inlineJavaScriptSuccessResult {
		t.Fatalf("primary result = %#v, want exact string %q", part.Json, inlineJavaScriptSuccessResult)
	}
}

func assertNoPrivateJavaScriptVMDiagnostics(t *testing.T, outputs ...string) {
	t.Helper()

	combined := strings.ToLower(strings.Join(outputs, "\n"))
	for _, marker := range privateJavaScriptVMDiagnosticMarkers {
		if strings.Contains(combined, strings.ToLower(marker)) {
			t.Fatalf("success diagnostics exposed private VM detail %q in %q", marker, strings.Join(outputs, "\n---\n"))
		}
	}
}

// typescript_test.go holds customer functional scenarios for TypeScript
// Factory loading through the public CLI and customer process boundary.
package loading_test

import (
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const typeScriptSuccessResult = "<TYPESCRIPT_SUCCESS>"

// TestTypeScriptFactoryTranspilesAndRuns proves a supported file-backed
// TypeScript Factory transpiles and completes through the public you run
// customer process boundary with a terminal COMPLETED primary outcome that
// reflects typed TypeScript source execution and without private VM internals
// in success diagnostics.
func TestTypeScriptFactoryTranspilesAndRuns(t *testing.T) {
	t.Parallel()

	dir := scaffoldFileBackedTypeScriptFactory(t)
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
		t.Fatalf("provider command runner call count = %d, want 0 for file-backed TypeScript factory without child dispatch", runner.CallCount())
	}

	result := decodeSingleInvocationResponse(t, inputs.Stdout())
	assertTypeScriptSuccessOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

func scaffoldFileBackedTypeScriptFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "typescript-loading",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.ts",
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, "workflow.ts"),
		[]byte(`type SuccessMarker = string;
const result: SuccessMarker = "`+typeScriptSuccessResult+`";
workflow.final(result);`),
		0o600,
	); err != nil {
		t.Fatalf("write TypeScript workflow entry: %v", err)
	}
	return dir
}

func assertTypeScriptSuccessOutcome(t *testing.T, result factoryapi.InvocationResponse) {
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
	if got, ok := part.Json.(string); !ok || got != typeScriptSuccessResult {
		t.Fatalf("primary result = %#v, want exact TypeScript success string %q", part.Json, typeScriptSuccessResult)
	}
}

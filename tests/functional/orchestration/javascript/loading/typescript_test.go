// typescript_test.go holds customer functional scenarios for TypeScript
// Factory loading through the public CLI and customer process boundary.
package loading_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	typeScriptSuccessResult              = "<TYPESCRIPT_SUCCESS>"
	typeScriptSyntaxErrorSource          = "type FailureMarker = string;\nworkflow.final(\"ok\");\nphase(\"setup\";\n"
	typeScriptSyntaxErrorLine            = 3
	typeScriptSourceMapSyntaxErrorSource = "interface Ignored {\n  prompt: string;\n}\nworkflow.final(\"ok\");\nphase(\"setup\";\n"
	typeScriptSourceMapAuthoredLine      = 5
	typeScriptSourceMapEmittedLine       = 2
)

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

// TestTypeScriptTypeOrSyntaxFailureReturnsCustomerDiagnostic proves a
// file-backed TypeScript Factory with a deliberate syntax error fails before
// work starts through the public you run customer process boundary with an
// actionable load/validation diagnostic and without private VM internals or
// external worker dispatch.
func TestTypeScriptTypeOrSyntaxFailureReturnsCustomerDiagnostic(t *testing.T) {
	t.Parallel()

	dir := scaffoldFileBackedTypeScriptFactoryWithSyntaxError(t)
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
	err := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}).Execute(inputs.Input)
	assertTypeScriptTypeOrSyntaxFailureOutcome(
		t,
		err,
		inputs.Stdout(),
		inputs.Stderr(),
		typeScriptSyntaxErrorLine,
	)
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for TypeScript syntax failure before dispatch", runner.CallCount())
	}
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

// TestTypeScriptSourceMapReportsAuthoredLocation proves a file-backed
// TypeScript Factory failure diagnostic reports the customer-authored .ts
// source line via source-map remapping rather than only the emitted JavaScript
// line after TypeScript stripping.
func TestTypeScriptSourceMapReportsAuthoredLocation(t *testing.T) {
	t.Parallel()

	dir := scaffoldFileBackedTypeScriptFactoryWithSourceMapSyntaxError(t)
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
	err := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}).Execute(inputs.Input)
	assertTypeScriptSourceMapFailureOutcome(
		t,
		err,
		inputs.Stdout(),
		inputs.Stderr(),
		typeScriptSourceMapAuthoredLine,
		typeScriptSourceMapEmittedLine,
	)
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for TypeScript source-map failure before dispatch", runner.CallCount())
	}
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

func scaffoldFileBackedTypeScriptFactoryWithSyntaxError(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "typescript-syntax-error",
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
		[]byte(typeScriptSyntaxErrorSource),
		0o600,
	); err != nil {
		t.Fatalf("write TypeScript workflow entry: %v", err)
	}
	return dir
}

func scaffoldFileBackedTypeScriptFactoryWithSourceMapSyntaxError(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "typescript-source-map-syntax-error",
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
		[]byte(typeScriptSourceMapSyntaxErrorSource),
		0o600,
	); err != nil {
		t.Fatalf("write TypeScript workflow entry: %v", err)
	}
	return dir
}

func assertTypeScriptTypeOrSyntaxFailureOutcome(
	t *testing.T,
	runErr error,
	stdout string,
	stderr string,
	wantLine int,
) {
	t.Helper()

	if runErr == nil {
		t.Fatalf("Process.Execute() error = nil, want TypeScript syntax failure before invocation\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout before terminal invocation on TypeScript syntax failure", stdout)
	}

	failureText := strings.Join([]string{runErr.Error(), stderr}, "\n")
	if !strings.Contains(failureText, workflowSourceSyntaxErrorCode) {
		t.Fatalf("failure output = %q, want customer-stable code %q", failureText, workflowSourceSyntaxErrorCode)
	}
	if !strings.Contains(failureText, "syntax error") {
		t.Fatalf("failure output = %q, want actionable syntax failure diagnostic", failureText)
	}
	if !strings.Contains(failureText, fmt.Sprintf("line %d", wantLine)) {
		t.Fatalf("failure output = %q, want authored source line %d indicator", failureText, wantLine)
	}
}

func assertTypeScriptSourceMapFailureOutcome(
	t *testing.T,
	runErr error,
	stdout string,
	stderr string,
	wantAuthoredLine int,
	wantEmittedLine int,
) {
	t.Helper()

	assertTypeScriptTypeOrSyntaxFailureOutcome(t, runErr, stdout, stderr, wantAuthoredLine)
	if wantAuthoredLine == wantEmittedLine {
		t.Fatalf("source-map scenario must use different authored and emitted line numbers; got %d", wantAuthoredLine)
	}

	failureText := strings.Join([]string{runErr.Error(), stderr}, "\n")
	remappedSuffix := fmt.Sprintf("(line %d, column", wantAuthoredLine)
	if !strings.Contains(failureText, remappedSuffix) {
		t.Fatalf(
			"failure output = %q, want remapped authored TypeScript location suffix %q",
			failureText,
			remappedSuffix,
		)
	}
}

func assertTypeScriptSuccessOutcome(t *testing.T, result factoryapi.InvocationResponse) {
	t.Helper()

	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("terminal outcome = %s, want COMPLETED", result.Status)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if part.Text != typeScriptSuccessResult {
		t.Fatalf("primary result = %q, want exact TypeScript success string %q", part.Text, typeScriptSuccessResult)
	}
}

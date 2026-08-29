// file_javascript_test.go holds customer functional scenarios for file-backed
// JavaScript Factory loading through the public CLI and customer process boundary.
package loading_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	fileJavaScriptImportedSuccessResult = "<FILE_IMPORT_SUCCESS>"
	fileJavaScriptMissingImportPath     = "./lib/missing.js"
	workflowSourceNotFoundCode          = "workflow.source.notFound"
)

// TestJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot proves a
// file-backed JavaScript Factory resolves a factory-relative import through
// the public you run customer process boundary with a terminal COMPLETED
// primary outcome that reflects the imported module contribution.
func runJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot(t *testing.T, fixture *loadingFixture) {
	dir := scaffoldFileBackedJavaScriptFactoryWithRelativeImport(t)
	mockWorkersPath := writeEmptyMockWorkersConfig(t, dir)
	result, inputs := fixture.runCLIInvocation(t, []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"hello",
	}, dir, t.TempDir())
	if got := fixture.provider.CallCount(); got != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for file-backed factory without child dispatch", got)
	}
	assertFileJavaScriptImportedSuccessOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

// TestJavaScriptFactoryMissingImportFailsActionably proves a file-backed
// JavaScript Factory with a missing factory-relative import fails before work
// starts through the public you run customer process boundary with an
// actionable load diagnostic and without private VM internals or external
// worker dispatch.
func runJavaScriptFactoryMissingImportFailsActionably(t *testing.T, fixture *loadingFixture) {
	dir := scaffoldFileBackedJavaScriptFactoryWithMissingImport(t)
	mockWorkersPath := writeEmptyMockWorkersConfig(t, dir)
	inputs, err := fixture.executeCLI(t, []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"hello",
	}, dir, t.TempDir())
	assertFileJavaScriptMissingImportFailureOutcome(
		t,
		err,
		inputs.Stdout(),
		inputs.Stderr(),
		fileJavaScriptMissingImportPath,
	)
	if got := fixture.provider.CallCount(); got != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for missing import before dispatch", got)
	}
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
	fixture.recoverAfterLoadFailure(t, "missing-import")
}

func scaffoldFileBackedJavaScriptFactoryWithRelativeImport(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "file-javascript-relative-import",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.js",
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	libDir := filepath.Join(dir, "lib")
	if err := os.MkdirAll(libDir, 0o700); err != nil {
		t.Fatalf("create lib directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(libDir, "constants.js"),
		[]byte(`export const successResult = "`+fileJavaScriptImportedSuccessResult+`";`),
		0o600,
	); err != nil {
		t.Fatalf("write imported module: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "workflow.js"),
		[]byte(`import { successResult } from "./lib/constants.js";
workflow.final(successResult);`),
		0o600,
	); err != nil {
		t.Fatalf("write workflow entry: %v", err)
	}
	return dir
}

func scaffoldFileBackedJavaScriptFactoryWithMissingImport(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "file-javascript-missing-import",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.js",
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, "workflow.js"),
		[]byte(fmt.Sprintf(`import { missing } from %q;
workflow.final(missing);`, fileJavaScriptMissingImportPath)),
		0o600,
	); err != nil {
		t.Fatalf("write workflow entry: %v", err)
	}
	return dir
}

func assertFileJavaScriptMissingImportFailureOutcome(
	t *testing.T,
	runErr error,
	stdout string,
	stderr string,
	wantImportPath string,
) {
	t.Helper()

	if runErr == nil {
		t.Fatalf("Process.Execute() error = nil, want missing import failure before invocation\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout before terminal invocation on missing import failure", stdout)
	}

	failureText := strings.Join([]string{runErr.Error(), stderr}, "\n")
	if !strings.Contains(failureText, workflowSourceNotFoundCode) {
		t.Fatalf("failure output = %q, want customer-stable code %q", failureText, workflowSourceNotFoundCode)
	}
	if !strings.Contains(failureText, wantImportPath) {
		t.Fatalf("failure output = %q, want missing import path %q", failureText, wantImportPath)
	}
}

func assertFileJavaScriptImportedSuccessOutcome(t *testing.T, result factoryapi.InvocationResponse) {
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
	if part.Text != fileJavaScriptImportedSuccessResult {
		t.Fatalf("primary result = %q, want exact imported string %q", part.Text, fileJavaScriptImportedSuccessResult)
	}
}

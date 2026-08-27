package workers_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const stableDisallowedPermissionDiagnostic = `policy denied: Factory "named-factory" child "skip-child" requested permission "SKIP_PERMISSIONS" not listed in allowedPermissions`

const disallowedPermissionWorkflow = `return (async function () {
  return await agent.run({
    prompt: "permission denial child",
    label: "skip-child",
    modelProvider: "codex",
    permissions: "SKIP_PERMISSIONS"
  });
})();`

// TestJavaScriptAgentRunCodexCommandCharacterization characterizes the public Codex command emitted by JavaScript agent execution.
func TestJavaScriptAgentRunCodexCommandCharacterization(t *testing.T) {
	tests := []struct {
		name        string
		permissions string
		wantArgs    []string
	}{
		{
			name:        "permissions-omitted",
			permissions: "omitted",
			wantArgs:    []string{"exec", "--json", "-"},
		},
		{
			name:        "permissions-skip",
			permissions: "SKIP_PERMISSIONS",
			wantArgs:    []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			dir := support.ScaffoldFactory(t, permissionMatrixFactoryConfig(permissionMatrixWorkflow(test.permissions)))
			runner := support.NewRecordingCommandRunner("permission matrix child output")
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "--json", "run",
				"--factory", filepath.Join(dir, "factory.json"),
				"--output", "primary",
				"--no-record",
				"permission matrix prompt",
			})
			inputs.Input.WorkingDirectory = dir
			homeDir := t.TempDir()
			inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)

			if err := support.BuildProcess(t, serviceedges.Edges{
				ProviderCommandRunner: runner,
			}).Execute(inputs.Input); err != nil {
				t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
			}

			requests := runner.Requests()
			if len(requests) != 1 {
				t.Fatalf("provider command requests = %d, want one request; requests=%#v", len(requests), requests)
			}
			got := requests[0]
			if got.Command != "codex" || !reflect.DeepEqual(got.Args, test.wantArgs) {
				t.Fatalf("provider command = %q %#v, want codex %#v", got.Command, got.Args, test.wantArgs)
			}
		})
	}
}

// TestJavaScriptAgentRunDisallowedPermissionFailsThroughPublicCLI proves disallowed agent permissions fail through the public CLI.
func TestJavaScriptAgentRunDisallowedPermissionFailsThroughPublicCLI(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, disallowedPermissionFactoryConfig())
	workflowDir := filepath.Join(dir, "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflow directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(disallowedPermissionWorkflow), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	runner := support.NewRecordingCommandRunner("unexpected provider execution")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--output", "primary",
		"--no-record",
		"permission denial prompt",
	})
	inputs.Input.WorkingDirectory = dir
	homeDir := t.TempDir()
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)

	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	err = process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf("Process.Execute() error = nil; stdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
	}
	output := strings.Join([]string{inputs.Stdout(), inputs.Stderr(), err.Error()}, "\n")
	if !strings.Contains(output, stableDisallowedPermissionDiagnostic) {
		t.Fatalf("public denial diagnostic = %q, want %q", output, stableDisallowedPermissionDiagnostic)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 before denied child dispatch", runner.CallCount())
	}
}

func permissionMatrixFactoryConfig(source string) map[string]any {
	config := map[string]any{}
	config["name"] = "javascript-permission-matrix"
	config["invocationSignature"] = map[string]any{
		"parameters": []any{map[string]any{
			"name": "prompt", "required": false,
			"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
		}},
	}
	config["orchestrator"] = map[string]any{
		"kind": "JAVASCRIPT",
		"javascript": map[string]any{
			"inlineSource": map[string]any{
				"encoding": "utf-8",
				"inline":   source,
			},
			"argsSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
				"additionalProperties": false,
			},
		},
	}
	return config
}

func disallowedPermissionFactoryConfig() map[string]any {
	config := permissionMatrixFactoryConfig(disallowedPermissionWorkflow)
	config["name"] = "named-factory"
	orchestrator := config["orchestrator"].(map[string]any)
	javascript := orchestrator["javascript"].(map[string]any)
	javascript["sourceRef"] = "workflows/review.js"
	delete(javascript, "inlineSource")
	javascript["defaultPolicy"] = map[string]any{
		"allowedPermissions": []any{"DEFAULT"},
	}
	return config
}

func permissionMatrixWorkflow(permissions string) string {
	return permissionMatrixWorkflowWithPrompt(permissions, "capture the current Codex command")
}

func permissionMatrixWorkflowWithPrompt(permissions, prompt string) string {
	field := ""
	if permissions != "omitted" {
		field = `, permissions: "` + permissions + `"`
	}
	return `return (async function () {
  return await agent.run({
    prompt: "` + prompt + `",
    label: "permission-matrix-child",
    modelProvider: "codex"` + field + `
  });
})();`
}

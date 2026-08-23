package workers_test

import (
	"path/filepath"
	"reflect"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestJavaScriptAgentRunCodexCommandCharacterization(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		skipPermissions string
		wantArgs        []string
	}{
		{
			name:            "mode-unset/skipPermissions-absent",
			skipPermissions: "absent",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "mode-unset/skipPermissions-false",
			skipPermissions: "false",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "mode-unset/skipPermissions-true",
			skipPermissions: "true",
			wantArgs:        []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-"},
		},
		{
			name:            "READ_ONLY/skipPermissions-absent",
			mode:            "READ_ONLY",
			skipPermissions: "absent",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "READ_ONLY/skipPermissions-false",
			mode:            "READ_ONLY",
			skipPermissions: "false",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "READ_ONLY/skipPermissions-true",
			mode:            "READ_ONLY",
			skipPermissions: "true",
			wantArgs:        []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			dir := support.ScaffoldFactory(t, permissionMatrixFactoryConfig(test.mode, permissionMatrixWorkflow(test.skipPermissions)))
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

func permissionMatrixFactoryConfig(mode, source string) map[string]any {
	defaultPolicy := map[string]any{}
	if mode != "" {
		defaultPolicy["mode"] = mode
	}
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
			"defaultPolicy": defaultPolicy,
		},
	}
	return config
}

func permissionMatrixWorkflow(skipPermissions string) string {
	field := ""
	if skipPermissions != "absent" {
		field = ", skipPermissions: " + skipPermissions
	}
	return `return (async function () {
  return await agent.run({
    prompt: "capture the current Codex command",
    label: "permission-matrix-child",
    modelProvider: "codex"` + field + `
  });
})();`
}

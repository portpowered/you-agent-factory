package shell_completion_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	shellFactoryName = "shell-fixture"
	shellModeValue   = "json"
	shellFileName    = "shell-config.json"
)

// TestGeneratedCompletionScriptsReachRootProcess proves generated completion
// and dynamic candidates through the production root process. Shell syntax is
// covered by transport-level unit and integration tests; this functional test
// does not build or invoke a CLI executable.
func TestGeneratedCompletionScriptsReachRootProcess(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "powershell"} {
		shell := shell
		t.Run(shell, func(t *testing.T) {
			result := executeCompletionCommand(
				t,
				completionProcess(t).invocation(t, false),
				"completion", shell,
			)
			requireCompletionSuccess(t, result, "generate "+shell)
			if len(strings.TrimSpace(result.stdout)) < 100 {
				t.Fatalf("generated %s completion is unexpectedly empty", shell)
			}
		})
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "factory", args: []string{"__complete", "run", "--named", "shell-fi"}, want: shellFactoryName},
		{name: "mode", args: []string{"__complete", "run", "--named", shellFactoryName, "--mode", "j"}, want: shellModeValue},
		{name: "file", args: []string{"__complete", "run", "--named", shellFactoryName, "--config", "shell-conf"}, want: "ShellCompDirectiveDefault"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := executeCompletionCommand(
				t,
				completionProcess(t).invocation(t, true),
				tc.args...,
			)
			requireCompletionSuccess(t, result, "request "+tc.name+" completion")
			if !strings.Contains(result.stdout+result.stderr, tc.want) {
				t.Fatalf("%s completion lacks %q:\nstdout:\n%s\nstderr:\n%s", tc.name, tc.want, result.stdout, result.stderr)
			}
		})
	}
}

func requireCompletionSuccess(t testing.TB, result completionCommandResult, operation string) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("%s: %v\nstdout:\n%s\nstderr:\n%s", operation, result.err, result.stdout, result.stderr)
	}
}

func requireCompletionHelp(t testing.TB, result completionCommandResult, operation string) {
	t.Helper()
	// Process.Execute returns nil for the successful exit status observed at
	// this in-process boundary.
	requireCompletionSuccess(t, result, operation+" help fallback")
	if !strings.Contains(result.stdout, "Usage:\n  you completion [command]") {
		t.Fatalf("%s help output = %q, want completion usage", operation, result.stdout)
	}
	if !strings.Contains(result.stdout, "Available Commands:") ||
		!strings.Contains(result.stdout, "Generate the autocompletion script") {
		t.Fatalf("%s help output = %q, want completion command inventory", operation, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("%s help stderr = %q, want empty", operation, result.stderr)
	}
}

func writeShellCompletionFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	definition := map[string]any{
		"id": shellFactoryName, "name": shellFactoryName,
		"workTypes": []any{}, "resources": []any{}, "workers": []any{}, "workstations": []any{},
		"invocationSignature": map[string]any{"parameters": []map[string]any{
			{"name": "mode", "externalName": "mode", "description": "output mode", "choices": []string{shellModeValue, "text"}, "bindings": []map[string]any{{"kind": "NAMED"}}},
			{"name": "config", "externalName": "config", "description": "configuration file", "typeHint": "FILE_PATH", "bindings": []map[string]any{{"kind": "NAMED"}}},
		}},
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("marshal shell completion Factory: %v", err)
	}
	factoryDirectory := filepath.Join(workingDirectory, "factory", shellFactoryName)
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create shell completion Factory directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), payload, 0o600); err != nil {
		t.Fatalf("write shell completion Factory: %v", err)
	}
}

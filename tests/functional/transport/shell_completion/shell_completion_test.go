package shell_completion_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	shellFactoryName = "shell-fixture"
	shellModeValue   = "json"
	shellFileName    = "shell-config.json"
)

// TestGeneratedCompletionScriptsReachRootProcess proves generated completion
// and dynamic candidates through the production root process. Shell syntax is
// covered by transport-level unit tests; functional tests do not build a CLI.
func TestGeneratedCompletionScriptsReachRootProcess(t *testing.T) {
	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	writeShellCompletionFactory(t, workingDirectory)
	if err := os.WriteFile(filepath.Join(workingDirectory, shellFileName), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write shell completion file: %v", err)
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	environment := append(os.Environ(), "HOME="+homeDirectory, "USERPROFILE="+homeDirectory)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	for _, shell := range []string{"bash", "zsh", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			command := harness.CommandContext(ctx, "completion", shell)
			command.Dir = workingDirectory
			command.Env = environment
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("generate %s completion: %v\n%s", shell, err, output)
			}
			if len(strings.TrimSpace(string(output))) < 100 {
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
		t.Run(tc.name, func(t *testing.T) {
			command := harness.CommandContext(ctx, tc.args...)
			command.Dir = workingDirectory
			command.Env = environment
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("request %s completion: %v\n%s", tc.name, err, output)
			}
			if !strings.Contains(string(output), tc.want) {
				t.Fatalf("%s completion lacks %q:\n%s", tc.name, tc.want, output)
			}
		})
	}
}

func writeShellCompletionFactory(t *testing.T, workingDirectory string) {
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

package goal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const customizedPackagedGoalFactoryName = "@test/goal"

// TestPackagedGoalInterpolationFailureStopsBeforeProviderExecution proves an
// omitted definition variable fails in Factory Definitions before provider
// selection for both named and explicit-file packaged goal invocation.
// Isolation is intentional: each selection gets its own customer home,
// working directory, materialized definition, and provider selector so named
// and file resolution cannot share durable state. Dependency fidelity is
// local-real root.BuildProcess/Process.Execute and Factory Definitions, with
// only ProviderCommandRunner controlled at the external provider edge.
func TestPackagedGoalInterpolationFailureStopsBeforeProviderExecution(t *testing.T) {
	provider := testutil.NewProviderCommandRunner()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: provider,
	})
	support.CleanupProcess(t, process)

	for _, selection := range []string{"named", "file"} {
		selection := selection
		t.Run(selection, func(t *testing.T) {
			homeDir := t.TempDir()
			workingDirectory := t.TempDir()
			environment := packagedGoalEnvironment(homeDir)
			factoryDir := support.InstallPackagedFactoryWithProcess(
				t, process, environment, workingDirectory, packagedGoalFactoryName,
			)
			factoryPath := filepath.Join(factoryDir, "factory.json")
			configureMissingInterpolationFactory(t, factoryPath)
			if selection == "named" {
				factoryDir = support.CopyFactoryAsNamed(t, factoryDir, homeDir, customizedPackagedGoalFactoryName)
				factoryPath = filepath.Join(factoryDir, "factory.json")
			}

			model := nextPackagedGoalSelector("interpolation-" + selection)
			var stdout, stderr bytes.Buffer
			stdinIsTTY := true
			stdoutIsTTY := false
			err := process.Execute(root.Input{
				Args: packagedGoalInterpolationFailureArguments(
					selection, factoryPath, customizedPackagedGoalFactoryName, model,
				),
				Env:              environment,
				Stdin:            strings.NewReader(""),
				Stdout:           &stdout,
				Stderr:           &stderr,
				Context:          t.Context(),
				WorkingDirectory: workingDirectory,
				StdinIsTTY:       &stdinIsTTY,
				StdoutIsTTY:      &stdoutIsTTY,
			})
			if err == nil {
				t.Fatalf("%s interpolation failure returned nil; stdout=%q stderr=%q", selection, stdout.String(), stderr.String())
			}
			if !strings.Contains(err.Error(), "resolve invocation-effective execution definition") {
				t.Fatalf("%s interpolation error = %v, want Factory Definitions resolution context", selection, err)
			}
			if !strings.Contains(err.Error(), `references omitted invocation parameter "missing"`) {
				t.Fatalf("%s interpolation error = %v, want omitted parameter context", selection, err)
			}
		})
	}
	if calls := provider.CallCount(); calls != 0 {
		t.Fatalf("provider command calls = %d, want zero after interpolation failure", calls)
	}
}

func configureMissingInterpolationFactory(t *testing.T, factoryPath string) {
	t.Helper()
	payload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read installed Factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode installed Factory: %v", err)
	}
	factory["invocationSignature"] = map[string]any{
		"parameters": []any{map[string]any{
			"name":     "input",
			"required": true,
			"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
		}},
	}
	workersConfig, ok := factory["workers"].([]any)
	if !ok || len(workersConfig) == 0 {
		t.Fatalf("installed Factory workers = %#v", factory["workers"])
	}
	workerConfig, ok := workersConfig[0].(map[string]any)
	if !ok {
		t.Fatalf("installed Factory worker = %#v", workersConfig[0])
	}
	workerConfig["body"] = "input=${input}|missing=${missing}"
	updated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode interpolation Factory fixture: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o600); err != nil {
		t.Fatalf("write interpolation Factory fixture: %v", err)
	}
	support.ReplaceGoalWorkerInstructions(t, factoryPath, "input=${input}|missing=${missing}")
}

func packagedGoalInterpolationFailureArguments(selection, factoryPath, factoryName, model string) []string {
	if selection == "named" {
		return []string{
			"you", "run", "--named", factoryName,
			"--no-record", "--provider", "CODEX", "--model", model,
			"provided interpolation input",
		}
	}
	return []string{
		"you", "run", "--factory", factoryPath,
		"--no-record", "--provider", "CODEX", "--model", model,
		"provided interpolation input",
	}
}

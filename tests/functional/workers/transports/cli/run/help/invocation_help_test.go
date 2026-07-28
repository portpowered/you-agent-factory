package help_test

import (
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	invocationHelpNamedFactoryName    = "invocation-help-alpha"
	invocationHelpFactoryConfigName   = "invocation-help-portable"
	invocationHelpWorkTypeName        = "help-task"
	invocationHelpRequiredParameter   = "input"
	invocationHelpOptionalParameter   = "mode"
	invocationHelpOptionalPathParameter = "artifact"
)

// TestCLIRunHelpShowsInvocationSignatureForNamedFactory proves you run --named
// <factory> --help prints Factory invocation help for the selected named Factory,
// including signature usage and the factory-defined argument surface customers
// use to compose a run command without starting Work.
func TestCLIRunHelpShowsInvocationSignatureForNamedFactory(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	sourceDir := support.ScaffoldFactory(t, invocationHelpFactoryConfig())
	support.CreateNamedFactory(
		t,
		homeDir,
		workingDirectory,
		invocationHelpNamedFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help",
	})
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(run --named %s --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			invocationHelpNamedFactoryName,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	got := inputs.Stdout()
	for _, want := range []string{
		"Factory invocation help",
		"Selected factory: " + invocationHelpFactoryConfigName + " (named factory " + invocationHelpNamedFactoryName + ")",
		"Usage:\n  you run --named " + invocationHelpNamedFactoryName + " <" + invocationHelpRequiredParameter + "> [--" + invocationHelpOptionalPathParameter + " <file-path>] [--" + invocationHelpOptionalParameter + " <value>]",
		"Factory-defined arguments:",
		invocationHelpRequiredParameter,
		invocationHelpOptionalParameter,
		invocationHelpOptionalPathParameter,
		"you run --named " + invocationHelpNamedFactoryName + " 'Fix the lint issues' --" + invocationHelpOptionalPathParameter + " report.md --" + invocationHelpOptionalParameter + " safe",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run --named %s --help missing %q:\n%s", invocationHelpNamedFactoryName, want, got)
		}
	}
	if strings.Contains(got, "Load workflow and run the factory engine") {
		t.Fatalf("expected signature-aware help instead of generic Cobra help:\n%s", got)
	}
}

func invocationHelpFactoryConfig() map[string]any {
	return map[string]any{
		"name": invocationHelpFactoryConfigName,
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":        invocationHelpRequiredParameter,
					"description": "Primary text input for the portable factory.",
					"required":    true,
					"bindings": []any{
						map[string]any{"kind": "POSITIONAL", "position": 1},
						map[string]any{"kind": "STDIN"},
					},
				},
				map[string]any{
					"name":         invocationHelpOptionalParameter,
					"description":  "Execution mode for the portable factory.",
					"choices":        []any{"fast", "safe"},
					"defaultValue": "safe",
					"bindings":     []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":        invocationHelpOptionalPathParameter,
					"description": "Optional output file path.",
					"aliases":     []any{"out"},
					"typeHint":    "FILE_PATH",
					"bindings":    []any{map[string]any{"kind": "NAMED"}},
				},
			},
			"outputContract": map[string]any{
				"mode":          "FILE",
				"pathParameter": invocationHelpOptionalPathParameter,
				"contentType":   "text/plain",
				"fileExtension": ".txt",
			},
			"examples": []any{
				map[string]any{
					"name": "positional-input",
					"argv": []any{
						"Fix the lint issues",
						"--" + invocationHelpOptionalParameter,
						"safe",
						"--" + invocationHelpOptionalPathParameter,
						"report.md",
					},
				},
			},
		},
		"workTypes": []any{
			map[string]any{
				"name":             invocationHelpWorkTypeName,
				"handlingBehavior": []any{"DEFAULT"},
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "complete", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []any{
			map[string]any{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-prompt",
				"worker":    "mock-worker",
				"inputs":    []any{map[string]any{"workType": invocationHelpWorkTypeName, "state": "init"}},
				"outputs":   []any{map[string]any{"workType": invocationHelpWorkTypeName, "state": "complete"}},
				"onFailure": []any{map[string]any{"workType": invocationHelpWorkTypeName, "state": "failed"}},
			},
		},
	}
}

package parameters_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRunAcceptsOnePositionalPrompt proves a single positional prompt that
// contains spaces and Unicode characters survives through the public CLI
// observation edge with the exact customer-supplied string intact.
func TestRunAcceptsOnePositionalPrompt(t *testing.T) {
	prompt := "Ship the café résumé plan"

	factoryDir := support.ScaffoldSingleStepFactory(t, "positional-values")
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	var observation cliobservation.Result
	process := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		prompt,
	})
	inputs.WorkingDirectory = t.TempDir()

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(run positional prompt) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if observation.Parse.CommandPath != "you run" {
		t.Fatalf("observed command path = %q, want you run", observation.Parse.CommandPath)
	}
	if len(observation.Parse.Positionals) != 1 {
		t.Fatalf("observed positional count = %d, want 1: %#v", len(observation.Parse.Positionals), observation.Parse.Positionals)
	}
	if got := observation.Parse.Positionals[0]; got != prompt {
		t.Fatalf("observed positional prompt = %q, want %q", got, prompt)
	}
}

// TestRunRejectsExtraPositionalValues proves surplus positional prompt values
// on you run --factory are rejected with a stable diagnostic before any worker
// provider dispatch can start.
func TestRunRejectsExtraPositionalValues(t *testing.T) {
	factoryDir := scaffoldSinglePositionalInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "processor",
			WorkstationName: "process",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})

	providerRunner := testutil.NewProviderCommandRunner()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"--with-mock-workers", mockWorkersPath,
		"first prompt",
		"second prompt",
	})
	inputs.WorkingDirectory = t.TempDir()

	executeErr := process.Execute(inputs.Input)
	if executeErr == nil {
		t.Fatalf(
			"Process.Execute(extra positional prompts) succeeded; stdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	diagnostic := executeErr.Error() + "\n" + inputs.Stderr()
	for _, want := range []string{
		"INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW",
		"received 2 positional arguments but the active invocationSignature only accepts 1",
	} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf(
				"extra positional diagnostic missing %q:\n%s",
				want,
				diagnostic,
			)
		}
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider dispatch calls = %d, want 0", providerRunner.CallCount())
	}
}

func scaffoldSinglePositionalInvocationFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "positional-values",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":     "input",
					"required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				},
			},
		},
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []any{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
}

package parameters_test

import (
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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

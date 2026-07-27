package parameters_test

import (
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIStringBooleanAndRepeatedFlagsReachRequest proves string, boolean, and
// repeated CLI flags reach the external observation edge with the
// customer-supplied values without starting product handlers.
func TestCLIStringBooleanAndRepeatedFlagsReachRequest(t *testing.T) {
	var observation cliobservation.Result
	process := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--server", "https://factory.example",
		"-v",
		"session", "dispatches", "session-customer",
		"--phase", "queued",
		"--phase", "active",
		"--json",
	})
	inputs.WorkingDirectory = t.TempDir()

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(session dispatches observation) error = %v", err)
	}
	if observation.Parse.CommandPath != "you session dispatches" {
		t.Fatalf("observed command path = %q, want you session dispatches", observation.Parse.CommandPath)
	}
	if len(observation.Parse.Positionals) != 1 || observation.Parse.Positionals[0] != "session-customer" {
		t.Fatalf("observed positional parse = %#v", observation.Parse.Positionals)
	}

	server, found := cliobservation.Flag(observation.Parse, "server")
	if !found || !server.Changed || server.Value != "https://factory.example" {
		t.Fatalf("observed --server parse = %#v found=%v", server, found)
	}
	verbose, found := cliobservation.Flag(observation.Parse, "verbose")
	if !found || !verbose.Changed || verbose.Value != "true" {
		t.Fatalf("observed -v/--verbose parse = %#v found=%v", verbose, found)
	}
	jsonOutput, found := cliobservation.Flag(observation.Parse, "json")
	if !found || !jsonOutput.Changed || jsonOutput.Value != "true" {
		t.Fatalf("observed --json parse = %#v found=%v", jsonOutput, found)
	}
	phase, found := cliobservation.Flag(observation.Parse, "phase")
	if !found || !phase.Changed || phase.Value != "active" {
		t.Fatalf("observed repeated --phase parse = %#v found=%v, want last supplied value active", phase, found)
	}
}

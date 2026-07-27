package parameters_test

import (
	"io/fs"
	"strings"
	"sync/atomic"
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

// TestCLIUnknownFlagFailsBeforeLifecycleStart proves an unknown or removed CLI
// flag is rejected with a stable diagnostic before operator configuration or
// other lifecycle-mutating external effects can start.
func TestCLIUnknownFlagFailsBeforeLifecycleStart(t *testing.T) {
	var mutations atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		OperatorSettingsFileSystem: mutationTrackingOperatorSettingsFileSystem{
			mutations: &mutations,
		},
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "init", "--legacy-scaffold", "legacy-factory",
	})
	inputs.WorkingDirectory = t.TempDir()

	executeErr := process.Execute(inputs.Input)
	if executeErr == nil || !strings.Contains(executeErr.Error(), "unknown flag: --legacy-scaffold") {
		t.Fatalf(
			"unknown init flag error = %v, want unknown flag: --legacy-scaffold; stdout=%q stderr=%q",
			executeErr,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if mutations.Load() != 0 {
		t.Fatalf("configuration mutations after unknown flag = %d, want 0", mutations.Load())
	}
}

// TestCLIFlagAfterPositionalValueUsesDocumentedParsing proves known flags that
// follow a positional value are still parsed at the external observation edge,
// matching the documented public CLI interspersed-flag behavior for manifest
// commands such as session dispatches.
func TestCLIFlagAfterPositionalValueUsesDocumentedParsing(t *testing.T) {
	var observation cliobservation.Result
	process := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"session", "dispatches", "session-customer",
		"--json",
		"--phase", "queued",
	})
	inputs.WorkingDirectory = t.TempDir()

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(session dispatches after-positional flags) error = %v", err)
	}
	if observation.Parse.CommandPath != "you session dispatches" {
		t.Fatalf("observed command path = %q, want you session dispatches", observation.Parse.CommandPath)
	}
	if len(observation.Parse.Positionals) != 1 || observation.Parse.Positionals[0] != "session-customer" {
		t.Fatalf("observed positional parse = %#v", observation.Parse.Positionals)
	}

	jsonOutput, found := cliobservation.Flag(observation.Parse, "json")
	if !found || !jsonOutput.Changed || jsonOutput.Value != "true" {
		t.Fatalf("observed --json after positional = %#v found=%v", jsonOutput, found)
	}
	phase, found := cliobservation.Flag(observation.Parse, "phase")
	if !found || !phase.Changed || phase.Value != "queued" {
		t.Fatalf("observed --phase after positional = %#v found=%v", phase, found)
	}
}

type mutationTrackingOperatorSettingsFileSystem struct {
	mutations *atomic.Int32
}

func (mutationTrackingOperatorSettingsFileSystem) ReadFile(string) ([]byte, error) {
	return nil, fs.ErrNotExist
}

func (fileSystem mutationTrackingOperatorSettingsFileSystem) MkdirAll(string, fs.FileMode) error {
	fileSystem.mutations.Add(1)
	return nil
}

func (fileSystem mutationTrackingOperatorSettingsFileSystem) Remove(string) error {
	fileSystem.mutations.Add(1)
	return nil
}

func (fileSystem mutationTrackingOperatorSettingsFileSystem) Chmod(string, fs.FileMode) error {
	fileSystem.mutations.Add(1)
	return nil
}

func (fileSystem mutationTrackingOperatorSettingsFileSystem) Rename(string, string) error {
	fileSystem.mutations.Add(1)
	return nil
}

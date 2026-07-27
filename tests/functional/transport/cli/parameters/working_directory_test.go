package parameters_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIRelativeFactoryPathResolvesFromInvocationDirectory proves the public CLI
// selects the Current Factory rooted at ./factory/factory.json relative to the
// invocation working directory, reporting the resolved Factory directory through
// customer-visible startup output.
func TestCLIRelativeFactoryPathResolvesFromInvocationDirectory(t *testing.T) {
	invocationDirectory := t.TempDir()
	factoryDirectory := filepath.Join(invocationDirectory, "factory")
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDirectory, "factory.json"),
		[]byte(idleCurrentFactoryJSON),
		0o600,
	); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}

	homeDirectory := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--no-record",
	})
	inputs.Input.WorkingDirectory = invocationDirectory
	inputs.Input.Env = append(
		os.Environ(),
		"HOME="+homeDirectory,
		"USERPROFILE="+homeDirectory,
	)

	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(Current Factory from invocation directory) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	wantInitiated := "Factory initiated: " + factoryDirectory
	if !strings.Contains(inputs.Stdout(), wantInitiated) {
		t.Fatalf(
			"stdout omitted resolved Factory root %q:\n%s",
			wantInitiated,
			inputs.Stdout(),
		)
	}
}

const idleCurrentFactoryJSON = `{
  "name": "current",
  "workTypes": [
    {
      "name": "task",
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [{"name": "processor"}],
  "workstations": [
    {
      "name": "process",
      "inputs": [{"workType": "task", "state": "init"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "worker": "processor"
    }
  ]
}`

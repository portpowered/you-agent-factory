package parameters_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
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

// TestCLIWorkingDirectoryDoesNotLeakIntoOutput proves customer-visible portable
// Factory config flatten output omits the absolute host invocation working
// directory while still returning canonical portable JSON from the public CLI.
func TestCLIWorkingDirectoryDoesNotLeakIntoOutput(t *testing.T) {
	invocationDirectory := t.TempDir()
	factoryDirectory := seedFlattenableFactoryUnderInvocation(t, invocationDirectory)

	homeDirectory := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "flatten", factoryDirectory,
	})
	inputs.Input.WorkingDirectory = invocationDirectory
	inputs.Input.Env = append(
		os.Environ(),
		"HOME="+homeDirectory,
		"USERPROFILE="+homeDirectory,
	)

	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory config flatten) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	portableOutput := inputs.Stdout() + inputs.Stderr()
	if strings.TrimSpace(portableOutput) == "" {
		t.Fatal("portable flatten output is empty")
	}
	if strings.Contains(portableOutput, invocationDirectory) {
		t.Fatalf(
			"portable output leaked invocation working directory %q:\n%s",
			invocationDirectory,
			portableOutput,
		)
	}
}

func seedFlattenableFactoryUnderInvocation(t *testing.T, invocationDirectory string) string {
	t.Helper()

	repositoryRoot := testutil.MustRepoRoot(t)
	sourceFactoryDirectory := filepath.Join(repositoryRoot, "examples", "basic", "factory")
	factoryDirectory := filepath.Join(invocationDirectory, "factory")
	if err := copyDirectoryTree(sourceFactoryDirectory, factoryDirectory); err != nil {
		t.Fatalf("copy flattenable factory fixture: %v", err)
	}
	return factoryDirectory
}

func copyDirectoryTree(sourceDirectory, destinationDirectory string) error {
	return filepath.WalkDir(sourceDirectory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDirectory, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destinationDirectory, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, payload, 0o644)
	})
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

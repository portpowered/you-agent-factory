package parameters_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

	inputs := parameterInputs(t, []string{
		"you", "run", "--no-record",
	})
	inputs.Input.WorkingDirectory = invocationDirectory

	if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err != nil {
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

	inputs := parameterInputs(t, []string{
		"you", "factory", "config", "flatten", factoryDirectory,
	})
	inputs.Input.WorkingDirectory = invocationDirectory

	if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err != nil {
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

// TestCLIMissingWorkingDirectoryAssetFailsActionably proves a missing
// invocation-local Current Factory asset fails with a stable diagnostic before
// provider dispatch or other lifecycle activation side effects can start.
func TestCLIMissingWorkingDirectoryAssetFailsActionably(t *testing.T) {
	invocationDirectory := t.TempDir()
	missingFactoryJSON := filepath.Join(invocationDirectory, "factory", "factory.json")

	beforeLifecycleEffects := parameterProcesses.lifecycleEffects.Load()
	beforeProviderCalls := parameterProcesses.providerRunner.CallCount()
	inputs := parameterInputs(t, []string{
		"you", "run", "--no-record",
	})
	inputs.Input.WorkingDirectory = invocationDirectory

	if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err == nil {
		t.Fatalf(
			"missing Current Factory succeeded; stdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(inputs.Stderr())), &response); err != nil {
		t.Fatalf(
			"stderr is not one ErrorResponse: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if response.Code != factoryapi.ErrorResponseCode("CURRENT_FACTORY_NOT_FOUND") {
		t.Fatalf("ErrorResponse = %#v, want code CURRENT_FACTORY_NOT_FOUND", response)
	}
	diagnostic := response.Message + inputs.Stderr()
	if !strings.Contains(diagnostic, "factory.json") {
		t.Fatalf(
			"diagnostic omitted missing factory.json asset %q:\n%s",
			missingFactoryJSON,
			diagnostic,
		)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("missing Current Factory stdout = %q, want empty", inputs.Stdout())
	}
	if got := parameterProcesses.providerRunner.CallCount() - beforeProviderCalls; got != 0 {
		t.Fatalf("provider dispatch call delta = %d, want 0", got)
	}
	if got := parameterProcesses.lifecycleEffects.Load() - beforeLifecycleEffects; got != 0 {
		t.Fatalf("lifecycle activation effect delta = %d, want 0", got)
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
